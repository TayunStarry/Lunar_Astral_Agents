// ==== movement-controller.js — 模型移动控制系统 ====
//
// 核心设计（先水平后垂直）：
//   - 水平：位置直接驱动 modelRoot 至目标
//   - 垂直：需求跃时通过 CharacterPhysics.startJump 触发（独立跳落，不再自带抛物线）
//   - 对物理体的推挤：统一由 CharacterPhysics 依据移动速度对附近 DYNAMIC 刚体施力

import * as THREE from '../vendor/three.module.js';

const JUMP_ANIM = 'animation.player_to_individuation.jump';
const SNEAK_ANIM = 'animation.player_to_individuation.sneak';
const RIDE_ANIM = 'animation.player_to_individuation.ride';

export class MovementController {
    constructor(deps) {
        this.renderer = deps.renderer;
        this.molang = deps.molang;
        this._characterPhysics = deps.characterPhysics || null;
        this._specialAnimRuntime = null;
        this.onMoveStateChange = deps.onMoveStateChange || (() => {});

        // ==== 目标与状态 ====
        this._target = new THREE.Vector3(0, 0, 0);
        this._hasTarget = false;
        this._isMoving = false;
        this._isFastMoving = false;

        // 移动参数（直接位置驱动，单位/秒）
        this._moveSpeedNormal = 15;
        this._moveSpeedFast = 30;
        this._distanceThreshold = 30;
        this._arrivalDistance = 0.5;

        // ==== 跳跃（委派给 CharacterPhysics 独立跳落） ====
        this._jumpVelocity = 40; // 跳跃初速度（对应 -98.2 重力约 8 单位高）

        // ==== 移动阶段 ====
        this._horizontalArrived = false;  // 水平方向是否已到达
        this._sneakingAnimOnly = false;   // 是否正在播放纯潜行动画（不位移）

        // ==== 坐下冷却 ====
        this._sitting = false;
        this._sitTimer = 0;
        this._sitDuration = 5;

        // ==== 朝向 ====
        this._yaw = 0;
        this._pitch = 0;
        this._targetYaw = 0;
        this._targetPitch = 0;
        this._rotationSpeed = 360;

        // ==== 鼠标追踪 ====
        this._mouseTracking = false;
        this._mouseLock = false;
        this._raycaster = new THREE.Raycaster();
        this._mouseNDC = new THREE.Vector2();
        this._headWorldPos = new THREE.Vector3();
        this._mouseSphereRadius = 10;
        this._mouseRotationSpeed = 120;

        // ==== 自动鼠标追踪状态 ====
        this._mouseInCanvas = false;           // 鼠标是否在画布内
        this._mouseIdleTime = 0;               // 鼠标静止累计时间（秒）
        this._modelStationaryTime = 0;         // 模型未移动累计时间（秒）
        this._autoTrackingCooldown = 5.0;      // 自动追踪冷却倒计时（初始5秒，追踪关闭后开始倒数）
        this._autoThreshold = 3.0;             // 模型静止/鼠标静止阈值（秒）
        this._isAutoResetting = false;         // 是否正在自动回正俯仰/偏航
        this._suppressAutoTracking = false;   // 外部抑制自动追踪（如点击移动模式）

        // ==== 闲置 ====
        this._lastMoveTime = performance.now();
        this._idleThreshold = 5000;
        this._idleTriggered = false;
        this._hasEverMoved = false;

        this._onIdle = null;
        this._onSetTarget = null;

        this._bindMouseEvents();
    }

    // ==== 公开 API ====

    setTarget(x, y, z) {
        if (this._sitting) return;

        this._target.set(x, y, z);
        this._hasTarget = true;
        this._isMoving = true;
        this._horizontalArrived = false;
        this._sneakingAnimOnly = false;

        this._updateTargetRotation();
        this._markMove();
        if (this._onSetTarget) this._onSetTarget();
    }

    setPosition(x, y, z) {
        if (this._sitting) return;
        // 瞬移：竖直状态由 CharacterPhysics 接管
        this._characterPhysics?.placeAt?.(x, y, z);
        this._hasTarget = false;
        this._isMoving = false;
        this._isFastMoving = false;
        this._characterPhysics?.stopMove();
        this._notifyMoveState();
        this._markMove();
    }

    /** 取消当前移动（清除目标，停止移动状态） */
    cancelMovement() {
        this._hasTarget = false;
        this._isMoving = false;
        this._isFastMoving = false;
        this._horizontalArrived = false;
        this._characterPhysics?.stopMove();
        this._notifyMoveState();
    }

