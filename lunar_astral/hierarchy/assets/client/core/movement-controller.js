// ==== movement-controller.js — 模型移动控制系统 ====
//
// 职责：
//   - 平滑移动模型到目标坐标（基于 modelRoot.position）
//   - 朝向控制：仅设置 q.target_x_rotation / q.target_y_rotation（不再旋转 modelRoot）
//   - 身体旋转由 BodyRotationInterpreter + 动画骨骼驱动（whole 骨骼引用 q.body_y_rotation）
//   - 碰撞检测：禁止穿过地板（Y >= 0）及边界
//   - 鼠标追踪：将鼠标位置投影到地面，作为朝向目标
//   - 自动锁定：持续锁定目标为鼠标位置
//   - 同步 MoLang 变量：is_moving / ground_speed / move_speed / target_*_rotation
//   - 通知 SpecialAnimationRuntime 移动状态变化
//   - 5 秒无操作触发 onIdle 回调（用于摄像头归位）

import * as THREE from '../three.module.js';

/**
 * 移动控制器
 *
 * 坐标系约定（Bedrock / Three.js）：
 *   X = 东/西，Y = 上/下，Z = 南/北
 *   偏航（yaw）= 绕 Y 轴旋转 = target_y_rotation
 *   俯仰（pitch）= 绕 X 轴旋转 = target_x_rotation
 */
export class MovementController {
    /**
     * @param {{
     *   renderer: import('./renderer.js').Renderer,
     *   molang: import('./molang-runtime.js').MolangRuntime,
     *   onMoveStateChange?: (isMoving: boolean, isFastMoving: boolean) => void
     * }} deps
     */
    constructor(deps) {
        this.renderer = deps.renderer;
        this.molang = deps.molang;
        this.onMoveStateChange = deps.onMoveStateChange || (() => {});

        // ==== 移动状态 ====
        /** @type {THREE.Vector3} 当前位置 */
        this._position = new THREE.Vector3(0, 0, 0);
        /** @type {THREE.Vector3} 目标位置 */
        this._target = new THREE.Vector3(0, 0, 0);
        /** @type {boolean} 是否正在移动 */
        this._isMoving = false;
        /** @type {boolean} 是否正在快速移动 */
        this._isFastMoving = false;

        // 移动参数
        /** @type {number} 普通移动速度（单位/秒），距离 ≤ 阈值时使用 */
        this._moveSpeedNormal = 30;
        /** @type {number} 快速移动速度（单位/秒），距离 > 阈值时使用 */
        this._moveSpeedFast = 45;
        /** @type {number} 距离阈值（单位）：超过此距离判定为快速移动 */
        this._distanceThreshold = 30;
        /** @type {number} 到达目标的判定距离 */
        this._arrivalDistance = 0.05;

        // ==== 朝向状态 ====
        /** @type {number} 当前偏航（度） */
        this._yaw = 0;
        /** @type {number} 当前俯仰（度） */
        this._pitch = 0;
        /** @type {number} 目标偏航（度） */
        this._targetYaw = 0;
        /** @type {number} 目标俯仰（度） */
        this._targetPitch = 0;
        /** @type {number} 朝向插值速度（度/秒） */
        this._rotationSpeed = 360;

        // ==== 鼠标追踪（球面映射） ====
        /** @type {boolean} 鼠标追踪是否启用 */
        this._mouseTracking = false;
        /** @type {boolean} 自动锁定鼠标位置 */
        this._mouseLock = false;
        /** @type {THREE.Raycaster} */
        this._raycaster = new THREE.Raycaster();
        /** @type {THREE.Vector2} 归一化鼠标坐标 */
        this._mouseNDC = new THREE.Vector2();
        /** @type {THREE.Vector3} 头部世界位置（每帧更新） */
        this._headWorldPos = new THREE.Vector3();
        /** @type {number} 球面半径（单位） */
        this._mouseSphereRadius = 10;
        /** @type {number} 鼠标追踪朝向过渡速度（度/秒，比移动朝向略慢） */
        this._mouseRotationSpeed = 120;

        // ==== 碰撞边界 ====
        /** @type {number} 地板高度（Y 最小值） */
        this._floorY = 0;
        /** @type {number} 边界范围（正方形区域 [-bound, bound]） */
        this._bound = 100;

        // ==== 回调 ====
        this._onPositionChange = null;
        this._onRotationChange = null;
        /** @type {(() => void)|null} 5 秒无操作回调 */
        this._onIdle = null;
        /** @type {(() => void)|null} setTarget 时回调（用于同步 body_y_rotation） */
        this._onSetTarget = null;

        // ==== 闲置检测 ====
        /** @type {number} 最后移动时间戳（毫秒，仅位置变化时更新） */
        this._lastMoveTime = performance.now();
        /** @type {number} 闲置触发阈值（毫秒） */
        this._idleThreshold = 5000;
        /** @type {boolean} 是否已触发闲置 */
        this._idleTriggered = false;
        /** @type {boolean} 是否曾发生过移动（一次都没有则永不触发闲置） */
        this._hasEverMoved = false;

        this._bindMouseEvents();
    }

