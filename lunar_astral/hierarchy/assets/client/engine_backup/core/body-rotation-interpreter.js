// ==== body-rotation-interpreter.js — 身体旋转追踪解释器 ====
//
// 职责：
//   - 读取 molang 的 target_y_rotation（目标偏航，由 MovementController 设置）
//   - 维护 body_y_rotation（身体当前偏航），逐帧逐渐趋近目标
//   - 追踪速度随角度差自适应：差越小越慢（二次曲线），避免小角度时抽搐
//   - 当头部与身体角度差 > 45° 时，强制加速身体旋转
//   - 直接操作 modelRoot.rotation.y 驱动身体旋转（不依赖动画系统）
//   - 直接操作 headCheek 骨骼 compensation 保持头部朝向目标（±90° 限制）
//   - 直接操作头发骨骼跟随头部俯仰
//
// 效果：设置目标朝向后，头部立即转向目标，身体则缓慢转动到面朝方向；
//   过程中头始终朝向目标，实现"先扭头再转身体"。

const DEG2RAD = Math.PI / 180;

/**
 * 身体旋转解释器
 *
 * 旋转策略（自适应速度曲线）：
 *   - 角度差 >  45°：快速追踪（forcedSpeed），身体强制追上头部
 *   - 角度差 ≤ 45°：速度随差值二次曲线递减（minSpeed → maxSpeed），
 *     差值越小越慢，避免身体旋转时的动画抽搐
 *
 * 角度差计算使用最短路径（处理 360° 环绕）
 * 头部补偿角度限制在 ±90° 以内，防止意外扭曲
 */
export class BodyRotationInterpreter {
    /**
     * @param {import('./molang-runtime.js').MolangRuntime} molang
     * @param {import('./renderer.js').Renderer} renderer
     * @param {import('./outliner.js').Outliner|null} outliner
     */
    constructor(molang, renderer, outliner) {
        this.molang = molang;
        this.renderer = renderer;
        this.outliner = outliner;

        /** @type {number} 身体当前偏航（度） */
        this._bodyYaw = 0;

        // ==== 骨骼引用（_findBones 填充） ====
        /** @type {import('./outliner.js').Bone|null} */
        this._headCheekBone = null;
        /** @type {import('./outliner.js').Bone|null} */
        this._rightLongHairBone = null;
        /** @type {import('./outliner.js').Bone|null} */
        this._leftLongHairBone = null;

        // ==== 参数 ====
        /** @type {number} 最小追踪速度（度/秒），角度差 → 0 时使用 */
        this.minSpeed = 5;
        /** @type {number} 正常追踪最大速度（度/秒），角度差 = 阈值时与 forcedSpeed 衔接 */
        this.maxSpeed = 360;
        /** @type {number} 强制追踪速度（度/秒，角度差 > 阈值时使用） */
        this.forcedSpeed = 360;
        /** @type {number} 触发强制追踪的角度阈值（度） */
        this.forceThreshold = 45;
        /** @type {number} 头部补偿角度上限（度），防止头部意外扭曲 */
        this._headYawMax = 90;
    }

    /**
     * 在模型加载后调用，查找骨骼引用并保存 rest pose 分量
     */
    findBones() {
        this._headCheekBone = null;
        this._rightLongHairBone = null;
        this._leftLongHairBone = null;
        if (!this.outliner) return;
        this.outliner.traverseBones(bone => {
            if (bone.name === 'headCheek') this._headCheekBone = bone;
            if (bone.name === 'RightLongHair') this._rightLongHairBone = bone;
            if (bone.name === 'LeftLongHair') this._leftLongHairBone = bone;
        });

        // 保存 rest pose 中不被覆盖的分量
        if (this._headCheekBone && this._headCheekBone.sceneObject) {
            this._headCheekRestZ = this._headCheekBone.sceneObject.rotation.z;
        }
        if (this._rightLongHairBone && this._rightLongHairBone.sceneObject) {
            this._rightHairRestY = this._rightLongHairBone.sceneObject.rotation.y;
            this._rightHairRestZ = this._rightLongHairBone.sceneObject.rotation.z;
        }
        if (this._leftLongHairBone && this._leftLongHairBone.sceneObject) {
            this._leftHairRestY = this._leftLongHairBone.sceneObject.rotation.y;
            this._leftHairRestZ = this._leftLongHairBone.sceneObject.rotation.z;
        }
    }

