import * as THREE from './three.module.js';
import * as CANNON from './cannon-es.module.js';

// ============ 物理引擎 ============
class PhysicsManager {
    constructor(sceneManager) {
        this.sm = sceneManager;
        this.world = new CANNON.World({
            gravity: new CANNON.Vec3(0, -9.82, 0),
        });
        // SAPBroadphase: O(n log n)，适合中型场景(30-100刚体)
        this.world.broadphase = new CANNON.SAPBroadphase(this.world);
        this.world.solver.iterations = 10;

        this.bodies = new Map(); // mesh.userData.id -> CANNON.Body
        this._meshById = new Map(); // mesh.userData.id -> THREE.Mesh (O(1) 查找)
        this.isActive = false;
        this._groundBody = null;
        this._groundY = -0.5;

        // 可调参数
        this._massSingle = 1;
        this._linearDamping = 0.1;
        this._angularDamping = 0.1;
    }

    get gravity() { return this.world.gravity.y; }
    set gravity(v) { this.world.gravity.y = v; }

    get groundY() { return this._groundY; }
    set groundY(v) {
        this._groundY = v;
        if (this._groundBody) {
            this._groundBody.position.set(0, v, 0);
        }
    }

    get massSingle() { return this._massSingle; }
    set massSingle(v) { this._massSingle = v; }

    get linearDamping() { return this._linearDamping; }
    set linearDamping(v) { this._linearDamping = v; }

    get angularDamping() { return this._angularDamping; }
    set angularDamping(v) { this._angularDamping = v; }

    _ensureGround() {
        if (this._groundBody) return;
        const groundShape = new CANNON.Plane();
        this._groundBody = new CANNON.Body({ mass: 0, shape: groundShape });
        this._groundBody.quaternion.setFromEuler(-Math.PI / 2, 0, 0);
        this._groundBody.position.set(0, this._groundY, 0);
        this.world.addBody(this._groundBody);
    }

    _removeGround() {
        if (this._groundBody) {
            this.world.removeBody(this._groundBody);
            this._groundBody = null;
        }
    }

    // ============ 碰撞体生成 ============

    /**
     * 根据图元类型生成对应的 CANNON 碰撞形状
     * 返回 [{ shape, offset, orientation }] 数组（Compound 用）
     */
    _createShapesForMesh(mesh) {
        const type = mesh.userData.type;
        const params = mesh.userData.params || {};
        const identityQuat = new CANNON.Quaternion();
        const zeroOffset = new CANNON.Vec3();

        switch (type) {
            case 'cube': {
                const w = Math.max((params.w || 1) / 2, 0.05);
                const h = Math.max((params.h || 1) / 2, 0.05);
                const d = Math.max((params.d || 1) / 2, 0.05);
                return [{ shape: new CANNON.Box(new CANNON.Vec3(w, h, d)), offset: zeroOffset, orientation: identityQuat }];
            }
            case 'sphere': {
                const r = Math.max(params.r || 0.5, 0.05);
                return [{ shape: new CANNON.Sphere(r), offset: zeroOffset, orientation: identityQuat }];
            }
            case 'cylinder': {
                const rt = Math.max(params.rt ?? 0.5, 0.01);
                const rb = Math.max(params.rb ?? 0.5, 0.01);
                const h = Math.max(params.h || 1, 0.1);
                const seg = Math.min(params.seg || 12, 24);
                // CANNON.Cylinder 轴向为 Y，与 THREE.CylinderGeometry 一致
                const shape = new CANNON.Cylinder(rt, rb, h, seg);
                return [{ shape, offset: zeroOffset, orientation: identityQuat }];
            }
            case 'cone': {
                const r = Math.max(params.r || 0.5, 0.05);
                const h = Math.max(params.h || 1, 0.1);
                const seg = Math.min(params.seg || 12, 24);
                const shape = new CANNON.Cylinder(0.001, r, h, seg);
                return [{ shape, offset: zeroOffset, orientation: identityQuat }];
            }
            case 'torus': {
                // 多段 Sphere 环形排列，中间留空洞
                return this._createTorusShapes(params);
            }
            case 'dodecahedron':
            case 'octahedron':
            case 'tetrahedron': {
                // detail=0 时顶点数少，可用 ConvexPolyhedron 精确表达
                const detail = params.detail || 0;
                if (detail === 0) {
                    const shape = this._createConvexFromGeometry(mesh.geometry);
                    if (shape) return [{ shape, offset: zeroOffset, orientation: identityQuat }];
                }
                // detail>0 顶点过多，降级为 Box
                return this._createBoxFromMesh(mesh);
            }
            case 'torusKnot': {
                // torusKnot 凹形复杂，凸包近似误差大，用 Box 兜底
                return this._createBoxFromMesh(mesh);
            }
            case 'ring':
            case 'plane': {
                // 薄 Box 近似
                const box = new THREE.Box3().setFromObject(mesh);
                const size = new THREE.Vector3(); box.getSize(size);
                const half = new CANNON.Vec3(
                    Math.max(size.x / 2, 0.05),
                    Math.max(size.y / 2, 0.02),
                    Math.max(size.z / 2, 0.05)
                );
                return [{ shape: new CANNON.Box(half), offset: zeroOffset, orientation: identityQuat }];
            }
            case 'group': {
                // 组合体：遍历子图元，生成 Compound shape
                return this._createGroupShapes(mesh);
            }
            default: {
                return this._createBoxFromMesh(mesh);
            }
        }
    }