    // ==== 公开 API ====

    /**
     * 设置目标位置
     * @param {number} x
     * @param {number} y
     * @param {number} z
     */
    setTarget(x, y, z) {
        // 碰撞检测：Y 不低于地板
        const clampedY = Math.max(this._floorY, y);
        // 边界检测
        const clampedX = Math.max(-this._bound, Math.min(this._bound, x));
        const clampedZ = Math.max(-this._bound, Math.min(this._bound, z));
        this._target.set(clampedX, clampedY, clampedZ);

        // 计算朝向目标方向
        this._updateTargetRotation();
        this._markMove();
        if (this._onSetTarget) this._onSetTarget();
    }

    /**
     * 直接设置模型位置（瞬移，不平滑移动）
     * @param {number} x
     * @param {number} y
     * @param {number} z
     */
    setPosition(x, y, z) {
        const clampedY = Math.max(this._floorY, y);
        const clampedX = Math.max(-this._bound, Math.min(this._bound, x));
        const clampedZ = Math.max(-this._bound, Math.min(this._bound, z));
        this._position.set(clampedX, clampedY, clampedZ);
        this._target.copy(this._position);
        this._applyPosition();
        this._isMoving = false;
        this._isFastMoving = false;
        this._notifyMoveState();
        this._markMove();
    }

    /**
     * 设置朝向（度）
     * @param {number} yaw 偏航
     * @param {number} pitch 俯仰
     */
    setRotation(yaw, pitch) {
        this._targetYaw = yaw;
        this._targetPitch = pitch;
    }

    /**
     * 立即设置朝向（无过渡）
     * @param {number} yaw
     * @param {number} pitch
     */
    setRotationImmediate(yaw, pitch) {
        this._yaw = yaw;
        this._pitch = pitch;
        this._targetYaw = yaw;
        this._targetPitch = pitch;
        this._applyRotation();
    }

    /**
     * 启用/禁用鼠标追踪
     * 启用时：重置朝向为 0，锁定摄像头到 (4, 28.5, 73) 聚焦角色
     * 禁用时：解锁摄像头，恢复轨道控制
     * @param {boolean} enabled
     */
    setMouseTracking(enabled) {
        this._mouseTracking = enabled;
        if (enabled) {
            // a. 重置朝向为 0
            this._targetYaw = 0;
            this._targetPitch = 0;
            this._yaw = 0;
            this._pitch = 0;

            // b. 不锁定摄像头，仅开启鼠标追踪（头部/身体跟随鼠标旋转）
            // 摄像头保持 _fitCameraToModel 的初始位置，轨道控制仍可用
        } else {
            this._mouseLock = false;
            // 解锁摄像头
            if (this.renderer?.unlockCamera) {
                this.renderer.unlockCamera();
            }
        }
    }

    /**
     * 启用/禁用自动锁定鼠标位置
     * @param {boolean} enabled
     */
    setMouseLock(enabled) {
        this._mouseLock = enabled;
        if (enabled) {
            this._mouseTracking = true;
        }
    }

    /**
     * 设置移动速度
     * @param {number} speed 单位/秒
     */
    setMoveSpeed(speed) {
        this._moveSpeedNormal = Math.max(0.1, speed);
    }

