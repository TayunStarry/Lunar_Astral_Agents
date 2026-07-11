// ==== character-physics.js — 角色物理桥接 ====
//
// 职责（简化版）：
//   - 创建 CANNON.Body 碰撞体（fixedRotation=true）
//   - 落地检测（world.contacts 遍历）
//   - 潜行碰撞箱切换
//   - 调试线框渲染
//   - 位置同步由 movementController 通过 _syncPhysicsBody 驱动
//
// 不再负责：
//   - 移动力/速度追踪（由 movementController 直接写 position）
//   - 跳跃触发（由 movementController 通过 _body.velocity.y 直接设置）

import * as THREE from '../vendor/three.module.js';
import * as CANNON from '../vendor/cannon-es.module.js';

class CharacterPhysics {
    constructor({ renderer, physicsManager, molang, gravity = 9.82 }) {
        this.renderer = renderer;
        this.physicsManager = physicsManager;
        this.molang = molang;
        this._gravity = gravity;

        this._modelRoot = null;
        this._body = null;

        // AABB
        this._boxSize = { width: 1, height: 2, depth: 1 };
        this._normalShape = null;
        this._sneakShape = null;

        // 状态
        this._attached = false;
        this._isGrounded = false;
        this._isSneaking = false;

        // 低摩擦材质
        this._charMaterial = new CANNON.Material('character');

        // 调试线框
        this._debugMesh = null;

        // 动画引用
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

        const half = this._boxSize;
        const halfXZ = Math.max(half.width, half.depth) / 2;
        this._normalShape = new CANNON.Box(new CANNON.Vec3(halfXZ, half.height / 2, halfXZ));
        this._sneakShape = new CANNON.Box(new CANNON.Vec3(halfXZ, half.height * 0.425, halfXZ));

        this._body = new CANNON.Body({
            mass: 5,
            shape: this._normalShape,
            position: new CANNON.Vec3(
                modelRoot.position.x,
                modelRoot.position.y + half.height / 2,
                modelRoot.position.z
            ),
            material: this._charMaterial,
            fixedRotation: true,
            linearDamping: 0.05,
        });
        this._body.updateMassProperties();

        this.physicsManager.world.addBody(this._body);

        // 低摩擦接触材质
        if (this.physicsManager._groundBody?.material) {
            this.physicsManager.world.addContactMaterial(new CANNON.ContactMaterial(
                this._charMaterial, this.physicsManager._groundBody.material,
                { friction: 0.05, restitution: 0.0 }
            ));
        }
        const defaultMat = new CANNON.Material('default');
        this.physicsManager.world.addContactMaterial(new CANNON.ContactMaterial(
            this._charMaterial, defaultMat,
            { friction: 0.1, restitution: 0.0 }
        ));

        this._registerDebugMesh();
        this._attached = true;

        console.log(`[CharacterPhysics] 附体完成，AABB: ${JSON.stringify(this._boxSize)}`);
    }

    _computeStaticAABB(modelRoot) {
        // 优先使用 'body' 骨骼的包围盒，而非整个模型
        const bodyBone = modelRoot.getObjectByName('body');

        if (bodyBone) {
            const box = new THREE.Box3();
            const tempBox = new THREE.Box3();
            let hasMesh = false;

            bodyBone.traverse(obj => {
                if (obj.isMesh && obj.geometry) {
                    if (!obj.geometry.boundingBox) obj.geometry.computeBoundingBox();
                    tempBox.copy(obj.geometry.boundingBox).applyMatrix4(obj.matrixWorld);
                    box.union(tempBox);
                    hasMesh = true;
                }
            });

            if (hasMesh) {
                const size = new THREE.Vector3();
                box.getSize(size);
                this._boxSize = {
                    width: Math.max(size.x, 0.2),
                    height: Math.max(size.y, 0.2),
                    depth: Math.max(size.z, 0.2),
                };
                console.log(`[CharacterPhysics] body骨骼 AABB: ${JSON.stringify(this._boxSize)}`);
                return;
            }
        }

        // 回退：遍历整个模型
        const box = new THREE.Box3();
        const tempBox = new THREE.Box3();
        let hasMesh = false;

        modelRoot.traverse(obj => {
            if (obj.isMesh && obj.geometry) {
                if (!obj.geometry.boundingBox) obj.geometry.computeBoundingBox();
                tempBox.copy(obj.geometry.boundingBox).applyMatrix4(obj.matrixWorld);
                box.union(tempBox);
                hasMesh = true;
            }
        });

        if (!hasMesh) {
            this._boxSize = { width: 1, height: 2, depth: 1 };
            return;
        }

        const size = new THREE.Vector3();
        box.getSize(size);
        this._boxSize = {
            width: Math.max(size.x, 0.2),
            height: Math.max(size.y, 0.2),
            depth: Math.max(size.z, 0.2),
        };
    }

    // ==== 调试线框 ====

    _registerDebugMesh() {
        if (!this._body || !this.physicsManager._debugGroup) return;
        this._createDebugMesh();
    }

