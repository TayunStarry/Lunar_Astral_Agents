import * as THREE from '../vendor/three.module.js';

// ============ 相机控制器 ============
class CameraController {
    constructor(renderer) {
        this.renderer = renderer;
        this.camera = renderer.camera;
        this.domElement = renderer.canvas;
        this.target = new THREE.Vector3();
        this.enabled = true;

        this._spherical = new THREE.Spherical();
        this._sphericalDelta = new THREE.Spherical();
        this._panOffset = new THREE.Vector3();
        this._isMouseDown = false;
        this._mouseButton = -1;
        this._lastMouse = new THREE.Vector2();

        // 键盘平移
        this._keys = {};
        this.keyboardPanSpeed = 0.6;
        this._cameraSpaceEnabled = true; // 无选中时允许 Space/Shift 升降摄像头

        // 环绕
        this._orbiting = false;
        this._orbitTarget = new THREE.Vector3();
        this._orbitSpeed = 1.5; // 弧度/秒

        // 越肩视角（物理操控模式）
        this._overShoulder = false;
        this._osTarget = null;            // 跟踪的 THREE.Object3D
        this._osSavedView = null;         // 进入前保存的视角，用于退出时恢复
        this._osHeight = 2.5;             // 相机相对目标的高度偏移
        this._osFocusOffsetY = 2;         // 越肩焦点相对目标脚底的高度（聚焦躯干）
        this._osMinDistance = 3;          // 越肩模式最小距离
        this._osFocus = null;             // 平滑插值后的焦点位置

        this.rotateSpeed = 0.5;
        this.zoomSpeed = 1.0;
        this.panSpeed = 0.8;
        this.minDistance = 0.5;
        this.maxDistance = 100;
        this.minPolarAngle = 0.1;
        this.maxPolarAngle = Math.PI - 0.1;

        this._onMouseDown = this._onMouseDown.bind(this);
        this._onMouseMove = this._onMouseMove.bind(this);
        this._onMouseUp = this._onMouseUp.bind(this);
        this._onWheel = this._onWheel.bind(this);
        this._onContextMenu = this._onContextMenu.bind(this);
        this._onKeyDown = this._onKeyDown.bind(this);
        this._onKeyUp = this._onKeyUp.bind(this);
        this.connect();
        this.updateSpherical();
    }

    connect() {
        const el = this.domElement;
        el.addEventListener('mousedown', this._onMouseDown);
        el.addEventListener('mousemove', this._onMouseMove);
        el.addEventListener('mouseup', this._onMouseUp);
        el.addEventListener('wheel', this._onWheel, { passive: false });
        el.addEventListener('contextmenu', this._onContextMenu);
        document.addEventListener('keydown', this._onKeyDown);
        document.addEventListener('keyup', this._onKeyUp);
    }

    disconnect() {
        const el = this.domElement;
        el.removeEventListener('mousedown', this._onMouseDown);
        el.removeEventListener('mousemove', this._onMouseMove);
        el.removeEventListener('mouseup', this._onMouseUp);
        el.removeEventListener('wheel', this._onWheel);
        el.removeEventListener('contextmenu', this._onContextMenu);
        document.removeEventListener('keydown', this._onKeyDown);
        document.removeEventListener('keyup', this._onKeyUp);
    }

    updateSpherical() {
        const offset = new THREE.Vector3().copy(this.camera.position).sub(this.target);
        this._spherical.setFromVector3(offset);
    }

    setTarget(pos) { this.target.copy(pos); this.updateSpherical(); }

    setView(position, target) {
        this.camera.position.copy(position);
        this.target.copy(target);
        this.camera.lookAt(target);
        this.updateSpherical();
    }

    getPosition() { return this.camera.position.clone(); }
    getTarget() { return this.target.clone(); }