    /**
     * 每帧更新（在渲染循环中调用）
     * @param {number} deltaTime 帧间隔（秒）
     */
    tick(deltaTime) {
        // 1. 更新头部世界位置
        this._updateHeadPosition();

        // 2. 鼠标追踪：计算球面映射并设置朝向目标
        if (this._mouseTracking) {
            this._computeSphereTracking();
        }

        // 3. 鼠标锁定模式：持续将目标设为鼠标球面映射位置
        if (this._mouseLock && this._mouseTracking) {
            // 将球面映射点投影到地面作为移动目标
            this._target.copy(this._headWorldPos);
            // 使用 current Yaw 计算前方方向，沿该方向推进
            this._updateTargetRotation();
            this._markMove();
            if (this._onSetTarget) this._onSetTarget();
        }

        // 4. 移动到目标位置
        this._updateMovement(deltaTime);

        // 5. 朝向插值
        this._updateRotation(deltaTime);

        // 6. 同步 MoLang 变量（target_y_rotation 等供 BodyRotationInterpreter 读取）
        this._syncMolang();

        // 7. 应用位置到渲染器（朝向不再旋转 modelRoot，由动画骨骼驱动）
        this._applyPosition();

        // 8. 闲置检测：仅当发生过移动 + 5 秒无新位置变化时触发
        if (this._hasEverMoved && !this._idleTriggered
            && (performance.now() - this._lastMoveTime > this._idleThreshold)) {
            this._idleTriggered = true;
            if (this._onIdle) this._onIdle();
        }
    }

    // ==== 属性 ====

    /** @returns {{x: number, y: number, z: number}} */
    get currentPosition() {
        return { x: this._position.x, y: this._position.y, z: this._position.z };
    }

    /** @returns {{yaw: number, pitch: number}} */
    get currentRotation() {
        return { yaw: this._yaw, pitch: this._pitch };
    }

    /** @returns {boolean} */
    get isMoving() { return this._isMoving; }

    /** @returns {boolean} */
    get isFastMoving() { return this._isFastMoving; }

    /** @returns {boolean} */
    get mouseTracking() { return this._mouseTracking; }

    /** @returns {boolean} */
    get mouseLock() { return this._mouseLock; }

    /**
     * 设置位置变化回调
     * @param {(pos: {x,y,z}) => void} cb
     */
    onPositionChange(cb) { this._onPositionChange = cb; }

    /**
     * 设置朝向变化回调
     * @param {(rot: {yaw,pitch}) => void} cb
     */
    onRotationChange(cb) { this._onRotationChange = cb; }

    /**
     * 设置闲置回调（5 秒无操作时触发一次）
     * @param {() => void} cb
     */
    onIdle(cb) { this._onIdle = cb; }

    /**
     * 设置 setTarget 回调（移动开始时同步 body_y_rotation）
     * @param {() => void} cb
     */
    onSetTarget(cb) { this._onSetTarget = cb; }

    /**
     * 标记一次位置移动（重置闲置计时器）
     * @private
     */
    _markMove() {
        this._lastMoveTime = performance.now();
        this._idleTriggered = false;
        this._hasEverMoved = true;
    }

    // ==== 内部实现 ====

    /**
     * 更新移动（平滑移动到目标）
     * 基于当前剩余距离动态切换动画类型与速度：
     *   距离 > 30 单位 → fast_move 动画，45 单位/秒
     *   距离 ≤ 30 单位 → move 动画，30 单位/秒
     * @param {number} dt
     * @private
     */
    _updateMovement(dt) {
        const dx = this._target.x - this._position.x;
        const dy = this._target.y - this._position.y;
        const dz = this._target.z - this._position.z;
        const dist = Math.sqrt(dx * dx + dy * dy + dz * dz);

        if (dist < this._arrivalDistance) {
            // 到达目标：关闭移动状态，触发动画淡出
            if (this._isMoving) {
                this._position.copy(this._target);
                this._isMoving = false;
                this._isFastMoving = false;
                this._notifyMoveState();
            }
            return;
        }

        // 基于当前剩余距离决定移动速度与动画类型
        const isFast = dist > this._distanceThreshold;
        const speed = isFast ? this._moveSpeedFast : this._moveSpeedNormal;
        const moveDist = Math.min(dist, speed * dt);

        // 归一化方向并移动
        const invDist = 1 / dist;
        this._position.x += dx * invDist * moveDist;
        this._position.y += dy * invDist * moveDist;
        this._position.z += dz * invDist * moveDist;

        // 碰撞检测：Y 不低于地板
        if (this._position.y < this._floorY) {
            this._position.y = this._floorY;
        }

        // 更新移动状态并通知动画系统
        const wasMoving = this._isMoving;
        const wasFast = this._isFastMoving;
        this._isMoving = true;
        this._isFastMoving = isFast;

        if (wasMoving !== this._isMoving || wasFast !== this._isFastMoving) {
            this._notifyMoveState();
        }

        // 标记位置变化（重置闲置计时器）
        this._markMove();
    }