    /**
     * 圆环碰撞体：N 段 Sphere 环形排列，留出中间空洞
     * 确保圆环可套入半径 < (R - t) 的圆柱
     */
    _createTorusShapes(params) {
        const R = Math.max(params.r || 0.5, 0.1);  // 大半径
        const t = Math.max(params.t || 0.2, 0.03); // 管半径
        // 计算段数：确保相邻 Sphere 重叠无缝隙
        const ratio = Math.min(t / R, 0.99);
        const minN = Math.ceil(Math.PI / Math.asin(ratio));
        const N = Math.max(6, Math.min(minN, 16));

        const shapes = [];
        for (let i = 0; i < N; i++) {
            const angle = (i / N) * Math.PI * 2;
            shapes.push({
                shape: new CANNON.Sphere(t),
                offset: new CANNON.Vec3(R * Math.cos(angle), 0, R * Math.sin(angle)),
                orientation: new CANNON.Quaternion(),
            });
        }
        return shapes;
    }

    /**
     * 组合体碰撞体：遍历子图元，各自生成形状并附带局部偏移
     */
    _createGroupShapes(groupMesh) {
        const shapes = [];
        for (const child of groupMesh.children) {
            const childShapes = this._createShapesForMesh(child);
            const childPos = new CANNON.Vec3(child.position.x, child.position.y, child.position.z);
            const childQuat = new CANNON.Quaternion(
                child.quaternion.x, child.quaternion.y, child.quaternion.z, child.quaternion.w
            );

            for (const cs of childShapes) {
                // 最终偏移 = 子图元位置 + (子图元旋转 * 子形状偏移)
                const rotatedOffset = new CANNON.Vec3();
                childQuat.vmult(cs.offset, rotatedOffset);
                const finalOffset = new CANNON.Vec3();
                childPos.vadd(rotatedOffset, finalOffset);

                // 最终朝向 = 子图元朝向 * 子形状朝向
                const finalOrientation = new CANNON.Quaternion();
                childQuat.mult(cs.orientation, finalOrientation);

                shapes.push({ shape: cs.shape, offset: finalOffset, orientation: finalOrientation });
            }
        }
        // 空组合体兜底
        if (shapes.length === 0) return this._createBoxFromMesh(groupMesh);
        return shapes;
    }

