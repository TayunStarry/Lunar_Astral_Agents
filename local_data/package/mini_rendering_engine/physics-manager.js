import * as THREE from './three.module.js';
import * as CANNON from './cannon-es.module.js';

// ============ 物理引擎 ============
class PhysicsManager {
    constructor(sceneManager) {
        this.sm = sceneManager;
        this.world = new CANNON.World({
            gravity: new CANNON.Vec3(0, -9.82, 0),
        });
        this.world.broadphase = new CANNON.NaiveBroadphase();
        this.world.solver.iterations = 10;

        this.bodies = new Map(); // mesh.userData.id -> CANNON.Body
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

    createBody(mesh) {
        if (this.bodies.has(mesh.userData.id)) return null;
        const box = new THREE.Box3().setFromObject(mesh);
        const size = new THREE.Vector3(); box.getSize(size);
        const half = new CANNON.Vec3(
            Math.max(size.x / 2, 0.1),
            Math.max(size.y / 2, 0.1),
            Math.max(size.z / 2, 0.1)
        );
        const shape = new CANNON.Box(half);
        const childCount = mesh.userData.type === 'group' ? mesh.children.length : 1;
        const mass = childCount * this._massSingle;
        const body = new CANNON.Body({
            mass,
            shape,
            position: new CANNON.Vec3(mesh.position.x, mesh.position.y, mesh.position.z),
            quaternion: new CANNON.Quaternion(mesh.quaternion.x, mesh.quaternion.y, mesh.quaternion.z, mesh.quaternion.w),
        });
        body.linearDamping = this._linearDamping;
        body.angularDamping = this._angularDamping;
        this.world.addBody(body);
        this.bodies.set(mesh.userData.id, body);
        return body;
    }

    removeBody(mesh) {
        const body = this.bodies.get(mesh.userData.id);
        if (body) {
            this.world.removeBody(body);
            this.bodies.delete(mesh.userData.id);
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
        this._removeGround();
        this.isActive = false;
    }

    update(dt) {
        const hasBodies = this.bodies.size > 0;
        if (!hasBodies) return;
        this.world.step(1 / 60, Math.min(dt, 0.05), 3);
        for (const [id, body] of this.bodies) {
            const mesh = this.sm.objects.find(o => o.userData.id === id);
            if (mesh) {
                mesh.position.copy(body.position);
                mesh.quaternion.copy(body.quaternion);
            }
        }
    }
}

export { PhysicsManager };