    /**
     * 计算朝向目标方向的偏航和俯仰
     * @private
     */
    _updateTargetRotation() {
        const dx = this._target.x - this._position.x;
        const dz = this._target.z - this._position.z;
        const dy = this._target.y - this._position.y;

        // 仅在有水平位移时更新偏航
        if (Math.abs(dx) > 0.001 || Math.abs(dz) > 0.001) {
            // 模型正面朝 -Z（Blockbench/Three.js 约定），atan2(dx, dz) 给出相对 +Z 的角度
            // 加 180° 翻转，使模型正面朝向目标而非屁股朝向目标
            this._targetYaw = Math.atan2(dx, dz) * 180 / Math.PI + 180;
        }

        // 俯仰：基于垂直位移与水平距离的比值
        const horizontalDist = Math.sqrt(dx * dx + dz * dz);
        if (horizontalDist > 0.001) {
            this._targetPitch = Math.atan2(dy, horizontalDist) * 180 / Math.PI;
            // 限制俯仰范围
            this._targetPitch = Math.max(-10, Math.min(45, this._targetPitch));
        }
    }

    /**
     * 朝向平滑插值
     * @param {number} dt
     * @private
     */
    _updateRotation(dt) {
        // 偏航：处理 360° 环绕（取最短路径）
        let yawDiff = this._targetYaw - this._yaw;
        while (yawDiff > 180) yawDiff -= 360;
        while (yawDiff < -180) yawDiff += 360;

        const maxStep = this._rotationSpeed * dt;
        const yawStep = Math.abs(yawDiff) <= maxStep ? yawDiff : Math.sign(yawDiff) * maxStep;
        const pitchDiff = this._targetPitch - this._pitch;
        const pitchStep = Math.abs(pitchDiff) <= maxStep ? pitchDiff : Math.sign(pitchDiff) * maxStep;

        this._yaw += yawStep;
        this._pitch += pitchStep;

        // 归一化到 [-180, 180]
        while (this._yaw > 180) this._yaw -= 360;
        while (this._yaw < -180) this._yaw += 360;
    }

    /**
     * 同步 MoLang 变量
     * @private
     */
    _syncMolang() {
        // 计算地面速度：使用当前动画类型对应的速度
        const speed = this._isFastMoving ? this._moveSpeedFast : this._moveSpeedNormal;
        const groundSpeed = this._isMoving ? speed : 0;

        this.molang.updateContext({
            target_x_rotation: this._pitch,
            target_y_rotation: this._yaw,
            is_moving: this._isMoving ? 1 : 0,
            is_sprinting: this._isFastMoving ? 1 : 0,
            ground_speed: groundSpeed,
            move_speed: groundSpeed,
            is_on_ground: this._position.y <= this._floorY + 0.01 ? 1 : 0
        });
    }

    /**
     * 应用位置到渲染器
     * @private
     */
    _applyPosition() {
        if (this.renderer && this.renderer.modelRoot) {
            this.renderer.modelRoot.position.set(this._position.x, this._position.y, this._position.z);
        }
        if (this._onPositionChange) {
            this._onPositionChange(this.currentPosition);
        }
    }

    /**
     * 应用朝向变化回调（不再旋转 modelRoot；偏航由动画骨骼 whole 引用 q.body_y_rotation 驱动）
     * @private
     */
    _applyRotation() {
        if (this._onRotationChange) {
            this._onRotationChange(this.currentRotation);
        }
    }

    /**
     * 通知移动状态变化
     * @private
     */
    _notifyMoveState() {
        this.onMoveStateChange(this._isMoving, this._isFastMoving);
    }

    // ==== 鼠标事件 ====

    /**
     * 绑定鼠标事件（用于鼠标追踪）
     * @private
     */
    _bindMouseEvents() {
        const canvas = this.renderer?.canvas;
        if (!canvas) return;

        this._onMouseMove = this._onMouseMove.bind(this);
        window.addEventListener('mousemove', this._onMouseMove);
    }

    /**
     * 鼠标移动处理 — 仅更新 NDC 坐标，球面计算在 tick() 中每帧执行
     * @param {MouseEvent} e
     * @private
     */
    _onMouseMove(e) {
        const canvas = this.renderer?.canvas;
        if (!canvas) return;

        const rect = canvas.getBoundingClientRect();
        this._mouseNDC.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
        this._mouseNDC.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;
    }

    /**
     * 更新头部世界位置（从 renderer 获取 headCheek 骨骼的全局位置）
     * @private
     */
    _updateHeadPosition() {
        if (this.renderer?.getHeadWorldPosition) {
            const pos = this.renderer.getHeadWorldPosition();
            if (pos) this._headWorldPos.copy(pos);
        }
    }