    setRotation(yaw, pitch) {
        this._targetYaw = yaw;
        this._targetPitch = pitch;
    }

    setMouseTracking(enabled) {
        // 允许外部强制设置（如执行动作时），覆盖自动逻辑
        this._mouseTracking = enabled;
        if (enabled) {
            this._isAutoResetting = false;
            this._targetYaw = 0;
            this._targetPitch = 0;
            this._yaw = 0;
            this._pitch = 0;
        } else {
            this._mouseLock = false;
        }
    }

    setMouseLock(enabled) {
        this._mouseLock = enabled;
        if (enabled) this._mouseTracking = true;
    }

    /** 抑制自动追踪（如点击移动模式启用时调用） */
    suppressAutoTracking(suppress) {
        this._suppressAutoTracking = suppress;
        if (suppress) {
            this._mouseTracking = false;
            this._isAutoResetting = true;
        }
    }

    setCharacterPhysics(cp) { this._characterPhysics = cp; }
    setSpecialAnimRuntime(sar) { this._specialAnimRuntime = sar; }

    sitDown() {
        if (this._sitting) return;
        this._sitting = true;
        this._sitTimer = 0;
        this._hasTarget = false;
        this._isMoving = false;
        this._horizontalArrived = false;
        this._sneakingAnimOnly = false;

        this._characterPhysics?.stopMove();
        this._playAnim(RIDE_ANIM);
        this._notifyMoveState();
        console.log('[MovementController] 坐下5秒');
    }

    // ==== 每帧更新 ====

    tick(dt) {
        // 1. 坐下冷却
        if (this._sitting) {
            this._sitTimer += dt;
            if (this._sitTimer >= this._sitDuration) {
                this._sitting = false;
                this._stopAnim(RIDE_ANIM);
                this._pitch = 0;
                this._targetPitch = 0;
            }
            this._syncMolang();
            return;
        }

        // 2. 自动鼠标追踪状态机
        this._updateAutoTracking(dt);

        // 3. 鼠标追踪（由自动状态机决定 _mouseTracking）
        this._updateHeadPosition();
        if (this._mouseTracking) {
            this._computeSphereTracking();
        }

        // 4. 自动回正俯仰/偏航（鼠标追踪关闭时逐渐归零）
        if (this._isAutoResetting) {
            this._tickAutoReset(dt);
        }

        // 5. 移动（竖直/跳跃由 CharacterPhysics 独立处理）
        this._tickMove(dt);

        // 7. 朝向插值（自动回正期间由 _tickAutoReset 自行处理，避免冲突）
        if (!this._isAutoResetting) {
            this._updateRotation(dt);
        }

        // 8. 同步 MoLang
        this._syncMolang();

        // 9. 闲置检测
        if (this._hasEverMoved && !this._idleTriggered
            && (performance.now() - this._lastMoveTime > this._idleThreshold)) {
            this._idleTriggered = true;
            if (this._onIdle) this._onIdle();
        }
    }

    // ==== 移动逻辑（先水平后垂直） ====

    _tickMove(dt) {
        const root = this.renderer?.modelRoot;
        if (!root || !this._hasTarget) {
            if (this._isMoving) {
                this._isMoving = false;
                this._isFastMoving = false;
                this._stopAnim(SNEAK_ANIM);
                this._notifyMoveState();
            }
            return;
        }

        const curPos = root.position;
        const dx = this._target.x - curPos.x;
        const dz = this._target.z - curPos.z;
        const horizontalDist = Math.sqrt(dx * dx + dz * dz);
        const dy = this._target.y - curPos.y;

        // ---- 阶段1：水平移动（竖直由 CharacterPhysics 独立处理；推挤也由它统一施加） ----
        if (!this._horizontalArrived) {
            if (horizontalDist > this._arrivalDistance) {
                const isFast = horizontalDist > this._distanceThreshold;
                const speed = isFast ? this._moveSpeedFast : this._moveSpeedNormal;
                this._isMoving = true;
                this._isFastMoving = isFast;

                const step = speed * dt;
                const nx = dx / horizontalDist;
                const nz = dz / horizontalDist;

                if (step >= horizontalDist) {
                    curPos.x = this._target.x;
                    curPos.z = this._target.z;
                } else {
                    curPos.x += nx * step;
                    curPos.z += nz * step;
                }

                this._notifyMoveState();
                this._markMove();
                return; // 水平未到，不处理垂直
            } else {
                this._horizontalArrived = true;
            }
        }

        // ---- 阶段2：水平已到达，处理垂直 ----
        if (Math.abs(dy) < 0.5) {
            this._stopAnim(SNEAK_ANIM);
            this._arrive();
            return;
        }
        if (dy > 0.3) {
            // 需要上升 → 触发跳跃（由 CharacterPhysics 控制跳跃/下落）
            this._stopAnim(SNEAK_ANIM);
            this._characterPhysics?.startJump?.(this._jumpVelocity);
        } else if (dy < -0.3) {
            // 目标更低 → 由角色物理自然下落
            if (!this._sneakingAnimOnly) {
                this._sneakingAnimOnly = true;
                this._playAnim(SNEAK_ANIM);
            }
        }

        this._notifyMoveState();
        this._markMove();
    }

