// ==== movement-controller.js — 模型移动控制系统 ====
//
// 核心设计（先水平后垂直）：
//   - 阶段1：水平移动至目标XY，忽略垂直偏差
//   - 阶段2：水平到达后处理垂直（正→跳跃，负→仅播放动画不位移）
//   - 跳跃：sin(π·progress) 抛物线直接写 modelRoot.position.y
//   - 跳跃失败落地：射线检测地面/图元表面，确保落在正确高度
//   - 3次跳跃失败 → 坐下5秒

import * as THREE from '../vendor/three.module.js';
import * as CANNON from '../vendor/cannon-es.module.js';

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

        // ==== 跳跃状态 ====
        this._jumping = false;
        this._jumpStartY = 0;
        this._jumpDuration = 0.6; // 一次跳跃的总时长（秒）
        this._jumpTimer = 0;
        this._jumpHeight = 5;     // 跳跃最大高度
        this._jumpFailCount = 0;
        this._jumpFailMax = 3;

        // ==== 移动阶段 ====
        this._horizontalArrived = false;  // 水平方向是否已到达
        this._sneakingAnimOnly = false;   // 是否正在播放纯潜行动画（不位移）
        this._justLanded = false;         // 刚落地，需要在地面判定跳跃结果

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
        this._justLanded = false;
        this._jumpFailCount = 0;

        this._updateTargetRotation();
        this._markMove();
        if (this._onSetTarget) this._onSetTarget();
    }

    setPosition(x, y, z) {
        if (this._sitting) return;
        // 瞬移
        const root = this.renderer?.modelRoot;
        if (root) {
            root.position.set(x, y, z);
        }
        const cp = this._characterPhysics;
        if (cp?._body) {
            const halfH = cp.getBoxHeight() / 2;
            cp._body.position.set(x, y + halfH, z);
            cp._body.velocity.set(0, 0, 0);
        }
        this._hasTarget = false;
        this._isMoving = false;
        this._isFastMoving = false;
        this._jumping = false;
        cp?.stopMove();
        this._notifyMoveState();
        this._markMove();
    }

    /** 取消当前移动（清除目标，停止移动状态） */
    cancelMovement() {
        this._hasTarget = false;
        this._isMoving = false;
        this._isFastMoving = false;
        this._jumping = false;
        this._horizontalArrived = false;
        this._characterPhysics?.stopMove();
        this._notifyMoveState();
    }

    setRotation(yaw, pitch) {
        this._targetYaw = yaw;
        this._targetPitch = pitch;
    }

    setMouseTracking(enabled) {
        this._mouseTracking = enabled;
        if (enabled) {
            // 不再锁定视角，只重置偏航/俯仰
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

    setCharacterPhysics(cp) { this._characterPhysics = cp; }
    setSpecialAnimRuntime(sar) { this._specialAnimRuntime = sar; }

    sitDown() {
        if (this._sitting) return;
        this._sitting = true;
        this._sitTimer = 0;
        this._hasTarget = false;
        this._isMoving = false;
        this._jumping = false;
        this._horizontalArrived = false;
        this._sneakingAnimOnly = false;
        this._justLanded = false;

        this._characterPhysics?.stopMove();
        this._playAnim(RIDE_ANIM);
        this._notifyMoveState();
        console.log('[MovementController] 跳跃失败3次，坐下5秒');
    }

    // ==== 每帧更新 ====

    tick(dt) {
        // 1. 坐下冷却
        if (this._sitting) {
            this._sitTimer += dt;
            if (this._sitTimer >= this._sitDuration) {
                this._sitting = false;
                this._stopAnim(RIDE_ANIM);
                this._pitch = 0;       // 坐下结束回正俯仰
                this._targetPitch = 0;
            }
            this._syncMolang();
            return;
        }

        // 2. 鼠标追踪
        this._updateHeadPosition();
        if (this._mouseTracking) {
            this._computeSphereTracking();
        }

        // 3. 跳跃更新（数学抛物线直接驱动Y）
        if (this._jumping) {
            this._tickJump(dt);
            this._syncMolang();
            return;
        }

        // 5. 移动
        this._tickMove(dt);

        // 6. 朝向插值
        this._updateRotation(dt);

        // 7. 同步 MoLang
        this._syncMolang();

        // 8. 闲置检测
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
                this._justLanded = false;
                this._notifyMoveState();
            }
            return;
        }

        const curPos = root.position;
        const dx = this._target.x - curPos.x;
        const dz = this._target.z - curPos.z;
        const horizontalDist = Math.sqrt(dx * dx + dz * dz);
        const dy = this._target.y - curPos.y;

        // ---- 落地判定（跳跃弧线结束即落地，位置由数学驱动） ----
        if (this._justLanded) {
            this._justLanded = false;

            // 射线检测落地表面，确保站在正确的地面/图元上
            const surfaceY = this._detectSurfaceY(curPos);
            if (curPos.y < surfaceY) {
                curPos.y = surfaceY;
                this._syncPhysicsBody();
            }

            // 判定跳跃是否成功
            if (dy > 0.3) {
                this._jumpFailCount++;
                console.log(`[MovementController] 跳跃未到达目标，失败计数: ${this._jumpFailCount}/${this._jumpFailMax}`);
                if (this._jumpFailCount >= this._jumpFailMax) {
                    this._onJumpFailed();
                    return;
                }
            } else {
                this._jumpFailCount = 0;
            }
            // 继续后续逻辑（可能再次触发跳跃）
        }

        // ---- 阶段1：水平移动 ----
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

                this._pushObstacles(nx, nz);
                this._syncPhysicsBody();
                this._notifyMoveState();
                this._markMove();
                return; // 水平未到，不处理垂直
            } else {
                // 水平到达
                this._horizontalArrived = true;
            }
        }

        // ---- 阶段2：水平已到达，处理垂直 ----

        // 综合判定到达（水平已到 + 垂直可接受）
        if (Math.abs(dy) < 0.5) {
            this._stopAnim(SNEAK_ANIM);
            this._arrive();
            return;
        }

        if (dy > 0.3) {
            // 需要上升 → 跳跃
            this._stopAnim(SNEAK_ANIM);
            this._startJump();
        } else if (dy < -0.3) {
            // 需要下降 → 仅播放潜行动画，不实际下移
            if (!this._sneakingAnimOnly) {
                this._sneakingAnimOnly = true;
                this._playAnim(SNEAK_ANIM);
            }
        }

        this._notifyMoveState();
        this._markMove();
    }

    // ==== 跳跃（数学抛物线直接驱动Y，不依赖物理引擎） ====

    _startJump() {
        const root = this.renderer?.modelRoot;
        if (!root || this._jumping) return;

        this._jumping = true;
        this._jumpStartY = root.position.y;
        this._jumpTimer = 0;

        // 播放跳跃动画
        this._playAnim(JUMP_ANIM);
        this._stopAnim(SNEAK_ANIM);

        console.log(`[MovementController] 跳跃: ${this._jumpStartY.toFixed(1)} → +${this._jumpHeight}`);
    }

    _tickJump(dt) {
        const root = this.renderer?.modelRoot;
        const cp = this._characterPhysics;
        if (!root) return;

        this._jumpTimer += dt;

        // ---- sin 抛物线驱动 Y 位置 ----
        const progress = Math.min(this._jumpTimer / this._jumpDuration, 1.0);
        const arcY = this._jumpStartY + this._jumpHeight * Math.sin(Math.PI * progress);
        root.position.y = arcY;

        // 物理体跟随模型Y（阻止物理引擎干扰）
        if (cp?._body) {
            const halfH = cp.getBoxHeight() / 2;
            cp._body.position.y = root.position.y + halfH;
            cp._body.velocity.y = 0;
        }

        // ---- 跳跃弧线结束 → 进入落地阶段 ----
        if (progress >= 1.0) {
            this._jumping = false;
            this._stopAnim(JUMP_ANIM);

            // 角色落回起跳点高度（弧线是对称的，回到 _jumpStartY）
            root.position.y = this._jumpStartY;
            if (cp?._body) {
                const halfH = cp.getBoxHeight() / 2;
                cp._body.position.y = root.position.y + halfH;
                cp._body.velocity.y = 0;
            }

            // 设置落地标记，等下一帧 _tickMove 在地面判定跳跃结果
            this._justLanded = true;
        }
    }

    // ==== 射线检测当前XZ位置的表面高度 ====

    _detectSurfaceY(position) {
        const pm = this._characterPhysics?.physicsManager;
        if (!pm) return 0;

        const surfaceY = pm.groundY ?? 0;

        // 检查当前XZ位置下方是否有图元
        for (const [id, body] of pm.bodies) {
            if (body.type !== CANNON.Body.STATIC && body.type !== CANNON.Body.DYNAMIC) continue;
            // 简单AABB检测：图元XZ范围是否覆盖当前角色位置
            for (const shape of body.shapes) {
                if (shape.type === CANNON.Shape.types.BOX) {
                    const hx = shape.halfExtents.x;
                    const hz = shape.halfExtents.z;
                    const bx = body.position.x;
                    const bz = body.position.z;
                    if (Math.abs(position.x - bx) < hx && Math.abs(position.z - bz) < hz) {
                        const topY = body.position.y + shape.halfExtents.y;
                        if (topY > surfaceY && topY <= position.y + 1) {
                            return topY; // 站在图元上方
                        }
                    }
                }
            }
        }

        return surfaceY;
    }

    _onJumpFailed() {
        this._jumpFailCount = 0;
        this._jumping = false;
        this.sitDown();
    }

    // ==== 推开障碍物 ====

    _pushObstacles(dirX, dirZ) {
        const cp = this._characterPhysics;
        const pm = cp?.physicsManager;
        if (!pm || !cp?._body) return;

        const myPos = cp._body.position;
        const pushForce = 200;

        for (const [id, body] of pm.bodies) {
            if (body.type !== CANNON.Body.DYNAMIC) continue;
            const dx = body.position.x - myPos.x;
            const dz = body.position.z - myPos.z;
            const dist = Math.sqrt(dx * dx + dz * dz);
            if (dist < 3 && dist > 0.01) {
                // 在前方且距离近 → 施加冲量推开
                const dot = (dx * dirX + dz * dirZ);
                if (dot > -0.3) { // 前方或侧方
                    const impulse = new CANNON.Vec3(
                        (dx / dist) * pushForce,
                        50,
                        (dz / dist) * pushForce
                    );
                    body.applyImpulse(impulse, body.position);
                }
            }
        }
    }

    // ==== 到达目标 ====

    _arrive() {
        this._hasTarget = false;
        this._isMoving = false;
        this._isFastMoving = false;
        this._horizontalArrived = false;
        this._sneakingAnimOnly = false;
        this._justLanded = false;
        this._jumpFailCount = 0;
        this._characterPhysics?.stopMove();
        this._stopAnim(JUMP_ANIM);
        this._stopAnim(SNEAK_ANIM);
        this._notifyMoveState();
    }

    // ==== 同步物理体位置到 modelRoot ====

    _syncPhysicsBody() {
        const root = this.renderer?.modelRoot;
        const cp = this._characterPhysics;
        if (!root || !cp?._body) return;

        const halfH = cp.getBoxHeight() / 2;
        // 水平位置由 modelRoot 驱动，物理体跟随
        cp._body.position.x = root.position.x;
        cp._body.position.z = root.position.z;
        // Y 位置：跳跃中由数学抛物线直接驱动 modelRoot
        // 非跳跃中由 modelRoot 驱动物理体
        if (!this._jumping) {
            cp._body.position.y = root.position.y + halfH;
            cp._body.velocity.x = 0;
            cp._body.velocity.z = 0;
        }
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
    get isMoving() { return this._isMoving || this._jumping; }
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
        const isJumpPhase = this._jumping;
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
        const isJumpPhase = this._jumping;
        const isSneakPhase = this._sneakingAnimOnly || (this._characterPhysics?.isSneaking ?? false);
        const suppressWalk = this._sitting || isJumpPhase || isSneakPhase;
        this.onMoveStateChange(suppressWalk ? false : this._isMoving, this._isFastMoving);
    }

    // ==== 鼠标 ====

    _bindMouseEvents() {
        this._onMouseMove = this._onMouseMove.bind(this);
        window.addEventListener('mousemove', this._onMouseMove);
    }

    _onMouseMove(e) {
        const canvas = this.renderer?.canvas;
        if (!canvas) return;
        const rect = canvas.getBoundingClientRect();
        this._mouseNDC.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
        this._mouseNDC.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;
    }

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
        window.removeEventListener('mousemove', this._onMouseMove);
    }
}