    update() {
        if (!this.enabled) return;

        // 越肩视角模式：跟踪目标，鼠标可环绕，禁用 WASD/Space 相机平移
        if (this._overShoulder) {
            if (!this._osTarget) { this._overShoulder = false; return; }
            // 应用鼠标旋转/缩放增量
            this._spherical.theta += this._sphericalDelta.theta;
            this._spherical.phi += this._sphericalDelta.phi;
            this._spherical.phi = Math.max(this.minPolarAngle, Math.min(this.maxPolarAngle, this._spherical.phi));
            this._spherical.radius = Math.max(
                this._osMinDistance,
                Math.min(this.maxDistance, this._spherical.radius + this._sphericalDelta.radius)
            );
            // 目标点：跟踪物体位置 + 适当高度（聚焦物体上半部，避免视角被地面遮挡）
            const desiredFocus = new THREE.Vector3(
                this._osTarget.position.x,
                this._osTarget.position.y + this._osHeight * 0.3,
                this._osTarget.position.z
            );
            // 焦点平滑插值：消除物体微小物理抖动向相机的传递，视角跟随更平稳
            if (!this._osFocus) this._osFocus = desiredFocus.clone();
            else this._osFocus.lerp(desiredFocus, 0.25);
            const focus = this._osFocus;
            // 相机位置 = 目标 + 球面偏移
            const offset = new THREE.Vector3().setFromSpherical(this._spherical);
            this.camera.position.copy(focus).add(offset);
            this.camera.lookAt(focus);
            this.target.copy(focus);
            // 衰减增量（平滑过渡，避免抖动）
            this._sphericalDelta.theta *= 0.85;
            this._sphericalDelta.phi *= 0.85;
            this._sphericalDelta.radius *= 0.85;
            this._panOffset.set(0, 0, 0);
            return;
        }

        // 环绕模式
        if (this._orbiting) {
            this._spherical.theta += this._orbitSpeed * 0.016; // ~60fps delta
            this._spherical.phi = Math.max(this.minPolarAngle, Math.min(this.maxPolarAngle, this._spherical.phi));
            const pos = new THREE.Vector3().setFromSpherical(this._spherical).add(this._orbitTarget);
            this.camera.position.copy(pos);
            this.camera.lookAt(this._orbitTarget);
            this.target.copy(this._orbitTarget);
            return;
        }

        this._spherical.theta += this._sphericalDelta.theta;
        this._spherical.phi += this._sphericalDelta.phi;
        this._spherical.radius = Math.max(this.minDistance, Math.min(this.maxDistance, this._spherical.radius + this._sphericalDelta.radius));
        this._spherical.phi = Math.max(this.minPolarAngle, Math.min(this.maxPolarAngle, this._spherical.phi));

        // 键盘平移
        const moving = this._keys['KeyW'] || this._keys['KeyS'] || this._keys['KeyA'] || this._keys['KeyD'];
        if (moving) {
            const forward = new THREE.Vector3().copy(this.target).sub(this.camera.position);
            forward.y = 0; forward.normalize();
            const right = new THREE.Vector3().crossVectors(forward, this.camera.up).normalize();
            const kps = this._spherical.radius * 0.001 * this.keyboardPanSpeed;
            if (this._keys['KeyW']) this._panOffset.addScaledVector(forward, kps);
            if (this._keys['KeyS']) this._panOffset.addScaledVector(forward, -kps);
            if (this._keys['KeyD']) this._panOffset.addScaledVector(right, kps);
            if (this._keys['KeyA']) this._panOffset.addScaledVector(right, -kps);
        }

        // 垂直升降（Space/Shift）
        if (this._cameraSpaceEnabled) {
            const space = this._keys['Space'];
            const shift = this._keys['ShiftLeft'] || this._keys['ShiftRight'];
            if (space || shift) {
                const kps = this._spherical.radius * 0.001 * this.keyboardPanSpeed;
                this._panOffset.addScaledVector(this.camera.up, (space ? kps : 0) + (shift ? -kps : 0));
            }
        }

        this.target.add(this._panOffset);
        const pos = new THREE.Vector3().setFromSpherical(this._spherical).add(this.target);
        this.camera.position.copy(pos);
        this.camera.lookAt(this.target);
        this._sphericalDelta.theta *= 0.85;
        this._sphericalDelta.phi *= 0.85;
        this._sphericalDelta.radius *= 0.85;
        this._panOffset.multiplyScalar(0.85);
        if (Math.abs(this._sphericalDelta.theta) < 0.0001) this._sphericalDelta.theta = 0;
        if (Math.abs(this._sphericalDelta.phi) < 0.0001) this._sphericalDelta.phi = 0;
        if (Math.abs(this._sphericalDelta.radius) < 0.0001) this._sphericalDelta.radius = 0;
    }

