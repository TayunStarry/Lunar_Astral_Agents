// ==== movement-controller.js — 模型移动控制系统 ====
//
// 职责：
//   - 平滑移动模型到目标坐标（基于 modelRoot.position）
//   - 朝向控制：计算并设置 q.target_x_rotation / q.target_y_rotation
//   - 碰撞检测：禁止穿过地板（Y >= 0）及边界
//   - 鼠标追踪：将鼠标位置投影到地面，作为朝向目标
//   - 自动锁定：持续锁定目标为鼠标位置
//   - 同步 MoLang 变量：is_moving / ground_speed / move_speed / target_*_rotation
//   - 通知 SpecialAnimationRuntime 移动状态变化

import * as THREE from '../vendor/three.module.js';

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
        /** @type {number} 普通移动速度（单位/秒） */
        this._moveSpeed = 5;
        /** @type {number} 快速移动速度阈值 */
        this._fastMoveThreshold = 10;
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

        // ==== 鼠标追踪 ====
        /** @type {boolean} 鼠标追踪是否启用 */
        this._mouseTracking = false;
        /** @type {boolean} 自动锁定鼠标位置 */
        this._mouseLock = false;
        /** @type {THREE.Vector3} 鼠标在世界地面的投影点 */
        this._mouseGroundPoint = new THREE.Vector3();
        /** @type {THREE.Raycaster} */
        this._raycaster = new THREE.Raycaster();
        /** @type {THREE.Vector2} 归一化鼠标坐标 */
        this._mouseNDC = new THREE.Vector2();
        /** @type {THREE.Plane} 地面平面（Y=0） */
        this._groundPlane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);

        // ==== 碰撞边界 ====
        /** @type {number} 地板高度（Y 最小值） */
        this._floorY = 0;
        /** @type {number} 边界范围（正方形区域 [-bound, bound]） */
        this._bound = 100;

        // ==== 回调 ====
        this._onPositionChange = null;
        this._onRotationChange = null;

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
     * @param {boolean} enabled
     */
    setMouseTracking(enabled) {
        this._mouseTracking = enabled;
        if (!enabled) {
            this._mouseLock = false;
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
        this._moveSpeed = Math.max(0.1, speed);
    }

    /**
     * 每帧更新（在渲染循环中调用）
     * @param {number} deltaTime 帧间隔（秒）
     */
    tick(deltaTime) {
        // 1. 鼠标锁定模式：持续将目标设为鼠标位置
        if (this._mouseLock && this._mouseTracking) {
            this._target.copy(this._mouseGroundPoint);
            this._target.y = Math.max(this._floorY, this._target.y);
            this._updateTargetRotation();
        }

        // 2. 移动到目标位置
        this._updateMovement(deltaTime);

        // 3. 朝向插值
        this._updateRotation(deltaTime);

        // 4. 同步 MoLang 变量
        this._syncMolang();

        // 5. 应用到渲染器
        this._applyPosition();
        this._applyRotation();
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

    // ==== 内部实现 ====

    /**
     * 更新移动（平滑移动到目标）
     * @param {number} dt
     * @private
     */
    _updateMovement(dt) {
        const dx = this._target.x - this._position.x;
        const dy = this._target.y - this._position.y;
        const dz = this._target.z - this._position.z;
        const dist = Math.sqrt(dx * dx + dy * dy + dz * dz);

        if (dist < this._arrivalDistance) {
            // 到达目标
            if (this._isMoving) {
                this._position.copy(this._target);
                this._isMoving = false;
                this._isFastMoving = false;
                this._notifyMoveState();
            }
            return;
        }

        // 计算移动速度（基于距离自适应：远距离全速，近距离减速）
        const speed = this._moveSpeed;
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

        // 更新移动状态
        const wasMoving = this._isMoving;
        const wasFast = this._isFastMoving;
        this._isMoving = true;
        // 快速移动判定：速度超过阈值 或 距离很远
        this._isFastMoving = speed >= this._fastMoveThreshold || dist > this._fastMoveThreshold * 2;

        if (wasMoving !== this._isMoving || wasFast !== this._isFastMoving) {
            this._notifyMoveState();
        }
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
            this._targetPitch = Math.max(-89, Math.min(89, this._targetPitch));
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
     *
     * target_y_rotation 刻意保持 0：偏航已由 modelRoot.rotation.y 直接控制，
     * 若再传入动画会让 whole 骨骼二次旋转，叠加产生 roll（Z 轴倾斜）。
     * target_x_rotation 正常传递：俯仰由动画中的 whole/arms 等骨骼消费。
     *
     * @private
     */
    _syncMolang() {
        // 计算地面速度
        const dx = this._target.x - this._position.x;
        const dz = this._target.z - this._position.z;
        const dist = Math.sqrt(dx * dx + dz * dz);
        const groundSpeed = this._isMoving ? Math.min(this._moveSpeed, dist) : 0;

        this.molang.updateContext({
            target_x_rotation: this._pitch,
            target_y_rotation: 0,
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
     * 应用朝向到渲染器
     *
     * 偏航（yaw）：直接设置 modelRoot.rotation.y，持久生效，不受动画切换影响
     * 俯仰（pitch）：通过 MoLang 变量 q.target_x_rotation 传递给动画系统（whole/arms 等骨骼消费）
     *
     * 注意：target_y_rotation MoLang 变量刻意保持为 0，避免动画中 whole 骨骼的
     * q.target_y_rotation 旋转与 modelRoot.rotation.y 双重叠加，在 ZYX Euler 顺序下
     * 产生 roll 分量导致 Z 轴倾斜。
     *
     * @private
     */
    _applyRotation() {
        if (this.renderer && this.renderer.modelRoot) {
            this.renderer.modelRoot.rotation.y = this._yaw * Math.PI / 180;
        }
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
     * 鼠标移动处理
     * @param {MouseEvent} e
     * @private
     */
    _onMouseMove(e) {
        if (!this._mouseTracking) return;

        const canvas = this.renderer?.canvas;
        if (!canvas) return;

        // 计算归一化设备坐标
        const rect = canvas.getBoundingClientRect();
        this._mouseNDC.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
        this._mouseNDC.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;

        // 射线投射到地面平面
        this._raycaster.setFromCamera(this._mouseNDC, this.renderer.camera);
        const hitPoint = new THREE.Vector3();
        this._raycaster.ray.intersectPlane(this._groundPlane, hitPoint);

        if (hitPoint) {
            this._mouseGroundPoint.copy(hitPoint);
        }

        // 鼠标追踪模式（非锁定）：仅更新朝向，不移动
        if (this._mouseTracking && !this._mouseLock) {
            // 计算朝向鼠标方向（+180° 翻转，同 _updateTargetRotation）
            const dx = this._mouseGroundPoint.x - this._position.x;
            const dz = this._mouseGroundPoint.z - this._position.z;
            if (Math.abs(dx) > 0.001 || Math.abs(dz) > 0.001) {
                this._targetYaw = Math.atan2(dx, dz) * 180 / Math.PI + 180;
            }
        }
    }

    /**
     * 清理（移除事件监听）
     */
    dispose() {
        window.removeEventListener('mousemove', this._onMouseMove);
    }
}
