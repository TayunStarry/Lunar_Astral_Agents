// ==== character-physics.js — 角色物理（无碰撞体） ====
//
// 月岩引擎角色不再使用物理碰撞包围体（CANNON.Body），彻底避免角色被物理体顶起。
// 职责：
//   - 独立实现跳跃 / 下落（重力积分 + 地面判定）
//   - 当角色靠近动态物理体时，按角色的朝向与速度对附近体块施加"碰撞推力"（推挤）
//   - 更新 MoLang 上下文（is_on_ground / is_jumping / ground_speed）
//
// 推挤对"鼠标点击移动"与"第三人称键盘操作"均生效：二者都直接驱动 modelRoot 水平位置，
// 本模块根据 modelRoot 的位移速度统一对附近的 DYNAMIC 刚体施力。

import * as THREE from '../vendor/three.module.js';
import * as CANNON from '../vendor/cannon-es.module.js';

class CharacterPhysics {
    /**
     * @param {{renderer: object, physicsManager: object, molang: object, gravity?: number, mass?: number}} deps
     */
    constructor({ renderer, physicsManager, molang, gravity = 98.2, mass = 100 }) {
        this.renderer = renderer;
        this.physicsManager = physicsManager;
        this.molang = molang;
        this._gravity = Math.abs(gravity);
        this._mass = mass; // 角色质量：仅用于推挤力度参考

        this._modelRoot = null;
        /** 脚底高度（世界 Y，由本模块管理） */
        this._y = 0;
        /** 垂直速度 */
        this._velocityY = 0;
        /** 是否接地 */
        this._grounded = true;
        /** 上一帧水平位置（用于求移动速度并推挤） */
        this._lastX = 0;
        this._lastZ = 0;

        this._boxSize = { width: 1, height: 2, depth: 1 };
        this._isSneaking = false;

        // 推挤参数：作用于角色前方的"包围盒"范围（大于模型本体）
        this.pushMinSpeed = 2;      // 低于该水平速度不推
        this.pushHalfWidth = 7;     // 包围盒半宽（左右各延伸，大于模型足迹）
        this.pushDepth = 16;        // 包围盒前方深度
        this.pushSpeed = 60;        // 体块被推达到的目标速度（相对原力度已翻倍）
        this.pushBlend = 0.5;       // 体块速度向目标趋近的速率（0~1，越大推得越干脆）

        // 跳跃
        this.jumpCooldown = 0.3;
        this._lastJumpTime = 0;

        // 动画 / 回调
        this._specialAnimRuntime = null;
        this._onJumpFailed = null;
    }

    setSpecialAnimRuntime(sar) { this._specialAnimRuntime = sar; }
    setOnJumpFailed(cb) { this._onJumpFailed = cb; }

    // ==== 附体 ====

    attachToModel(modelRoot) {
        if (!modelRoot) return;
        this._modelRoot = modelRoot;
        this._computeStaticAABB(modelRoot);
        this._y = modelRoot.position.y;
        this._velocityY = 0;
        this._grounded = true;
        this._lastX = modelRoot.position.x;
        this._lastZ = modelRoot.position.z;
        console.log(`[CharacterPhysics] 无碰撞体附体完成，高度 ${this._boxSize.height.toFixed(1)}`);
    }

    _computeStaticAABB(modelRoot) {
        // 优先取 'body' 骨骼，否则整模型
        const bodyBone = modelRoot.getObjectByName('body');
        const box = new THREE.Box3();
        const tempBox = new THREE.Box3();
        let hasMesh = false;
        const scan = (root) => root.traverse(obj => {
            if (obj.isMesh && obj.geometry) {
                if (!obj.geometry.boundingBox) obj.geometry.computeBoundingBox();
                tempBox.copy(obj.geometry.boundingBox).applyMatrix4(obj.matrixWorld);
                box.union(tempBox);
                hasMesh = true;
            }
        });
        if (bodyBone) scan(bodyBone);
        if (!hasMesh) scan(modelRoot);
        const size = new THREE.Vector3();
        if (hasMesh) box.getSize(size);
        this._boxSize = {
            width: Math.max(size.x, 0.2) || 1,
            height: Math.max(size.y, 0.2) || 2,
            depth: Math.max(size.z, 0.2) || 1,
        };
    }

    detach() {
        this._modelRoot = null;
        this._grounded = false;
        this._velocityY = 0;
    }

    // ==== 独立跳跃 / 下落 ====

    /** 触发跳跃（需接地 + 冷却） */
    startJump(velocity) {
        const now = performance.now() / 1000;
        if (!this._grounded) return false;
        if (now - this._lastJumpTime < this.jumpCooldown) return false;
        this._velocityY = Math.max(velocity, 0);
        this._grounded = false;
        this._lastJumpTime = now;
        this.molang?.updateContext?.({ is_jumping: 1 });
        this._specialAnimRuntime?.setJumping?.(true);
        return true;
    }

    // ==== 每帧（在物理步进前调用） ====