    // ==== 到达目标 ====

    _arrive() {
        this._hasTarget = false;
        this._isMoving = false;
        this._isFastMoving = false;
        this._horizontalArrived = false;
        this._sneakingAnimOnly = false;
        this._characterPhysics?.stopMove();
        this._stopAnim(JUMP_ANIM);
        this._stopAnim(SNEAK_ANIM);
        this._notifyMoveState();
    }

    // ==== 动画 ====

    _playAnim(animName) {
        if (animName === JUMP_ANIM) this._specialAnimRuntime?.setJumping?.(true);
        else if (animName === SNEAK_ANIM) this._specialAnimRuntime?.setSneaking?.(true);
        else if (animName === RIDE_ANIM) this._specialAnimRuntime?.setSitting?.(true);
    }

    _stopAnim(animName) {
        if (animName === JUMP_ANIM) this._specialAnimRuntime?.setJumping?.(false);
        else if (animName === SNEAK_ANIM) {
            this._specialAnimRuntime?.setSneaking?.(false);
            if (this._sneakingAnimOnly) {
                this._sneakingAnimOnly = false;
                this._pitch = 0;       // 潜行结束回正俯仰
                this._targetPitch = 0;
            }
        }
        else if (animName === RIDE_ANIM) this._specialAnimRuntime?.setSitting?.(false);
    }

    // ==== 属性 ====

    get currentPosition() {
        const root = this.renderer?.modelRoot;
        return root ? { x: root.position.x, y: root.position.y, z: root.position.z } : { x: 0, y: 0, z: 0 };
    }
    get currentRotation() { return { yaw: this._yaw, pitch: this._pitch }; }
    get isMoving() { return this._isMoving; }
    get isFastMoving() { return this._isFastMoving; }
    get mouseTracking() { return this._mouseTracking; }

    onIdle(cb) { this._onIdle = cb; }
    onSetTarget(cb) { this._onSetTarget = cb; }

    _markMove() {
        this._lastMoveTime = performance.now();
        this._idleTriggered = false;
        this._hasEverMoved = true;
    }

    // ==== 内部实现 ====

    _updateTargetRotation() {
        const curPos = this.currentPosition;
        const dx = this._target.x - curPos.x;
        const dz = this._target.z - curPos.z;

        if (Math.abs(dx) > 0.001 || Math.abs(dz) > 0.001) {
            this._targetYaw = Math.atan2(dx, dz) * 180 / Math.PI + 180;
        }

        const horizontalDist = Math.sqrt(dx * dx + dz * dz);
        const dy = this._target.y - curPos.y;
        if (horizontalDist > 0.001) {
            this._targetPitch = Math.max(-89, Math.min(89, Math.atan2(dy, horizontalDist) * 180 / Math.PI));
        }
    }

    _updateRotation(dt) {
        let yawDiff = this._targetYaw - this._yaw;
        while (yawDiff > 180) yawDiff -= 360;
        while (yawDiff < -180) yawDiff += 360;

        const maxStep = this._rotationSpeed * dt;
        this._yaw += Math.abs(yawDiff) <= maxStep ? yawDiff : Math.sign(yawDiff) * maxStep;
        const pitchDiff = this._targetPitch - this._pitch;
        this._pitch += Math.abs(pitchDiff) <= maxStep ? pitchDiff : Math.sign(pitchDiff) * maxStep;

        while (this._yaw > 180) this._yaw -= 360;
        while (this._yaw < -180) this._yaw += 360;
    }