    _onMouseDown(e) { if (!this.enabled) return; this._isMouseDown = true; this._mouseButton = e.button; this._lastMouse.set(e.clientX, e.clientY); }

    _onMouseMove(e) {
        if (!this._isMouseDown || !this.enabled) return;
        const dx = e.clientX - this._lastMouse.x;
        const dy = e.clientY - this._lastMouse.y;
        this._lastMouse.set(e.clientX, e.clientY);
        switch (this._mouseButton) {
            case 0: this._sphericalDelta.theta -= dx * 0.005 * this.rotateSpeed; this._sphericalDelta.phi -= dy * 0.005 * this.rotateSpeed; break;
            case 1:
            case 2: {
                const right = new THREE.Vector3().crossVectors(this.camera.up, new THREE.Vector3().copy(this.camera.position).sub(this.target).normalize()).normalize();
                const up = new THREE.Vector3().copy(this.camera.up);
                const ps = this._spherical.radius * 0.001 * this.panSpeed;
                this._panOffset.add(right.multiplyScalar(-dx * ps)).add(up.multiplyScalar(dy * ps));
                break;
            }
        }
    }

    _onMouseUp() { this._isMouseDown = false; this._mouseButton = -1; }
    _onWheel(e) { if (!this.enabled) return; e.preventDefault(); this._sphericalDelta.radius += e.deltaY * 0.01 * this.zoomSpeed; }
    _onContextMenu(e) { e.preventDefault(); }
    _onKeyDown(e) { this._keys[e.code] = true; }
    _onKeyUp(e) { this._keys[e.code] = false; }

    startOrbit(target) {
        this._orbitTarget.copy(target);
        this.target.copy(target);
        this.updateSpherical();
        const dist = this._spherical.radius;
        if (dist < 1) this._spherical.radius = 3;
        if (dist > 50) this._spherical.radius = 10;
        this._orbiting = true;
    }

    stopOrbit() {
        this._orbiting = false;
    }

    focusOnObject(obj) {
        const box = new THREE.Box3().setFromObject(obj);
        const center = new THREE.Vector3(); box.getCenter(center);
        const size = new THREE.Vector3(); box.getSize(size);
        const maxDim = Math.max(size.x, size.y, size.z);
        const dist = maxDim * 2.5;
        const dir = new THREE.Vector3().copy(this.camera.position).sub(this.target).normalize();
        const newPos = center.clone().add(dir.multiplyScalar(dist));
        // 如果处于越肩模式，更新焦点和球面坐标
        if (this._overShoulder) {
            this._osFocus.copy(center);
            this._spherical.radius = dist;
            this.target.copy(center);
        }
        this.setView(newPos, center);
    }

    focusOnAxis(obj, axis) {
        const box = new THREE.Box3().setFromObject(obj);
        const center = new THREE.Vector3(); box.getCenter(center);
        const size = new THREE.Vector3(); box.getSize(size);
        const maxDim = Math.max(size.x, size.y, size.z);
        const dist = maxDim * 2.5;
        let pos;
        switch (axis) {
            case 'top': pos = new THREE.Vector3(center.x, center.y + dist, center.z); break;
            case 'front': pos = new THREE.Vector3(center.x, center.y, center.z + dist); break;
            case 'right': pos = new THREE.Vector3(center.x + dist, center.y, center.z); break;
            default: this.focusOnObject(obj); return;
        }
        this.setView(pos, center);
    }

    // ============ 越肩视角（物理操控模式） ============

