import * as THREE from './three.module.js';

// ============ 相机控制器 ============
class CameraController {
    constructor(camera, domElement) {
        this.camera = camera;
        this.domElement = domElement;
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
}

export { CameraController };