    _syncMolang() {
        const isJumpPhase = this._characterPhysics?.isGrounded === false; // 空中视为跳跃阶段
        const isSneakPhase = this._sneakingAnimOnly || (this._characterPhysics?.isSneaking ?? false);
        const suppressWalk = this._sitting || isJumpPhase || isSneakPhase;
        const effectiveMoving = suppressWalk ? false : this._isMoving;

        this.molang.updateContext({
            target_x_rotation: this._pitch,
            target_y_rotation: this._yaw,
            is_moving: effectiveMoving ? 1 : 0,
            is_sprinting: (effectiveMoving && this._isFastMoving) ? 1 : 0,
            is_on_ground: this._characterPhysics?.isGrounded ? 1 : 0,
            is_jumping: isJumpPhase ? 1 : 0,
            is_sneaking: isSneakPhase ? 1 : 0,
        });
    }

    _notifyMoveState() {
        const isJumpPhase = this._characterPhysics?.isGrounded === false;
        const isSneakPhase = this._sneakingAnimOnly || (this._characterPhysics?.isSneaking ?? false);
        const suppressWalk = this._sitting || isJumpPhase || isSneakPhase;
        this.onMoveStateChange(suppressWalk ? false : this._isMoving, this._isFastMoving);
    }

    // ==== 鼠标 ====

    _bindMouseEvents() {
        const canvas = this.renderer?.canvas;
        if (!canvas) return;

        this._onMouseMove = this._onMouseMove.bind(this);
        this._onMouseEnter = this._onMouseEnter.bind(this);
        this._onMouseLeave = this._onMouseLeave.bind(this);

        canvas.addEventListener('mousemove', this._onMouseMove);
        canvas.addEventListener('mouseenter', this._onMouseEnter);
        canvas.addEventListener('mouseleave', this._onMouseLeave);
    }

    _onMouseMove(e) {
        const canvas = this.renderer?.canvas;
        if (!canvas) return;
        const rect = canvas.getBoundingClientRect();
        this._mouseNDC.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
        this._mouseNDC.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;

        // 重置鼠标静止计时器（有移动 = 不再静止）
        this._mouseIdleTime = 0;
    }

    _onMouseEnter() {
        this._mouseInCanvas = true;
    }

    _onMouseLeave() {
        this._mouseInCanvas = false;
    }

    // ==== 自动鼠标追踪状态机 ====

    _updateAutoTracking(dt) {
        const isMoving = this._isMoving || this._characterPhysics?.isGrounded === false;
        const wasTracking = this._mouseTracking;

        // ---- 更新计时器 ----
        if (this._mouseInCanvas) {
            this._mouseIdleTime += dt;
        }

        if (isMoving) {
            // 模型正在移动：重置静止计时器，冷却恢复为满值
            this._modelStationaryTime = 0;
            this._autoTrackingCooldown = 5.0;
        } else {
            // 模型静止：累加静止时间
            this._modelStationaryTime += dt;
        }

        // 追踪关闭时：冷却倒计时递减
        if (!this._mouseTracking) {
            this._autoTrackingCooldown = Math.max(0, this._autoTrackingCooldown - dt);
        }

        // ---- 关闭追踪的条件 ----
        if (this._mouseTracking) {
            const shouldDisable =
                !this._mouseInCanvas ||                          // 鼠标离开画布
                this._mouseIdleTime >= this._autoThreshold;      // 鼠标静止超过3秒

            if (shouldDisable) {
                this._mouseTracking = false;
                this._isAutoResetting = true;
                this._autoTrackingCooldown = 5.0;                // 启动冷却倒计时
            }
        }

        // ---- 开启追踪的条件 ----
        if (!this._mouseTracking && !this._isAutoResetting && !this._suppressAutoTracking) {
            const shouldEnable =
                this._modelStationaryTime >= this._autoThreshold &&   // 模型3秒未移动
                this._autoTrackingCooldown <= 0 &&                    // 冷却倒计时结束
                this._mouseInCanvas;                                   // 鼠标在画布内

            if (shouldEnable) {
                this._mouseTracking = true;
                // 重置计时器，避免刚开启就被关闭条件触发
                this._mouseIdleTime = 0;
                // 重置追踪角度
                this._targetYaw = 0;
                this._targetPitch = 0;
                this._yaw = 0;
                this._pitch = 0;
            }
        }

        // ---- 追踪状态变化时通知外部 ----
        if (wasTracking !== this._mouseTracking) {
            this._onTrackingChanged?.(this._mouseTracking);
        }
    }

    // ==== 自动回正俯仰/偏航（逐渐归零） ====