    /**
     * 球面映射追踪 — 以头部为圆心、半径 10 构建球面，将鼠标映射到球面上
     * 计算偏航与俯仰，平滑过渡 target_yaw / target_pitch
     * @private
     */
    _computeSphereTracking() {
        const camera = this.renderer?.camera;
        if (!camera) return;

        // 从相机通过鼠标 NDC 发出射线
        this._raycaster.setFromCamera(this._mouseNDC, camera);
        const ray = this._raycaster.ray;

        // 射线与球面求交（球心 = 头部位置，半径 = 10）
        // 参考: https://www.scratchapixel.com/lessons/3d-basic-rendering/minimal-ray-tracer-rendering-simple-shapes/ray-sphere-intersection
        const L = ray.origin.clone().sub(this._headWorldPos);
        const a = ray.direction.dot(ray.direction); // 应为 1
        const b = 2 * ray.direction.dot(L);
        const c = L.dot(L) - this._mouseSphereRadius * this._mouseSphereRadius;
        const discriminant = b * b - 4 * a * c;

        let hitPoint;
        if (discriminant >= 0) {
            // 射线与球面相交，取最近的交点
            const t = (-b - Math.sqrt(discriminant)) / (2 * a);
            if (t > 0) {
                hitPoint = ray.origin.clone().addScaledVector(ray.direction, t);
            } else {
                // 交点在相机后方，回退到最近点
                const tNear = (-b + Math.sqrt(discriminant)) / (2 * a);
                hitPoint = ray.origin.clone().addScaledVector(ray.direction, tNear);
            }
        } else {
            // 射线未命中球面：取球面上离射线最近的点
            const tCa = -ray.direction.dot(L) / a;
            const closestOnRay = ray.origin.clone().addScaledVector(ray.direction, tCa);
            const dirToSphere = this._headWorldPos.clone().sub(closestOnRay);
            const distToSphere = dirToSphere.length();
            if (distToSphere < 0.001) {
                // 射线穿过球心，取球面上方一点
                hitPoint = this._headWorldPos.clone().add(
                    new THREE.Vector3(0, this._mouseSphereRadius, 0)
                );
            } else {
                dirToSphere.normalize();
                hitPoint = this._headWorldPos.clone().addScaledVector(dirToSphere, -this._mouseSphereRadius);
            }
        }

        // 从头部到命中点的方向向量
        const dir = hitPoint.clone().sub(this._headWorldPos);
        const dist = dir.length();
        if (dist < 0.001) return;

        dir.normalize();

        // 计算偏航（水平角）：模型正面朝 -Z，加 180° 翻转
        // yaw = atan2(dir.x, -dir.z) → 标准 atan2(X, Z) 的朝向
        // 但模型正面朝 -Z，所以要加 180°
        let targetYaw = Math.atan2(dir.x, dir.z) * 180 / Math.PI + 180;

        // d. 偏航限制在 [-45°, 45°] 范围内（鼠标追踪模式下）
        // 规范到 [-180, 180] 再截断
        while (targetYaw > 180) targetYaw -= 360;
        while (targetYaw < -180) targetYaw += 360;
        targetYaw = Math.max(-45, Math.min(45, targetYaw));

        // 计算俯仰（垂直角）
        const horizontalDist = Math.sqrt(dir.x * dir.x + dir.z * dir.z);
        let targetPitch = Math.atan2(dir.y, horizontalDist) * 180 / Math.PI;
        targetPitch = Math.max(-10, Math.min(45, targetPitch));

        // 平滑过渡 target_yaw / target_pitch（使用较慢的鼠标追踪速度）
        let yawDiff = targetYaw - this._targetYaw;
        while (yawDiff > 180) yawDiff -= 360;
        while (yawDiff < -180) yawDiff += 360;

        const maxStep = this._mouseRotationSpeed * (1 / 60); // 每帧最大步长
        const yawStep = Math.abs(yawDiff) <= maxStep ? yawDiff : Math.sign(yawDiff) * maxStep;
        const pitchDiff = targetPitch - this._targetPitch;
        const pitchStep = Math.abs(pitchDiff) <= maxStep ? pitchDiff : Math.sign(pitchDiff) * maxStep;

        this._targetYaw += yawStep;
        this._targetPitch += pitchStep;
    }

    /**
     * 清理（移除事件监听）
     */
    dispose() {
        window.removeEventListener('mousemove', this._onMouseMove);
    }
}