    /**
     * 启动越肩视角：相机持续跟踪目标物体，鼠标可环绕
     * 保存当前视角以便退出时恢复
     */
    startOverShoulder(target) {
        if (this._overShoulder) return;
        this._osTarget = target;
        this._osSavedView = {
            position: this.camera.position.clone(),
            target: this.target.clone(),
        };
        // 初始化平滑焦点为物体当前位置，避免进入瞬间相机跳变
        this._osFocus = new THREE.Vector3(
            target.position.x,
            target.position.y + this._osHeight * 0.3,
            target.position.z
        );
        // 基于物体包围盒初始化合适的观察距离与躯干焦点高度
        const box = new THREE.Box3().setFromObject(target);
        const size = new THREE.Vector3(); box.getSize(size);
        const maxDim = Math.max(size.x, size.y, size.z);
        const dist = Math.max(this._osMinDistance, maxDim * 3);
        this._osFocusOffsetY = Math.max(size.y * 0.45, 1);
        // 从当前相机方向计算球面坐标，保持视角连续性
        const offset = new THREE.Vector3().copy(this.camera.position).sub(this.target);
        this._spherical.setFromVector3(offset);
        this._spherical.radius = dist;
        this._overShoulder = true;
        this._orbiting = false; // 与环绕模式互斥
    }

    /**
     * 退出越肩视角：恢复进入前的默认视角
     */
    stopOverShoulder() {
        if (!this._overShoulder) return;
        this._overShoulder = false;
        this._osTarget = null;
        this._osFocus = null;
        if (this._osSavedView) {
            this.setView(this._osSavedView.position, this._osSavedView.target);
            this._osSavedView = null;
        }
    }

    isOverShoulder() { return this._overShoulder; }

    /**
     * 启用越肩跟随模式（WASD 按下时由 engine.js 调用）
     * 若已有跟踪目标则保持，否则等待 CharacterPhysics 附体后再启用
     */
    enableFollow() {
        if (!this._overShoulder && this._osTarget) {
            this._overShoulder = true;
        }
    }

    /**
     * 设置跟随目标（由 CharacterPhysics.attachToModel 调用）
     * @param {THREE.Object3D} target
     */
    setFollowTarget(target) {
        this._osTarget = target;
        this._osFocus = new THREE.Vector3().copy(target.position);
    }

    /**
     * 每帧更新（engine.js onUpdate 中调用）
     * 越肩模式下平滑跟随目标
     * @param {number} dt 帧间隔（秒）
     */
    tick(dt) {
        if (!this._overShoulder || !this._osTarget) {
            // 非越肩模式：执行默认轨道控制更新
            this.update();
            return;
        }
        // 越肩模式：平滑插值焦点到目标躯干位置，并用球坐标环绕（集成鼠标拖拽 / 滚轮）
        if (!this._osFocus) this._osFocus = new THREE.Vector3().copy(this._osTarget.position);
        const focusTarget = new THREE.Vector3(
            this._osTarget.position.x,
            this._osTarget.position.y + this._osFocusOffsetY,
            this._osTarget.position.z
        );
        this._osFocus.lerp(focusTarget, Math.min(1, dt * 18));

        // 应用鼠标/滚轮增量（环绕与缩放）
        this._spherical.theta += this._sphericalDelta.theta;
        this._spherical.phi += this._sphericalDelta.phi;
        this._spherical.phi = Math.max(this.minPolarAngle, Math.min(this.maxPolarAngle, this._spherical.phi));
        this._spherical.radius = Math.max(
            this._osMinDistance,
            Math.min(this.maxDistance, this._spherical.radius + this._sphericalDelta.radius)
        );

        // 相机位置 = 焦点 + 球面偏移，看向焦点
        const offset = new THREE.Vector3().setFromSpherical(this._spherical);
        this.camera.position.copy(this._osFocus).add(offset);
        this.target.copy(this._osFocus);
        this.camera.lookAt(this._osFocus);

        // 衰减增量（平滑过渡，避免抖动）
        this._sphericalDelta.theta *= 0.85;
        this._sphericalDelta.phi *= 0.85;
        this._sphericalDelta.radius *= 0.85;
        this._panOffset.set(0, 0, 0);
    }

    /**
     * 返回基于当前视角的水平移动方向（XZ 平面）
     * 用于 WASD 施力方向计算，确保操控方向与视角一致
     * @returns {{forward: THREE.Vector3, right: THREE.Vector3}}
     */
    getMoveDirections() {
        const forward = new THREE.Vector3().copy(this.target).sub(this.camera.position);
        forward.y = 0;
        if (forward.lengthSq() < 1e-6) forward.set(0, 0, -1);
        forward.normalize();
        const right = new THREE.Vector3().crossVectors(forward, this.camera.up).normalize();
        return { forward, right };
    }
}

export { CameraController };