    _createDebugMesh() {
        if (!this._body) return;
        const pm = this.physicsManager;
        if (!pm._debugGroup) return;

        this._removeDebugMesh();

        const group = new THREE.Group();
        const material = new THREE.MeshBasicMaterial({
            color: 0x00ff88,
            wireframe: true,
            transparent: true,
            opacity: 0.6,
            depthTest: true,
        });

        for (let i = 0; i < this._body.shapes.length; i++) {
            const shape = this._body.shapes[i];
            if (shape.type === CANNON.Shape.types.BOX) {
                const geo = new THREE.BoxGeometry(
                    shape.halfExtents.x * 2,
                    shape.halfExtents.y * 2,
                    shape.halfExtents.z * 2
                );
                const mesh = new THREE.Mesh(geo, material);
                const offset = this._body.shapeOffsets[i];
                const orient = this._body.shapeOrientations[i];
                mesh.position.set(offset.x, offset.y, offset.z);
                mesh.quaternion.set(orient.x, orient.y, orient.z, orient.w);
                group.add(mesh);
            }
        }

        group.position.copy(this._body.position);
        group.quaternion.copy(this._body.quaternion);
        group.visible = pm._debugVisible;
        pm._debugGroup.add(group);
        this._debugMesh = group;
    }

    _removeDebugMesh() {
        if (this._debugMesh) {
            this.physicsManager._debugGroup?.remove(this._debugMesh);
            this._debugMesh.traverse(child => {
                if (child.geometry) child.geometry.dispose();
            });
            this._debugMesh = null;
        }
    }

    _updateDebugMesh() {
        if (!this._debugMesh || !this._body) return;
        this._debugMesh.position.copy(this._body.position);
        this._debugMesh.quaternion.copy(this._body.quaternion);
    }

    setDebugVisible(visible) {
        if (visible && !this._debugMesh) {
            // 确保调试组存在（Z 键可能先于角色附体触发）
            if (!this.physicsManager._debugGroup) {
                this.physicsManager._debugGroup = new THREE.Group();
                this.physicsManager._debugGroup.name = 'physics-debug';
                this.physicsManager.renderer.scene.add(this.physicsManager._debugGroup);
            }
            this._createDebugMesh();
        }
        if (this._debugMesh) this._debugMesh.visible = visible;
    }

    // ==== 每帧调用 ====

    prePhysicsStep(dt) {
        // 仅更新接地状态，力驱动已移除
        this._updateGrounded();
    }

    syncToModel() {
        if (!this._body || !this._modelRoot) return;
        // 位置同步由 movementController 统一驱动（跳跃用数学抛物线，非跳跃用 _syncPhysicsBody）
        this._updateDebugMesh();

        this.molang?.updateContext?.({
            is_on_ground: this._isGrounded ? 1 : 0,
            ground_speed: this._getHorizontalSpeed(),
            is_sneaking: this._isSneaking ? 1 : 0,
        });
    }

    // ==== 潜行 ====

    setSneak(sneak) {
        if (!this._body || this._isSneaking === sneak) return;
        this._isSneaking = sneak;

        const oldShape = this._body.shapes[0];
        if (sneak) {
            this._body.removeShape(oldShape);
            this._body.addShape(this._sneakShape);
        } else {
            this._body.removeShape(oldShape);
            this._body.addShape(this._normalShape);
        }

        this._notifySneakState(sneak);
    }

    stopMove() {
        this._isSneaking = false;
        this._notifySneakState(false);
    }

    // ==== 落地检测 ====

    _updateGrounded() {
        if (!this._body) {
            this._isGrounded = false;
            return;
        }

        let grounded = false;
        for (const c of this.physicsManager.world.contacts) {
            if (c.bi === this._body && c.ni.y < -0.3) { grounded = true; break; }
            if (c.bj === this._body && c.ni.y > 0.3) { grounded = true; break; }
        }
        this._isGrounded = grounded;
    }

    // ==== 工具 ====

    getPosition() {
        if (!this._modelRoot) return null;
        return this._modelRoot.position;
    }

    getBoxHeight() { return this._boxSize.height; }
    get isGrounded() { return this._isGrounded; }
    get isSneaking() { return this._isSneaking; }

    setFixedRotation(enabled) {
        if (!this._body) return;
        this._body.fixedRotation = enabled;
        this._body.updateMassProperties();
    }

    _getHorizontalSpeed() {
        if (!this._body) return 0;
        return Math.sqrt(this._body.velocity.x ** 2 + this._body.velocity.z ** 2);
    }

    _notifySneakState(isSneaking) {
        this.molang?.updateContext?.({ is_sneaking: isSneaking ? 1 : 0 });
        this._specialAnimRuntime?.setSneaking?.(isSneaking);
    }

    detach() {
        this._removeDebugMesh();
        if (this._body) {
            this.physicsManager.world.removeBody(this._body);
            this._body = null;
        }
        this._attached = false;
        this._modelRoot = null;
    }
}

export { CharacterPhysics };