    _tickAutoReset(dt) {
        const resetSpeed = 90; // 度/秒

        // 回正偏航
        let yawDiff = 0 - this._yaw;
        while (yawDiff > 180) yawDiff -= 360;
        while (yawDiff < -180) yawDiff += 360;
        const maxYawStep = resetSpeed * dt;
        this._yaw += Math.abs(yawDiff) <= maxYawStep ? yawDiff : Math.sign(yawDiff) * maxYawStep;

        // 回正俯仰
        const pitchDiff = 0 - this._pitch;
        const maxPitchStep = resetSpeed * dt;
        this._pitch += Math.abs(pitchDiff) <= maxPitchStep ? pitchDiff : Math.sign(pitchDiff) * maxPitchStep;

        // 同步 target 值（避免 _updateRotation 干扰）
        this._targetYaw = this._yaw;
        this._targetPitch = this._pitch;

        // 检查是否已归零
        if (Math.abs(this._yaw) < 0.1 && Math.abs(this._pitch) < 0.1) {
            this._yaw = 0;
            this._pitch = 0;
            this._targetYaw = 0;
            this._targetPitch = 0;
            this._isAutoResetting = false;
        }
    }

    /** 注册追踪状态变化回调（供引擎广播 mouse_tracking_changed） */
    onTrackingChanged(cb) { this._onTrackingChanged = cb; }

    _updateHeadPosition() {
        if (this.renderer?.getHeadWorldPosition) {
            const pos = this.renderer.getHeadWorldPosition();
            if (pos) this._headWorldPos.copy(pos);
        }
    }

    _computeSphereTracking() {
        const camera = this.renderer?.camera;
        if (!camera) return;

        this._raycaster.setFromCamera(this._mouseNDC, camera);
        const ray = this._raycaster.ray;
        const L = ray.origin.clone().sub(this._headWorldPos);
        const a = ray.direction.dot(ray.direction);
        const b = 2 * ray.direction.dot(L);
        const c = L.dot(L) - this._mouseSphereRadius * this._mouseSphereRadius;
        const discriminant = b * b - 4 * a * c;

        let hitPoint;
        if (discriminant >= 0) {
            const t = (-b - Math.sqrt(discriminant)) / (2 * a);
            hitPoint = t > 0
                ? ray.origin.clone().addScaledVector(ray.direction, t)
                : ray.origin.clone().addScaledVector(ray.direction, (-b + Math.sqrt(discriminant)) / (2 * a));
        } else {
            const tCa = -ray.direction.dot(L) / a;
            const closestOnRay = ray.origin.clone().addScaledVector(ray.direction, tCa);
            const dirToSphere = this._headWorldPos.clone().sub(closestOnRay);
            const distToSphere = dirToSphere.length();
            if (distToSphere < 0.001) {
                hitPoint = this._headWorldPos.clone().add(new THREE.Vector3(0, this._mouseSphereRadius, 0));
            } else {
                dirToSphere.normalize();
                hitPoint = this._headWorldPos.clone().addScaledVector(dirToSphere, -this._mouseSphereRadius);
            }
        }

        const dir = hitPoint.clone().sub(this._headWorldPos);
        const dist = dir.length();
        if (dist < 0.001) return;
        dir.normalize();

        let targetYaw = Math.atan2(dir.x, dir.z) * 180 / Math.PI + 180;
        while (targetYaw > 180) targetYaw -= 360;
        while (targetYaw < -180) targetYaw += 360;
        targetYaw = Math.max(-45, Math.min(45, targetYaw));

        const hDist = Math.sqrt(dir.x * dir.x + dir.z * dir.z);
        let targetPitch = Math.max(-89, Math.min(89, Math.atan2(dir.y, hDist) * 180 / Math.PI));

        const maxStep = this._mouseRotationSpeed * (1 / 60);
        let yawDiff = targetYaw - this._targetYaw;
        while (yawDiff > 180) yawDiff -= 360;
        while (yawDiff < -180) yawDiff += 360;
        this._targetYaw += Math.abs(yawDiff) <= maxStep ? yawDiff : Math.sign(yawDiff) * maxStep;
        const pitchDiff = targetPitch - this._targetPitch;
        this._targetPitch += Math.abs(pitchDiff) <= maxStep ? pitchDiff : Math.sign(pitchDiff) * maxStep;
    }

    dispose() {
        const canvas = this.renderer?.canvas;
        if (canvas) {
            canvas.removeEventListener('mousemove', this._onMouseMove);
            canvas.removeEventListener('mouseenter', this._onMouseEnter);
            canvas.removeEventListener('mouseleave', this._onMouseLeave);
        }
    }
}