    prePhysicsStep(dt) {
        const root = this._modelRoot;
        if (!root || dt <= 0) return;

        // 1. 重力积分（竖直方向独立管理，不受物理碰撞影响）
        const wasGrounded = this._grounded;
        this._velocityY -= this._gravity * dt;
        this._y += this._velocityY * dt;
        const gY = this.physicsManager?.groundY ?? 0;
        if (this._y <= gY) {
            this._y = gY;
            this._velocityY = 0;
            this._grounded = true;
        } else if (Math.abs(this._y - gY) < 0.5 && this._velocityY <= 0) {
            // 贴近地面微抖 → 贴平
            this._y = gY;
            this._velocityY = 0;
            this._grounded = true;
        } else {
            this._grounded = false;
        }
        root.position.y = this._y;

        // 状态机：不在地上 → 进入跳跃状态；落地 → 回归行走/奔跑
        if (this._grounded !== wasGrounded) {
            this._specialAnimRuntime?.setJumping?.(this._grounded ? false : true);
            this.molang?.updateContext?.({ is_jumping: this._grounded ? 0 : 1 });
        }
        this.molang?.updateContext?.({ is_on_ground: this._grounded ? 1 : 0 });

        // 2. 靠近物理体 → 按朝向/速度对 DYNAMIC 刚体施加推力
        this._applyPush(dt);

        // 3. 记录水平位置（供下一帧求速度）
        this._lastX = root.position.x;
        this._lastZ = root.position.z;
    }

    /** 基于角色移动，对前方"包围盒"范围内的 DYNAMIC 刚体施力（同时兼容点击移动与键盘移动） */
    _applyPush(dt) {
        const root = this._modelRoot;
        const pm = this.physicsManager;
        if (!root || !pm) return;

        const vx = (root.position.x - this._lastX) / dt;
        const vz = (root.position.z - this._lastZ) / dt;
        const speed = Math.hypot(vx, vz);
        if (speed < this.pushMinSpeed) return;
        // 前进 / 右向基（按移动方向）
        const fx = vx / speed, fz = vz / speed;
        const rx = -fz, rz = fx; // 右向（XZ 平面）

        for (const body of pm.bodies.values()) {
            if (body.type !== CANNON.Body.DYNAMIC) continue;
            const dx = body.position.x - root.position.x;
            const dz = body.position.z - root.position.z;
            // 投影到移动方向坐标系：lx=左右，lz=前后
            const lx = dx * rx + dz * rz;
            const lz = dx * fx + dz * fz;
            // 落在角色前方的"包围盒"内（半宽更宽、向前延伸，避免仅中轴命中才触发）
            if (Math.abs(lx) > this.pushHalfWidth) continue;
            if (lz < 0 || lz > this.pushDepth) continue;

            // 越近推得越强；体块越重越抗推
            const depthFrac = 1 - (lz / this.pushDepth);
            const massResist = this._mass / (this._mass + body.mass * 0.2);
            const target = this.pushSpeed * (0.3 + 0.7 * depthFrac) * massResist;
            // 沿前进方向拨动体块速度（推力已放大）
            body.velocity.x += (fx * target - body.velocity.x) * this.pushBlend;
            body.velocity.z += (fz * target - body.velocity.z) * this.pushBlend;
        }
    }

    // ==== 同步 / 状态 ====

    syncToModel() {
        // 竖直位置已在 prePhysicsStep 直接写到 modelRoot，此处仅刷新 molang 速度
        if (!this._modelRoot) return;
        this.molang?.updateContext?.({
            is_on_ground: this._grounded ? 1 : 0,
            ground_speed: this._getHorizontalSpeed(),
            is_sneaking: this._isSneaking ? 1 : 0,
        });
    }

    setSneak(sneak) {
        if (this._isSneaking === sneak) return;
        this._isSneaking = sneak;
        this._specialAnimRuntime?.setSneaking?.(sneak);
        this.molang?.updateContext?.({ is_sneaking: sneak ? 1 : 0 });
    }

    stopMove() {
        this._isSneaking = false;
        this._specialAnimRuntime?.setSneaking?.(false);
    }

    /** 瞬移放置（供 setPosition 等调用） */
    placeAt(x, y, z) {
        const root = this._modelRoot;
        if (root) {
            root.position.set(x, y, z);
        }
        this._y = y;
        this._velocityY = 0;
        this._grounded = true;
        this._lastX = x;
        this._lastZ = z;
    }

    _getHorizontalSpeed() {
        if (!this._modelRoot) return 0;
        const vx = this._modelRoot.position.x - this._lastX;
        const vz = this._modelRoot.position.z - this._lastZ;
        return Math.sqrt(vx * vx + vz * vz);
    }

    // ==== 属性与参数 ====

    getPosition() {
        return this._modelRoot ? this._modelRoot.position : null;
    }
    getBoxHeight() { return this._boxSize.height; }
    get isGrounded() { return this._grounded; }
    get isSneaking() { return this._isSneaking; }
    /** 当前脚底高度 */
    get groundY() { return this._y; }

    /** 无碰撞体，固定旋转无意义（保留兼容） */
    setFixedRotation() {}

    setGravity(g) {
        if (typeof g === 'number') this._gravity = Math.abs(g);
    }
    getMass() { return this._mass; }
    setMass(m) {
        if (typeof m === 'number' && m > 0) this._mass = m;
    }
}

export { CharacterPhysics };