    /**
     * 每帧更新（在渲染循环中、MovementController.tick 之后调用）
     * @param {number} deltaTime 帧间隔（秒）
     */
    tick(deltaTime) {
        // 1. 更新身体偏航（逐渐趋近 target_y_rotation）
        const targetYaw = this.molang.query.target_y_rotation || 0;
        const diff = this._shortestAngleDiff(targetYaw, this._bodyYaw);
        const absDiff = Math.abs(diff);

        // 自适应速度：差值大 → 快，差值小 → 慢（二次曲线避免抽搐）
        let speed;
        if (absDiff > this.forceThreshold) {
            // > 45°：强制快速追踪
            speed = this.forcedSpeed;
        } else {
            // ≤ 45°：二次曲线，差值越小越慢
            // speed = minSpeed + (maxSpeed - minSpeed) * (absDiff / threshold)²
            const t = absDiff / this.forceThreshold;
            speed = this.minSpeed + (this.maxSpeed - this.minSpeed) * t * t;
        }
        const maxStep = speed * deltaTime;
        const step = absDiff <= maxStep ? diff : Math.sign(diff) * maxStep;

        this._bodyYaw += step;
        while (this._bodyYaw > 180) this._bodyYaw -= 360;
        while (this._bodyYaw < -180) this._bodyYaw += 360;

        // 2. 写回 molang
        this.molang.updateContext({ body_y_rotation: this._bodyYaw });

        // 3. 直接应用身体旋转到 modelRoot（不依赖动画系统）
        this._applyBodyRotation();

        // 4. 直接应用头部追踪（补偿身体旋转，使头始终朝向目标）
        this._applyHeadTracking();
    }

    /**
     * 立即同步身体偏航到目标（移动时调用，避免"屁股朝目标"）
     */
    syncToTarget() {
        this._bodyYaw = this.molang.query.target_y_rotation || 0;
        while (this._bodyYaw > 180) this._bodyYaw -= 360;
        while (this._bodyYaw < -180) this._bodyYaw += 360;
        this.molang.updateContext({ body_y_rotation: this._bodyYaw });
    }

    /**
     * 立即设置身体偏航
     * @param {number} yaw
     */
    setBodyYaw(yaw) {
        this._bodyYaw = yaw;
        while (this._bodyYaw > 180) this._bodyYaw -= 360;
        while (this._bodyYaw < -180) this._bodyYaw += 360;
        this.molang.updateContext({ body_y_rotation: this._bodyYaw });
    }

    /** @returns {number} 当前身体偏航（度） */
    get bodyYaw() { return this._bodyYaw; }

    // ==== 内部实现 ====

    /**
     * 应用身体旋转到 modelRoot（直接操作 Three.js，不依赖动画系统）
     * @private
     */
    _applyBodyRotation() {
        if (this.renderer && this.renderer.modelRoot) {
            this.renderer.modelRoot.rotation.y = this._bodyYaw * DEG2RAD;
        }
    }

    /**
     * 应用头部追踪：补偿身体旋转，使头始终朝向 target_y_rotation
     * 同时处理头发骨骼的俯仰跟随
     * @private
     */
    _applyHeadTracking() {
        const targetYaw = this.molang.query.target_y_rotation || 0;
        const targetPitch = this.molang.query.target_x_rotation || 0;
        const isGliding = this.molang.query.is_gliding || 0;

        // headCheek：补偿身体旋转（仅覆盖 X/Y，保留 rest pose Z）
        if (this._headCheekBone && this._headCheekBone.sceneObject) {
            const headX = (isGliding ? -75 : targetPitch) * DEG2RAD;
            // 头部补偿角度 = 目标角度 - 身体角度，限制在 ±90° 防止意外扭曲
            let headY = targetYaw - this._bodyYaw;
            headY = Math.max(-this._headYawMax, Math.min(this._headYawMax, headY));
            this._headCheekBone.sceneObject.rotation.set(
                headX, headY * DEG2RAD, this._headCheekRestZ ?? 0
            );
        }

        // 头发骨骼：跟随俯仰 + 飘动（仅覆盖 X，保留 rest pose Y/Z）
        if (this._rightLongHairBone && this._rightLongHairBone.sceneObject) {
            const hairX = this._evalHairRot(targetPitch, 100) * DEG2RAD;
            this._rightLongHairBone.sceneObject.rotation.set(
                hairX, this._rightHairRestY ?? 0, this._rightHairRestZ ?? 0
            );
        }

        if (this._leftLongHairBone && this._leftLongHairBone.sceneObject) {
            const hairX = this._evalHairRot(targetPitch, 125) * DEG2RAD;
            this._leftLongHairBone.sceneObject.rotation.set(
                hairX, this._leftHairRestY ?? 0, this._leftHairRestZ ?? 0
            );
        }
    }

    /**
     * 计算头发骨骼的 X 旋转值（度）
     * 等价于 MoLang：-q.target_x_rotation + (q.is_riding ? 0 : clamp(ground_speed,-1,3)*3) + cos(freq*life_time)*3
     * @param {number} targetPitch
     * @param {number} freq
     * @returns {number}
     * @private
     */
    _evalHairRot(targetPitch, freq) {
        const isRiding = this.molang.query.is_riding || 0;
        const groundSpeed = this.molang.query.ground_speed || 0;
        const lifeTime = this.molang.query.life_time || 0;
        const rideTerm = isRiding ? 0 : Math.max(-1, Math.min(3, groundSpeed)) * 3;
        return -targetPitch + rideTerm + Math.cos(freq * lifeTime) * 3;
    }

    /**
     * 计算最短角度差（target - current），结果在 [-180, 180]
     * @param {number} target
     * @param {number} current
     * @returns {number}
     * @private
     */
    _shortestAngleDiff(target, current) {
        let diff = (target - current) % 360;
        while (diff > 180) diff -= 360;
        while (diff < -180) diff += 360;
        return diff;
    }
}