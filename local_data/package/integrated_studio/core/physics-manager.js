import * as THREE from '../vendor/three.module.js';
import * as CANNON from '../vendor/cannon-es.module.js';

// ============ 物理引擎 ============
class PhysicsManager {
    constructor(renderer) {
        this.renderer = renderer;
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
        this._groundY = -0.05;

        // 可调参数
        this._massSingle = 1;
        this._linearDamping = 0.1;
        this._angularDamping = 0.1;
        this._restitution = 0.3;
        this._friction = 0.3;
        this.fallSpeedMultiplier = 2.0;

        // 碰撞体调试可视化
        this._debugGroup = null;       // THREE.Group 持有所有调试线框
        this._debugMeshes = new Map(); // mesh.userData.id -> THREE.Group
        this._debugVisible = false;

        // 物理操控模式（越肩视角 + WASD 速度追踪力 + 空格跳跃）
        this._cameraController = null;
        this.controlConfig = {
            targetSpeed: 6,       // 目标水平速度（m/s）
            forceGain: 10,        // 速度追踪力增益（力 = k × 速度差）
            jumpImpulse: 8,       // 跳跃瞬时冲量
            jumpCooldown: 0.5,    // 跳跃冷却（秒），防止连续跳跃
        };
        this.controlState = {
            active: false,
            meshId: null,
            body: null,
            lastJumpTime: 0,
            grounded: false,      // 是否接地（接触支撑面），用于跳跃限制
            keys: new Set(),      // 当前按下的键 code（KeyW/KeyA/KeyS/KeyD/Space）
        };
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

    get restitution() { return this._restitution; }
    set restitution(v) { this._restitution = v; }

    get friction() { return this._friction; }
    set friction(v) { this._friction = v; }

    _ensureGround() {
        if (this._groundBody) return;
        const groundShape = new CANNON.Plane();
        this._groundBody = new CANNON.Body({ mass: 0, shape: groundShape });
        this._groundBody.quaternion.setFromEuler(-Math.PI / 2, 0, 0);
        this._groundBody.position.set(0, this._groundY, 0);
        this.world.addBody(this._groundBody);
        // 调试可视化：为地面创建线框
        if (this._debugVisible) {
            this._createDebugMesh('__ground__', this._groundBody);
        }
    }

    _removeGround() {
        if (this._groundBody) {
            this._removeDebugMesh('__ground__');
            this.world.removeBody(this._groundBody);
            this._groundBody = null;
        }
    }

    // ============ 碰撞体生成 ============

    /**
     * 根据图元类型生成对应的 CANNON 碰撞形状
     * 返回 [{ shape, offset, orientation }] 数组（Compound 用）
     * @param {THREE.Object3D} mesh 网格
     * @param {{x,y,z}|null} parentScale 父级累计缩放（组合体嵌套用）
     */
    _createShapesForMesh(mesh, parentScale = null) {
        const type = mesh.userData.type;
        const params = mesh.userData.primitiveParams || {};
        // 有效缩放 = 父级缩放 * 自身缩放
        const ms = mesh.scale;
        const sx = parentScale ? parentScale.x * ms.x : ms.x;
        const sy = parentScale ? parentScale.y * ms.y : ms.y;
        const sz = parentScale ? parentScale.z * ms.z : ms.z;
        const identityQuat = new CANNON.Quaternion();
        const zeroOffset = new CANNON.Vec3();

        switch (type) {
            case 'cube': {
                const w = Math.max((params.w || 1) * sx / 2, 0.05);
                const h = Math.max((params.h || 1) * sy / 2, 0.05);
                const d = Math.max((params.d || 1) * sz / 2, 0.05);
                return [{ shape: new CANNON.Box(new CANNON.Vec3(w, h, d)), offset: zeroOffset, orientation: identityQuat }];
            }
            case 'sphere': {
                // 非均匀缩放时取最大轴，避免穿模（cannon 不支持椭球）
                const r = Math.max((params.r || 0.5) * Math.max(sx, sy, sz), 0.05);
                return [{ shape: new CANNON.Sphere(r), offset: zeroOffset, orientation: identityQuat }];
            }
            case 'cylinder': {
                // CANNON.Cylinder 轴向为 Y，半径在 XZ 平面
                const rt = Math.max((params.rt ?? 0.5) * Math.max(sx, sz), 0.01);
                const rb = Math.max((params.rb ?? 0.5) * Math.max(sx, sz), 0.01);
                const h = Math.max((params.h || 1) * sy, 0.1);
                const seg = Math.min(params.seg || 12, 24);
                const shape = new CANNON.Cylinder(rt, rb, h, seg);
                return [{ shape, offset: zeroOffset, orientation: identityQuat }];
            }
            case 'cone': {
                const r = Math.max((params.r || 0.5) * Math.max(sx, sz), 0.05);
                const h = Math.max((params.h || 1) * sy, 0.1);
                const seg = Math.min(params.seg || 12, 24);
                const shape = new CANNON.Cylinder(0.001, r, h, seg);
                return [{ shape, offset: zeroOffset, orientation: identityQuat }];
            }
            case 'torus': {
                // N 段 Box 切向排列，内孔精确匹配视觉
                return this._createTorusShapes(params, sx, sy, sz);
            }
            case 'dodecahedron':
            case 'octahedron':
            case 'tetrahedron': {
                // detail=0 时顶点数少，可用 ConvexPolyhedron 精确表达
                const detail = params.detail || 0;
                if (detail === 0) {
                    const shape = this._createConvexFromGeometry(mesh.geometry, sx, sy, sz);
                    if (shape) return [{ shape, offset: zeroOffset, orientation: identityQuat }];
                }
                // detail>0 顶点过多，降级为 Box
                return this._createBoxFromMesh(mesh, parentScale);
            }
            case 'torusKnot': {
                // torusKnot 凹形复杂，凸包近似误差大，用 Box 兜底
                return this._createBoxFromMesh(mesh, parentScale);
            }
            case 'ring': {
                // 圆环面在 XY 平面，Z 方向薄
                const outer = Math.max((params.outer || 0.5) * Math.max(sx, sy), 0.05);
                const half = new CANNON.Vec3(outer, outer, 0.02);
                return [{ shape: new CANNON.Box(half), offset: zeroOffset, orientation: identityQuat }];
            }
            case 'plane': {
                // 平面在 XY 平面，Z 方向薄
                const w = Math.max((params.w || 1) * sx / 2, 0.05);
                const h = Math.max((params.h || 1) * sy / 2, 0.05);
                const half = new CANNON.Vec3(w, h, 0.02);
                return [{ shape: new CANNON.Box(half), offset: zeroOffset, orientation: identityQuat }];
            }
            case 'group': {
                // 组合体：遍历子图元，生成 Compound shape
                return this._createGroupShapes(mesh, { x: sx, y: sy, z: sz });
            }
            default: {
                return this._createBoxFromMesh(mesh, parentScale);
            }
        }
    }

    /**
     * 圆环碰撞体重构：N 段 Box 切向排列
     *
     * 【核心改进】使用 Box 替代 Sphere，彻底解决内孔尺寸不匹配问题
     *
     * Sphere 方案的根本缺陷：
     *   球心在半径 R 圆上，球半径 t。内边缘(R-t)处两相邻球面之间存在
     *   固有间隙/凸起，数学上要求 cos(π/N)≥1 即 N→∞ 才能消除。
     *   大半径时 N 不足导致球体断开，内孔被球面凸起侵占，与视觉不符。
     *
     * Box 方案优势：
     *   每个 Box 径向半厚 = t → 内边缘为平面，精确位于 R-t，与视觉完全一致
     *   Box 切向长度 = 外缘弧长 × 1.15（15% 重叠确保无缝衔接）
     *   Box 垂直半厚 = t → 匹配管厚度
     *   大半径调整时孔径始终精确匹配视觉表现
     *
     * 坐标系（THREE.TorusGeometry 默认 XY 平面）：
     *   环在 XY 平面，管沿 Z 方向
     *   Box 局部轴：X=径向, Y=切向, Z=垂直(管厚方向)
     *   朝向：绕 Z 轴旋转 angle，使 Y 轴沿切向
     *
     * @param {object} params 圆环参数 {r, t, tSeg}
     * @param {number} sx sy sz 有效缩放
     */
    _createTorusShapes(params, sx = 1, sy = 1, sz = 1) {
        // THREE.TorusGeometry 默认在 XY 平面：环在 XY，管沿 Z 方向
        const R = Math.max(params.r || 0.5, 0.1) * Math.max(sx, sy);  // 大半径
        const t = Math.max(params.t || 0.2, 0.03) * Math.max(sx, sy, sz); // 管半径

        // 段数：基于管分段参数，12~48 之间
        const tSeg = params.tSeg || 32;
        const N = Math.max(12, Math.min(tSeg, 48));

        const arcAngle = (2 * Math.PI) / N;
        // Box 切向半长：外缘弧长的一半 × 1.15（15% 重叠确保相邻 Box 无缝）
        const halfLen = (R + t) * arcAngle * 0.575;

        const shapes = [];
        for (let i = 0; i < N; i++) {
            const angle = i * arcAngle;
            // halfExtents: X=径向半厚(t), Y=切向半长(halfLen), Z=垂直半厚(t)
            const halfExtents = new CANNON.Vec3(t, halfLen, t);
            // 位置：环上（XY 平面）
            const offset = new CANNON.Vec3(R * Math.cos(angle), R * Math.sin(angle), 0);
            // 朝向：绕 Z 轴旋转 angle，使 X 轴沿径向、Y 轴沿切向
            const orientation = new CANNON.Quaternion();
            orientation.setFromEuler(0, 0, angle);

            shapes.push({ shape: new CANNON.Box(halfExtents), offset, orientation });
        }
        return shapes;
    }

    /**
     * 组合体碰撞体：遍历子图元，各自生成形状并附带局部偏移
     * @param {THREE.Group} groupMesh 组合体
     * @param {{x,y,z}|null} parentScale 父级累计缩放
     */
    _createGroupShapes(groupMesh, parentScale = null) {
        const shapes = [];
        // 组合体有效缩放 = 父级缩放 * 自身缩放
        const gs = groupMesh.scale;
        const sx = parentScale ? parentScale.x * gs.x : gs.x;
        const sy = parentScale ? parentScale.y * gs.y : gs.y;
        const sz = parentScale ? parentScale.z * gs.z : gs.z;
        const groupScale = { x: sx, y: sy, z: sz };

        for (const child of groupMesh.children) {
            // 子图元有效缩放 = 组合体缩放 * 子图元缩放（在 _createShapesForMesh 内部计算）
            const childShapes = this._createShapesForMesh(child, groupScale);
            // 子图元位置偏移需乘以组合体缩放（在组合体局部空间内）
            const childPos = new CANNON.Vec3(
                child.position.x * sx,
                child.position.y * sy,
                child.position.z * sz
            );
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
        if (shapes.length === 0) return this._createBoxFromMesh(groupMesh, parentScale);
        return shapes;
    }

    /**
     * 从 THREE.BufferGeometry 创建 ConvexPolyhedron（用于凸多面体）
     * @param {THREE.BufferGeometry} geometry 几何体
     * @param {number} sx sy sz 有效缩放（缩放顶点坐标）
     */
    _createConvexFromGeometry(geometry, sx = 1, sy = 1, sz = 1) {
        const pos = geometry.attributes.position;
        const index = geometry.index;
        if (!pos) return null;

        // 顶点数过多时跳过（性能保护）
        if (pos.count > 100) return null;

        const vertices = [];
        const vertexMap = new Map();
        const tol = 0.0001;

        const getVertexIndex = (x, y, z) => {
            // 应用缩放
            const vx = x * sx, vy = y * sy, vz = z * sz;
            const key = `${Math.round(vx / tol)},${Math.round(vy / tol)},${Math.round(vz / tol)}`;
            if (vertexMap.has(key)) return vertexMap.get(key);
            const idx = vertices.length;
            vertices.push(new CANNON.Vec3(vx, vy, vz));
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
     * 优先使用几何体本地包围盒 + 有效缩放，避免组合体子图元的世界包围盒错误
     * @param {THREE.Object3D} mesh 网格
     * @param {{x,y,z}|null} parentScale 父级累计缩放
     */
    _createBoxFromMesh(mesh, parentScale = null) {
        const ms = mesh.scale;
        const sx = parentScale ? parentScale.x * ms.x : ms.x;
        const sy = parentScale ? parentScale.y * ms.y : ms.y;
        const sz = parentScale ? parentScale.z * ms.z : ms.z;

        // 优先使用几何体本地包围盒（未变换），乘以有效缩放
        if (mesh.geometry) {
            if (!mesh.geometry.boundingBox) mesh.geometry.computeBoundingBox();
            const bb = mesh.geometry.boundingBox;
            if (bb) {
                const size = new THREE.Vector3();
                bb.getSize(size);
                const half = new CANNON.Vec3(
                    Math.max(size.x * sx / 2, 0.05),
                    Math.max(size.y * sy / 2, 0.05),
                    Math.max(size.z * sz / 2, 0.05)
                );
                return [{ shape: new CANNON.Box(half), offset: new CANNON.Vec3(), orientation: new CANNON.Quaternion() }];
            }
        }
        // 兜底：使用世界包围盒（组合体等无 geometry 的情况）
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

        // 调试可视化：为新刚体创建线框
        if (this._debugVisible) {
            this._createDebugMesh(mesh.userData.id, body);
        }

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
        // 若被移除的刚体正是当前操控目标，先退出操控模式
        if (this.controlState.active && this.controlState.meshId === mesh.userData.id) {
            this.stopControl();
        }
        const body = this.bodies.get(mesh.userData.id);
        if (body) {
            this._removeDebugMesh(mesh.userData.id);
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
        for (const obj of this.renderer.primitivesRoot.children) {
            this.createBody(obj);
        }
        return true;
    }

    reset() {
        // 退出操控模式（恢复视角与操控方式）
        this.stopControl();
        // 清理调试线框
        for (const id of this._debugMeshes.keys()) {
            this._removeDebugMesh(id);
        }
        for (const body of this.bodies.values()) {
            this.world.removeBody(body);
        }
        this.bodies.clear();
        this._meshById.clear();
        this._removeGround();
        this.isActive = false;
    }

    // ============ 作用域适配 API（engine.js 调用） ============

    /**
     * 添加图元到物理世界（作用域内图元启用物理）
     * @param {THREE.Object3D} mesh
     */
    addPrimitive(mesh) {
        if (!mesh) return;
        this._ensureGround();
        this.createBody(mesh);
        this.isActive = true;
    }

    /**
     * 从物理世界移除图元（超出作用域，完全移除物理体）
     * @param {THREE.Object3D} mesh
     */
    removePrimitive(mesh) {
        if (!mesh) return;
        this.removeBody(mesh);
        if (this.bodies.size === 0) {
            this.isActive = false;
        }
    }

    /**
     * 设置调试可视化可见性（Z 键触发）
     * @param {boolean} visible
     */
    setDebugVisible(visible) {
        if (visible && !this._debugVisible) {
            this._debugVisible = true;
            this._showDebug();
        } else if (!visible && this._debugVisible) {
            this._debugVisible = false;
            this._hideDebug();
        }
    }

    // ============ 物理操控模式 ============

    /**
     * 绑定相机控制器（用于获取视角方向与启动越肩视角）
     * 由 UIManager 在构造时注入
     */
    setCameraController(cc) { this._cameraController = cc; }

    /**
     * 判断图元是否可被操控（存在且为 DYNAMIC 刚体）
     * 锚定/静态/自动旋转(KINEMATIC) 刚体不可操控
     */
    canControl(mesh) {
        if (!mesh) return false;
        const body = this.bodies.get(mesh.userData.id);
        return !!body && body.type === CANNON.Body.DYNAMIC;
    }

    /**
     * 启动物理操控模式：锁定目标刚体并启动越肩视角
     *
     * 设计原则：不修改刚体任何物理属性（旋转/阻尼/摩擦均保持原样），
     * 仅通过速度追踪力实现"推一把"移动，地面摩擦自然诱导滚动。
     * 这样完全保留原有物理滚动机制，且地面/空中均可移动。
     * @returns {boolean} 是否启动成功
     */
    startControl(mesh) {
        if (!this.canControl(mesh)) return false;
        const body = this.bodies.get(mesh.userData.id);
        this.controlState.active = true;
        this.controlState.meshId = mesh.userData.id;
        this.controlState.body = body;
        this.controlState.lastJumpTime = 0;
        this.controlState.grounded = false;
        this.controlState.keys.clear();
        if (this._cameraController) this._cameraController.startOverShoulder(mesh);
        return true;
    }

    /**
     * 退出物理操控模式：释放目标并恢复默认视角
     */
    stopControl() {
        if (!this.controlState.active) return;
        this.controlState.active = false;
        this.controlState.meshId = null;
        this.controlState.body = null;
        this.controlState.grounded = false;
        this.controlState.keys.clear();
        if (this._cameraController) this._cameraController.stopOverShoulder();
    }

    /**
     * 更新操控按键状态（由 UIManager keydown/keyup 转发）
     */
    setControlKey(code, isDown) {
        if (!this.controlState.active) return;
        if (isDown) this.controlState.keys.add(code);
        else this.controlState.keys.delete(code);
    }

    /**
     * 每帧应用操控力（在 world.step 之前调用，确保本帧生效）
     *
     * 【速度追踪力模型】力 = k × (目标速度 - 当前水平速度)
     *   - 按键时：目标速度 = 按键方向 × targetSpeed，力推动图元趋向目标速度
     *   - 松键时：不施力，图元靠摩擦+阻尼自然滑行/滚动（"推一把"语义）
     *   - 速度接近目标时力自然趋零，无速度上限 → 无振荡 → 无抖动
     *   - 力作用于质心，地面摩擦自然诱导滚动，完全保留物理滚动机制
     *   - 地面/空中均施力（空中无摩擦，靠速度差减速至目标速度）
     *
     * 跳跃：基于真实接触检测 + 冷却，确保落地前无法再次起跳
     */
    updateControl() {
        if (!this.controlState.active || !this.controlState.body) return;
        const body = this.controlState.body;
        const keys = this.controlState.keys;
        const cfg = this.controlConfig;

        // 获取视角方向（XZ 平面）
        let fx = 0, fz = -1, rx = 1, rz = 0;
        if (this._cameraController) {
            const dirs = this._cameraController.getMoveDirections();
            fx = dirs.forward.x; fz = dirs.forward.z;
            rx = dirs.right.x; rz = dirs.right.z;
        }

        // 累加按键方向
        let dx = 0, dz = 0;
        if (keys.has('KeyW')) { dx += fx; dz += fz; }
        if (keys.has('KeyS')) { dx -= fx; dz -= fz; }
        if (keys.has('KeyD')) { dx += rx; dz += rz; }
        if (keys.has('KeyA')) { dx -= rx; dz -= rz; }

        // 有按键时施加速度追踪力；无按键时不施力（自然滑行/滚动）
        if (dx !== 0 || dz !== 0) {
            // 归一化方向（防止对角线叠加导致速度翻倍）× 目标速度
            const len = Math.sqrt(dx * dx + dz * dz);
            const tvx = (dx / len) * cfg.targetSpeed;
            const tvz = (dz / len) * cfg.targetSpeed;
            // 速度追踪力：力 = k × (目标速度 - 当前水平速度)
            const dvx = tvx - body.velocity.x;
            const dvz = tvz - body.velocity.z;
            body.applyForce(
                new CANNON.Vec3(dvx * cfg.forceGain, 0, dvz * cfg.forceGain),
                new CANNON.Vec3(0, 0, 0) // 作用于质心，摩擦自然诱导滚动
            );
        }

        // 跳跃：必须接地（与支撑面有真实接触）+ 冷却，确保落地前无法再次起跳
        if (keys.has('Space')) {
            const now = performance.now() / 1000;
            if (this.controlState.grounded && now - this.controlState.lastJumpTime > cfg.jumpCooldown) {
                body.applyImpulse(
                    new CANNON.Vec3(0, cfg.jumpImpulse, 0),
                    new CANNON.Vec3(0, 0, 0)
                );
                this.controlState.lastJumpTime = now;
                this.controlState.grounded = false; // 起跳后立即标记离地，防止同帧连跳
            }
        }
    }

    /**
     * 检测操控刚体是否接地（与地面或其他图元存在支撑接触）
     *
     * 通过遍历 world.contacts 接触方程的法线方向判定：
     *   cannon-es 中 ni 为"指向 body i 外部"的法线
     *   - 操控体为 bi 且 ni.y < 0：法线朝下，说明下方有支撑
     *   - 操控体为 bj 且 ni.y > 0：法线朝上，说明下方有支撑
     * 此方法基于真实接触状态，可靠区分"在地面/图元上"与"空中"，彻底防止空中连跳
     */
    _isControlGrounded() {
        const body = this.controlState.body;
        if (!body) return false;
        for (const c of this.world.contacts) {
            if (c.bi === body && c.ni.y < -0.3) return true;
            if (c.bj === body && c.ni.y > 0.3) return true;
        }
        return false;
    }

    // ============ 物理步进 ============

    update(dt) {
        const hasBodies = this.bodies.size > 0;
        if (!hasBodies) return;
        // 操控力在步进前施加，确保本帧响应（120Hz 下延迟 < 1 帧 ≈ 8ms）
        this.updateControl();
        // 固定步长 1/120（120Hz）+ 8 子步：碰撞响应更精确，抖动显著降低
        // 子步数 = ceil(期望帧时间 / 固定步长) = ceil(0.05 / (1/120)) = 6 → 取 8 留余量
        this.world.step(1 / 120, Math.min(dt, 0.05), 8);

        const zeroPoint = new CANNON.Vec3(0, 0, 0);

        for (const [id, body] of this.bodies) {
            const mesh = this._meshById.get(id);
            if (!mesh) continue;
            const phys = mesh.userData.physics || {};
            const isAnchored = phys.anchored;
            const hasAutoRotate = phys.autoRotate?.enabled;

            // ---- 下落加速（DYNAMIC 物体下落时施加强制力） ----
            if (body.type === CANNON.Body.DYNAMIC && body.velocity.y < 0 && this.fallSpeedMultiplier > 1) {
                body.applyForce(
                    new CANNON.Vec3(0, -9.82 * (this.fallSpeedMultiplier - 1) * body.mass, 0),
                    zeroPoint
                );
            }

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

        // 调试可视化：同步线框位置/旋转
        if (this._debugVisible) {
            for (const [id, body] of this.bodies) {
                const dbg = this._debugMeshes.get(id);
                if (dbg) {
                    dbg.position.copy(body.position);
                    dbg.quaternion.copy(body.quaternion);
                }
            }
        }

        // 更新操控刚体的接地状态（基于本帧步进后的真实接触），供下一帧跳跃判定
        if (this.controlState.active) {
            this.controlState.grounded = this._isControlGrounded();
        }
    }

    // ============ 碰撞体调试可视化 ============

    /**
     * 切换碰撞体线框可视化（Z 键触发）
     * 为每个刚体创建半透明线框，直观对比碰撞体与视觉网格
     */
    toggleDebug() {
        this._debugVisible = !this._debugVisible;
        if (this._debugVisible) {
            this._showDebug();
        } else {
            this._hideDebug();
        }
        return this._debugVisible;
    }

    _showDebug() {
        if (!this._debugGroup) {
            this._debugGroup = new THREE.Group();
            this._debugGroup.name = 'physics-debug';
            this.renderer.scene.add(this._debugGroup);
        }
        // 为所有已存在刚体创建线框
        for (const [id, body] of this.bodies) {
            if (!this._debugMeshes.has(id)) {
                this._createDebugMesh(id, body);
            }
        }
        // 地面
        if (this._groundBody && !this._debugMeshes.has('__ground__')) {
            this._createDebugMesh('__ground__', this._groundBody);
        }
        this._debugGroup.visible = true;
    }

    _hideDebug() {
        if (this._debugGroup) {
            this._debugGroup.visible = false;
        }
    }

    /**
     * 为单个刚体创建调试线框组
     * 支持 Compound Shape（多 shape + offset + orientation）
     */
    _createDebugMesh(id, body) {
        this._removeDebugMesh(id);

        const group = new THREE.Group();
        const material = new THREE.MeshBasicMaterial({
            color: 0x00ffff,
            wireframe: true,
            transparent: true,
            opacity: 0.5,
            depthTest: true,
        });

        for (let i = 0; i < body.shapes.length; i++) {
            const shape = body.shapes[i];
            const offset = body.shapeOffsets[i];
            const orient = body.shapeOrientations[i];
            const mesh = this._createDebugShapeMesh(shape, material);
            if (!mesh) continue;
            mesh.position.set(offset.x, offset.y, offset.z);
            mesh.quaternion.set(orient.x, orient.y, orient.z, orient.w);
            group.add(mesh);
        }

        group.position.copy(body.position);
        group.quaternion.copy(body.quaternion);
        this._debugGroup.add(group);
        this._debugMeshes.set(id, group);
    }

    /**
     * 将 CANNON.Shape 转为 THREE.Mesh 线框
     */
    _createDebugShapeMesh(shape, material) {
        let geometry = null;
        const types = CANNON.Shape.types;

        switch (shape.type) {
            case types.BOX:
                geometry = new THREE.BoxGeometry(
                    shape.halfExtents.x * 2,
                    shape.halfExtents.y * 2,
                    shape.halfExtents.z * 2
                );
                break;
            case types.SPHERE:
                geometry = new THREE.SphereGeometry(shape.radius, 12, 8);
                break;
            case types.CYLINDER:
            case types.CONVEXPOLYHEDRON:
                geometry = this._convexToGeometry(shape);
                break;
            case types.PLANE:
                geometry = new THREE.PlaneGeometry(20, 20);
                break;
            default:
                return null;
        }
        return new THREE.Mesh(geometry, material);
    }

    /**
     * 从 ConvexPolyhedron（含 Cylinder）提取顶点/面，构建 THREE.BufferGeometry
     */
    _convexToGeometry(shape) {
        const verts = [];
        for (const v of shape.vertices) {
            verts.push(v.x, v.y, v.z);
        }
        const indices = [];
        for (const face of shape.faces) {
            // 三角扇分解多边形面
            for (let i = 1; i < face.length - 1; i++) {
                indices.push(face[0], face[i], face[i + 1]);
            }
        }
        const geo = new THREE.BufferGeometry();
        geo.setAttribute('position', new THREE.Float32BufferAttribute(verts, 3));
        geo.setIndex(indices);
        geo.computeVertexNormals();
        return geo;
    }

    _removeDebugMesh(id) {
        const group = this._debugMeshes.get(id);
        if (group) {
            this._debugGroup.remove(group);
            group.traverse(child => {
                if (child.geometry) child.geometry.dispose();
            });
            this._debugMeshes.delete(id);
        }
    }
}

export { PhysicsManager };