    /**
     * 从 THREE.BufferGeometry 创建 ConvexPolyhedron（用于凸多面体）
     */
    _createConvexFromGeometry(geometry) {
        const pos = geometry.attributes.position;
        const index = geometry.index;
        if (!pos) return null;

        // 顶点数过多时跳过（性能保护）
        if (pos.count > 100) return null;

        const vertices = [];
        const vertexMap = new Map();
        const tol = 0.0001;

        const getVertexIndex = (x, y, z) => {
            const key = `${Math.round(x / tol)},${Math.round(y / tol)},${Math.round(z / tol)}`;
            if (vertexMap.has(key)) return vertexMap.get(key);
            const idx = vertices.length;
            vertices.push(new CANNON.Vec3(x, y, z));
            vertexMap.set(key, idx);
            return idx;
        };

        const faces = [];
        const triCount = index ? index.count / 3 : pos.count / 3;
        for (let i = 0; i < triCount; i++) {
            const i0 = index ? index.getX(i * 3) : i * 3;
            const i1 = index ? index.getX(i * 3 + 1) : i * 3 + 1;
            const i2 = index ? index.getX(i * 3 + 2) : i * 3 + 2;
            const a = getVertexIndex(pos.getX(i0), pos.getY(i0), pos.getZ(i0));
            const b = getVertexIndex(pos.getX(i1), pos.getY(i1), pos.getZ(i1));
            const c = getVertexIndex(pos.getX(i2), pos.getY(i2), pos.getZ(i2));
            if (a !== b && b !== c && a !== c) faces.push([a, b, c]);
        }

        if (vertices.length < 4 || faces.length < 4) return null;
        return new CANNON.ConvexPolyhedron({ vertices, faces });
    }

    /**
     * 从网格包围盒创建 Box 碰撞体（兜底方案）
     */
    _createBoxFromMesh(mesh) {
        const box = new THREE.Box3().setFromObject(mesh);
        const size = new THREE.Vector3(); box.getSize(size);
        const half = new CANNON.Vec3(
            Math.max(size.x / 2, 0.05),
            Math.max(size.y / 2, 0.05),
            Math.max(size.z / 2, 0.05)
        );
        return [{ shape: new CANNON.Box(half), offset: new CANNON.Vec3(), orientation: new CANNON.Quaternion() }];
    }

    // ============ 刚体创建 ============

    createBody(mesh) {
        if (this.bodies.has(mesh.userData.id)) return null;
        const phys = mesh.userData.physics || {};
        const isAnchored = phys.anchored;
        const hasAutoRotate = phys.autoRotate?.enabled;

        // 生成碰撞形状
        const shapes = this._createShapesForMesh(mesh);

        // 确定刚体类型
        let bodyType, mass;
        if (isAnchored) {
            bodyType = CANNON.Body.STATIC;
            mass = 0;
        } else if (hasAutoRotate) {
            // 自动旋转 → KINEMATIC：不受力但能推动动态物体，通过 angularVelocity 旋转
            bodyType = CANNON.Body.KINEMATIC;
            mass = 0;
        } else {
            bodyType = CANNON.Body.DYNAMIC;
            const childCount = mesh.userData.type === 'group' ? mesh.children.length : 1;
            mass = childCount * this._massSingle;
        }

        const body = new CANNON.Body({
            mass,
            type: bodyType,
            position: new CANNON.Vec3(mesh.position.x, mesh.position.y, mesh.position.z),
            quaternion: new CANNON.Quaternion(
                mesh.quaternion.x, mesh.quaternion.y, mesh.quaternion.z, mesh.quaternion.w
            ),
        });

        // 添加所有碰撞形状（带偏移，支持 Compound）
        for (const s of shapes) {
            body.addShape(s.shape, s.offset, s.orientation);
        }

        body.linearDamping = this._linearDamping;
        body.angularDamping = this._angularDamping;

        // 初始动能（仅 DYNAMIC）
        if (phys.initialKinetic?.enabled && !isAnchored && !hasAutoRotate) {
            const v = phys.initialKinetic.velocity;
            body.velocity.set(v.x || 0, v.y || 0, v.z || 0);
        }

        // KINEMATIC 自动旋转：设置角速度
        if (hasAutoRotate && !isAnchored) {
            this._applyAutoRotateAngularVelocity(body, phys.autoRotate);
        }

        this.world.addBody(body);
        this.bodies.set(mesh.userData.id, body);
        this._meshById.set(mesh.userData.id, mesh);
        return body;
    }

    /**
     * 设置 KINEMATIC 刚体的角速度（用于自动旋转）
     */
    _applyAutoRotateAngularVelocity(body, autoRotate) {
        const speed = autoRotate.speed || 1;
        const axis = autoRotate.axis || 'y';
        body.angularVelocity.set(
            axis === 'x' ? speed : 0,
            axis === 'y' ? speed : 0,
            axis === 'z' ? speed : 0
        );
    }

    removeBody(mesh) {
        const body = this.bodies.get(mesh.userData.id);
        if (body) {
            this.world.removeBody(body);
            this.bodies.delete(mesh.userData.id);
            this._meshById.delete(mesh.userData.id);
        }
    }

    toggleObject(mesh) {
        if (this.bodies.has(mesh.userData.id)) {
            this.removeBody(mesh);
            if (this.bodies.size === 0) this._removeGround();
            return false;
        }
        this._ensureGround();
        this.createBody(mesh);
        return true;
    }

    toggleAll() {
        if (this.bodies.size > 0) {
            this.reset();
            return false;
        }
        this._ensureGround();
        for (const obj of this.sm.objects) {
            this.createBody(obj);
        }
        return true;
    }

    reset() {
        for (const body of this.bodies.values()) {
            this.world.removeBody(body);
        }
        this.bodies.clear();
        this._meshById.clear();
        this._removeGround();
        this.isActive = false;
    }

    // ============ 物理步进 ============

    update(dt) {
        const hasBodies = this.bodies.size > 0;
        if (!hasBodies) return;
        // 4 子步（偏效果）：碰撞响应更精确
        this.world.step(1 / 60, Math.min(dt, 0.05), 4);

        const zeroPoint = new CANNON.Vec3(0, 0, 0);

        for (const [id, body] of this.bodies) {
            const mesh = this._meshById.get(id);
            if (!mesh) continue;
            const phys = mesh.userData.physics || {};
            const isAnchored = phys.anchored;
            const hasAutoRotate = phys.autoRotate?.enabled;

            // ---- 位置/旋转同步 ----
            if (isAnchored && hasAutoRotate) {
                // STATIC + 自动旋转：直接旋转 mesh，同步到 body（用于碰撞检测）
                const speed = (phys.autoRotate.speed || 1) * dt;
                const axis = phys.autoRotate.axis || 'y';
                if (axis === 'x') mesh.rotation.x += speed;
                else if (axis === 'y') mesh.rotation.y += speed;
                else mesh.rotation.z += speed;
                body.quaternion.copy(mesh.quaternion);
            } else if (hasAutoRotate) {
                // KINEMATIC：刷新角速度，从 body 同步到 mesh
                this._applyAutoRotateAngularVelocity(body, phys.autoRotate);
                mesh.position.copy(body.position);
                mesh.quaternion.copy(body.quaternion);
            } else if (!isAnchored) {
                // DYNAMIC：从 body 同步到 mesh
                mesh.position.copy(body.position);
                mesh.quaternion.copy(body.quaternion);
            }
            // STATIC（anchored, 无 autoRotate）：不同步

            // ---- 引力（修复：applyForce 在质心施加，零向量避免扭矩） ----
            if (phys.attraction?.enabled && phys.attraction.targetId) {
                const targetMesh = this._meshById.get(phys.attraction.targetId);
                if (targetMesh) {
                    const dir = new CANNON.Vec3(
                        targetMesh.position.x - body.position.x,
                        targetMesh.position.y - body.position.y,
                        targetMesh.position.z - body.position.z
                    );
                    const dist = dir.length();
                    if (dist > 0.1) {
                        dir.normalize();
                        body.applyForce(dir.scale((phys.attraction.strength || 1) * 10), zeroPoint);
                    }
                }
            }

            // ---- 斥力（修复：同上） ----
            if (phys.repulsion?.enabled && phys.repulsion.targetId) {
                const targetMesh = this._meshById.get(phys.repulsion.targetId);
                if (targetMesh) {
                    const dir = new CANNON.Vec3(
                        body.position.x - targetMesh.position.x,
                        body.position.y - targetMesh.position.y,
                        body.position.z - targetMesh.position.z
                    );
                    const dist = dir.length();
                    if (dist > 0.1) {
                        dir.normalize();
                        body.applyForce(dir.scale((phys.repulsion.strength || 1) * 10), zeroPoint);
                    }
                }
            }
        }
    }
}

export { PhysicsManager };
