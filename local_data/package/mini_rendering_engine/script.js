import * as THREE from '/file/read/package/minecraft-demo/three.module.js';

// ============ 图元定义 ============
const PRIMITIVES = {
    cube:     { geo: () => new THREE.BoxGeometry(1, 1, 1),          icon: 'fa-cube',        name: '立方体' },
    sphere:   { geo: () => new THREE.SphereGeometry(0.5, 32, 32),   icon: 'fa-circle',      name: '球体' },
    cylinder: { geo: () => new THREE.CylinderGeometry(0.5, 0.5, 1, 32), icon: 'fa-database', name: '圆柱体' },
    cone:     { geo: () => new THREE.ConeGeometry(0.5, 1, 32),      icon: 'fa-traffic-cone',name: '圆锥体' },
    plane:    { geo: () => new THREE.PlaneGeometry(1, 1),           icon: 'fa-square',      name: '平面' },
    torus:    { geo: () => new THREE.TorusGeometry(0.5, 0.2, 16, 32), icon: 'fa-donut',     name: '圆环' },
};

// ============ 场景管理器 ============
class SceneManager {
    constructor(canvas) {
        this.canvas = canvas;
        this.objects = [];         // 场景中的用户对象
        this.selected = null;      // 当前选中的对象
        this.gizmoMode = 'translate'; // translate | rotate | scale
        this.gizmoActive = false;
        this.nextId = 1;

        // 场景
        this.scene = new THREE.Scene();
        this.scene.background = new THREE.Color('#2a2a3a');

        // 渲染器
        this.renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true });
        this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
        this.renderer.shadowMap.enabled = true;
        this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
        this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
        this.renderer.toneMappingExposure = 1.0;

        // 相机
        this.camera = new THREE.PerspectiveCamera(55, 2, 0.1, 500);
        this.camera.position.set(5, 4, 8);
        this.camera.lookAt(0, 0, 0);

        // 灯光
        this.setupLights();
        // 地面网格
        this.setupGrid();
        // 天空盒
        this.setupSkybox();
        // 射线检测
        this.raycaster = new THREE.Raycaster();

        // 响应式
        this.onResize = this.onResize.bind(this);
        window.addEventListener('resize', this.onResize);
        this.onResize();
    }

    setupLights() {
        this.ambientLight = new THREE.AmbientLight(0x404060, 0.6);
        this.scene.add(this.ambientLight);

        this.hemiLight = new THREE.HemisphereLight(0x606080, 0x303040, 0.5);
        this.scene.add(this.hemiLight);

        this.sunLight = new THREE.DirectionalLight(0xfff5e8, 1.2);
        this.sunLight.position.set(10, 15, 8);
        this.sunLight.castShadow = true;
        this.sunLight.shadow.mapSize.set(1024, 1024);
        this.sunLight.shadow.camera.near = 0.5;
        this.sunLight.shadow.camera.far = 80;
        this.sunLight.shadow.camera.left = -20;
        this.sunLight.shadow.camera.right = 20;
        this.sunLight.shadow.camera.top = 20;
        this.sunLight.shadow.camera.bottom = -20;
        this.sunLight.shadow.bias = -0.0005;
        this.scene.add(this.sunLight);
    }

    setupGrid() {
        const grid = new THREE.GridHelper(20, 20, 0x444466, 0x222244);
        this.scene.add(grid);

        // 接收阴影的地面
        const groundGeo = new THREE.PlaneGeometry(20, 20);
        const groundMat = new THREE.ShadowMaterial({ opacity: 0.3 });
        this.groundPlane = new THREE.Mesh(groundGeo, groundMat);
        this.groundPlane.rotation.x = -Math.PI / 2;
        this.groundPlane.receiveShadow = true;
        this.scene.add(this.groundPlane);
    }

    setupSkybox() {
        const skyGeo = new THREE.SphereGeometry(100, 32, 16);
        const skyMat = new THREE.ShaderMaterial({
            side: THREE.BackSide,
            depthWrite: false,
            uniforms: {
                topColor:    { value: new THREE.Color('#1a2a4a') },
                midColor:    { value: new THREE.Color('#3a5a7a') },
                bottomColor: { value: new THREE.Color('#5a7a9a') },
            },
            vertexShader: `
                varying vec3 vWorldPos;
                void main() {
                    vec4 wp = modelMatrix * vec4(position, 1.0);
                    vWorldPos = wp.xyz;
                    gl_Position = projectionMatrix * viewMatrix * wp;
                }
            `,
            fragmentShader: `
                uniform vec3 topColor, midColor, bottomColor;
                varying vec3 vWorldPos;
                void main() {
                    float h = normalize(vWorldPos).y;
                    vec3 col = h > 0.0 ? mix(midColor, topColor, h) : mix(midColor, bottomColor, -h);
                    gl_FragColor = vec4(col, 1.0);
                }
            `
        });
        this.skySphere = new THREE.Mesh(skyGeo, skyMat);
        this.scene.add(this.skySphere);
    }

    setSkyboxColors(top, mid, bottom) {
        this.skySphere.material.uniforms.topColor.value.set(top);
        this.skySphere.material.uniforms.midColor.value.set(mid);
        this.skySphere.material.uniforms.bottomColor.value.set(bottom);
    }

    addPrimitive(type) {
        const def = PRIMITIVES[type];
        if (!def) return null;
        const geometry = def.geo();
        const material = new THREE.MeshStandardMaterial({
            color: this._randomColor(),
            roughness: 0.5,
            metalness: 0.1,
        });
        const mesh = new THREE.Mesh(geometry, material);
        mesh.castShadow = true;
        mesh.receiveShadow = true;
        mesh.position.set(0, 0.5, 0);
        mesh.userData = {
            id: this.nextId++,
            name: `${def.name} ${this.nextId - 1}`,
            type: type,
            primitiveType: type,
        };
        this.scene.add(mesh);
        this.objects.push(mesh);
        this.select(mesh);
        return mesh;
    }

    addImportedMesh(geometry, materialData, name, position, rotation, scale) {
        const material = this._createMaterial(materialData);
        const mesh = new THREE.Mesh(geometry, material);
        mesh.castShadow = true;
        mesh.receiveShadow = true;
        mesh.position.copy(position || new THREE.Vector3(0, 0.5, 0));
        if (rotation) mesh.rotation.set(rotation.x, rotation.y, rotation.z);
        if (scale) mesh.scale.set(scale.x, scale.y, scale.z);
        mesh.userData = {
            id: this.nextId++,
            name: name || '导入模型',
            type: 'imported',
            materialData: materialData,
        };
        this.scene.add(mesh);
        this.objects.push(mesh);
        return mesh;
    }

    select(mesh) {
        if (this.selected === mesh) return;
        this.deselect();
        this.selected = mesh;
        // 添加选中轮廓（简单的发光效果：调亮材质）
        if (mesh.material && mesh.material.emissive) {
            mesh.material._savedEmissive = mesh.material.emissive.getHex();
            mesh.material.emissive.set(0x333333);
        }
    }

    deselect() {
        if (!this.selected) return;
        if (this.selected.material && this.selected.material.emissive && this.selected.material._savedEmissive !== undefined) {
            this.selected.material.emissive.setHex(this.selected.material._savedEmissive);
        }
        this.selected = null;
    }

    deleteSelected() {
        if (!this.selected) return;
        this.scene.remove(this.selected);
        if (this.selected.geometry) this.selected.geometry.dispose();
        if (this.selected.material) this.selected.material.dispose();
        const idx = this.objects.indexOf(this.selected);
        if (idx !== -1) this.objects.splice(idx, 1);
        this.selected = null;
    }

    duplicateSelected() {
        if (!this.selected) return null;
        const src = this.selected;
        const geo = src.geometry.clone();
        const matData = this._materialToData(src.material);
        const mat = this._createMaterial(matData);
        const mesh = new THREE.Mesh(geo, mat);
        mesh.castShadow = true;
        mesh.receiveShadow = true;
        mesh.position.copy(src.position).add(new THREE.Vector3(0.8, 0, 0.8));
        mesh.rotation.copy(src.rotation);
        mesh.scale.copy(src.scale);
        mesh.userData = {
            id: this.nextId++,
            name: `${src.userData.name} (副本)`,
            type: src.userData.type,
            primitiveType: src.userData.primitiveType,
            materialData: matData,
        };
        this.scene.add(mesh);
        this.objects.push(mesh);
        this.select(mesh);
        return mesh;
    }

    focusOnSelected() {
        if (!this.selected) return;
        const box = new THREE.Box3().setFromObject(this.selected);
        const center = new THREE.Vector3();
        box.getCenter(center);
        const size = box.getSize(new THREE.Vector3());
        const dist = Math.max(size.x, size.y, size.z) * 1.5;
        this.camera.position.copy(center).add(new THREE.Vector3(dist, dist * 0.6, dist));
        this.camera.lookAt(center);
    }

    getObjectsArray() { return this.objects; }

    getTotalTriangles() {
        let count = 0;
        for (const obj of this.objects) {
            if (obj.geometry && obj.geometry.index) {
                count += obj.geometry.index.count / 3;
            } else if (obj.geometry && obj.geometry.attributes.position) {
                count += obj.geometry.attributes.position.count / 3;
            }
        }
        return Math.floor(count);
    }

    onResize() {
        this.camera.aspect = window.innerWidth / window.innerHeight;
        this.camera.updateProjectionMatrix();
        this.renderer.setSize(window.innerWidth, window.innerHeight);
    }

    render() {
        this.renderer.render(this.scene, this.camera);
    }

    // 辅助方法
    _randomColor() {
        const h = Math.random();
        const s = 0.4 + Math.random() * 0.3;
        const l = 0.35 + Math.random() * 0.25;
        return new THREE.Color().setHSL(h, s, l);
    }

    _materialToData(mat) {
        if (!mat || !mat.isMaterial) return null;
        const data = {
            color: '#' + mat.color.getHexString(),
            roughness: mat.roughness,
            metalness: mat.metalness,
            opacity: mat.opacity,
            transparent: mat.transparent,
            type: mat.isMeshStandardMaterial ? 'standard' : 'basic',
        };
        if (mat.map) {
            data.texture = this._textureToBase64(mat.map);
        }
        return data;
    }

    _createMaterial(data) {
        if (!data) return new THREE.MeshStandardMaterial({ color: 0x888888, roughness: 0.5, metalness: 0.1 });
        const opts = {
            color: new THREE.Color(data.color || '#888888'),
            roughness: data.roughness ?? 0.5,
            metalness: data.metalness ?? 0.1,
            opacity: data.opacity ?? 1,
            transparent: data.transparent ?? false,
        };
        if (data.texture) {
            opts.map = this._base64ToTexture(data.texture);
        }
        return data.type === 'basic'
            ? new THREE.MeshBasicMaterial(opts)
            : new THREE.MeshStandardMaterial(opts);
    }

    _textureToBase64(texture) {
        if (!texture || !texture.image) return null;
        const canvas = document.createElement('canvas');
        canvas.width = texture.image.width;
        canvas.height = texture.image.height;
        const ctx = canvas.getContext('2d');
        ctx.drawImage(texture.image, 0, 0);
        return canvas.toDataURL('image/png');
    }

    _base64ToTexture(base64) {
        const img = new Image();
        img.src = base64;
        const tex = new THREE.Texture(img);
        tex.wrapS = THREE.RepeatWrapping;
        tex.wrapT = THREE.RepeatWrapping;
        tex.colorSpace = THREE.SRGBColorSpace;
        img.onload = () => tex.needsUpdate = true;
        return tex;
    }

    dispose() {
        window.removeEventListener('resize', this.onResize);
        for (const obj of this.objects) {
            if (obj.geometry) obj.geometry.dispose();
            if (obj.material) obj.material.dispose();
        }
        this.renderer.dispose();
    }
}

// ============ 相机控制器（轨道模式） ============
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
    }

    disconnect() {
        const el = this.domElement;
        el.removeEventListener('mousedown', this._onMouseDown);
        el.removeEventListener('mousemove', this._onMouseMove);
        el.removeEventListener('mouseup', this._onMouseUp);
        el.removeEventListener('wheel', this._onWheel);
        el.removeEventListener('contextmenu', this._onContextMenu);
    }

    updateSpherical() {
        const offset = new THREE.Vector3().copy(this.camera.position).sub(this.target);
        this._spherical.setFromVector3(offset);
    }

    setTarget(pos) {
        this.target.copy(pos);
        this.updateSpherical();
    }

    setView(position, target) {
        this.camera.position.copy(position);
        this.target.copy(target);
        this.camera.lookAt(target);
        this.updateSpherical();
    }

    update() {
        if (!this.enabled) return;

        this._spherical.theta += this._sphericalDelta.theta;
        this._spherical.phi   += this._sphericalDelta.phi;
        this._spherical.radius = Math.max(this.minDistance, Math.min(this.maxDistance, this._spherical.radius + this._sphericalDelta.radius));
        this._spherical.phi    = Math.max(this.minPolarAngle, Math.min(this.maxPolarAngle, this._spherical.phi));

        this.target.add(this._panOffset);

        const pos = new THREE.Vector3().setFromSpherical(this._spherical).add(this.target);
        this.camera.position.copy(pos);
        this.camera.lookAt(this.target);

        // 阻尼
        this._sphericalDelta.theta *= 0.85;
        this._sphericalDelta.phi   *= 0.85;
        this._sphericalDelta.radius *= 0.85;
        this._panOffset.multiplyScalar(0.85);

        // 如果变化很小，直接归零
        if (Math.abs(this._sphericalDelta.theta) < 0.0001) this._sphericalDelta.theta = 0;
        if (Math.abs(this._sphericalDelta.phi)   < 0.0001) this._sphericalDelta.phi   = 0;
        if (Math.abs(this._sphericalDelta.radius) < 0.0001) this._sphericalDelta.radius = 0;
    }

    _onMouseDown(e) {
        if (!this.enabled) return;
        this._isMouseDown = true;
        this._mouseButton = e.button;
        this._lastMouse.set(e.clientX, e.clientY);
    }

    _onMouseMove(e) {
        if (!this._isMouseDown || !this.enabled) return;
        const dx = e.clientX - this._lastMouse.x;
        const dy = e.clientY - this._lastMouse.y;
        this._lastMouse.set(e.clientX, e.clientY);

        switch (this._mouseButton) {
            case 0: // 左键：旋转
                this._sphericalDelta.theta -= dx * 0.005 * this.rotateSpeed;
                this._sphericalDelta.phi   -= dy * 0.005 * this.rotateSpeed;
                break;
            case 1: // 中键：平移
            case 2: // 右键：平移
                const right = new THREE.Vector3();
                const up = new THREE.Vector3();
                this.camera.getWorldDirection(new THREE.Vector3());
                right.crossVectors(this.camera.up, new THREE.Vector3().copy(this.camera.position).sub(this.target).normalize()).normalize();
                up.copy(this.camera.up);
                const panScale = this._spherical.radius * 0.001 * this.panSpeed;
                this._panOffset.add(right.multiplyScalar(-dx * panScale));
                this._panOffset.add(up.multiplyScalar(dy * panScale));
                break;
        }
    }

    _onMouseUp() { this._isMouseDown = false; this._mouseButton = -1; }

    _onWheel(e) {
        if (!this.enabled) return;
        e.preventDefault();
        this._sphericalDelta.radius += e.deltaY * 0.01 * this.zoomSpeed;
    }

    _onContextMenu(e) { e.preventDefault(); }
}

// ============ 资产管理器（导入/导出） ============
class AssetManager {
    /**
     * 导出场景为 JSON 方案文件
     */
    static exportScene(objects, skyboxColors) {
        const data = {
            version: '1.0',
            name: '渲染方案',
            createdAt: new Date().toISOString(),
            skybox: {
                top: skyboxColors.top,
                mid: skyboxColors.mid,
                bottom: skyboxColors.bottom,
            },
            objects: objects.map(obj => AssetManager._serializeObject(obj)),
        };
        const json = JSON.stringify(data, null, 2);
        const blob = new Blob([json], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `scene_${Date.now()}.json`;
        a.click();
        URL.revokeObjectURL(url);
    }

    /**
     * 导入场景 JSON 方案文件
     * @returns {Promise<object>} 解析后的场景数据
     */
    static importScene(file) {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = (e) => {
                try {
                    const data = JSON.parse(e.target.result);
                    if (!data.version || !data.objects) {
                        reject(new Error('无效的方案文件格式'));
                        return;
                    }
                    resolve(data);
                } catch (err) {
                    reject(new Error('JSON 解析失败: ' + err.message));
                }
            };
            reader.onerror = () => reject(new Error('文件读取失败'));
            reader.readAsText(file);
        });
    }

    /**
     * 导出单个模型为 JSON（含材质纹理 base64）
     */
    static exportModel(mesh) {
        const data = AssetManager._serializeObject(mesh);
        const json = JSON.stringify(data, null, 2);
        const blob = new Blob([json], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `${mesh.userData.name || 'model'}.json`;
        a.click();
        URL.revokeObjectURL(url);
    }

    /**
     * 导出材质预设为 JSON
     */
    static exportMaterial(mesh) {
        if (!mesh || !mesh.material) return;
        const mat = mesh.material;
        const data = {
            version: '1.0',
            type: 'material',
            name: `${mesh.userData.name || 'material'}_材质`,
            color: '#' + mat.color.getHexString(),
            roughness: mat.roughness,
            metalness: mat.metalness,
            opacity: mat.opacity,
            transparent: mat.transparent,
            materialType: mat.isMeshStandardMaterial ? 'standard' : 'basic',
        };
        if (mat.map && mat.map.image) {
            const canvas = document.createElement('canvas');
            canvas.width = mat.map.image.width;
            canvas.height = mat.map.image.height;
            canvas.getContext('2d').drawImage(mat.map.image, 0, 0);
            data.texture = canvas.toDataURL('image/png');
        }
        const json = JSON.stringify(data, null, 2);
        const blob = new Blob([json], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `${data.name}.json`;
        a.click();
        URL.revokeObjectURL(url);
    }

    /** 序列化单个 Three.js Mesh */
    static _serializeObject(mesh) {
        const data = {
            id: mesh.userData.id,
            name: mesh.userData.name || '未命名',
            type: mesh.userData.type || 'unknown',
            primitiveType: mesh.userData.primitiveType || null,
            position: { x: mesh.position.x, y: mesh.position.y, z: mesh.position.z },
            rotation: { x: mesh.rotation.x, y: mesh.rotation.y, z: mesh.rotation.z },
            scale:    { x: mesh.scale.x, y: mesh.scale.y, z: mesh.scale.z },
        };

        // 材质
        if (mesh.material) {
            data.material = {
                color: '#' + mesh.material.color.getHexString(),
                roughness: mesh.material.roughness,
                metalness: mesh.material.metalness,
                opacity: mesh.material.opacity,
                transparent: mesh.material.transparent,
                type: mesh.material.isMeshStandardMaterial ? 'standard' : 'basic',
            };
            if (mesh.material.map && mesh.material.map.image) {
                const canvas = document.createElement('canvas');
                canvas.width = mesh.material.map.image.width;
                canvas.height = mesh.material.map.image.height;
                canvas.getContext('2d').drawImage(mesh.material.map.image, 0, 0);
                data.material.texture = canvas.toDataURL('image/png');
            }
        }

        // 几何体（仅图元保存参数）
        if (mesh.geometry && mesh.userData.primitiveType) {
            data.geometry = AssetManager._serializeGeometry(mesh.geometry, mesh.userData.primitiveType);
        }

        return data;
    }

    static _serializeGeometry(geo, primitiveType) {
        const params = geo.parameters || {};
        const data = { type: primitiveType };
        switch (primitiveType) {
            case 'cube':     data.width = params.width || 1; data.height = params.height || 1; data.depth = params.depth || 1; break;
            case 'sphere':   data.radius = params.radius || 0.5; break;
            case 'cylinder': data.radiusTop = params.radiusTop || 0.5; data.radiusBottom = params.radiusBottom || 0.5; data.height = params.height || 1; break;
            case 'cone':     data.radius = params.radius || 0.5; data.height = params.height || 1; break;
            case 'plane':    data.width = params.width || 1; data.height = params.height || 1; break;
            case 'torus':    data.radius = params.radius || 0.5; data.tube = params.tube || 0.2; break;
        }
        return data;
    }

    /** 从序列化数据重建几何体 */
    static deserializeGeometry(data) {
        switch (data.type) {
            case 'cube':     return new THREE.BoxGeometry(data.width || 1, data.height || 1, data.depth || 1);
            case 'sphere':   return new THREE.SphereGeometry(data.radius || 0.5, 32, 32);
            case 'cylinder': return new THREE.CylinderGeometry(data.radiusTop || 0.5, data.radiusBottom || 0.5, data.height || 1, 32);
            case 'cone':     return new THREE.ConeGeometry(data.radius || 0.5, data.height || 1, 32);
            case 'plane':    return new THREE.PlaneGeometry(data.width || 1, data.height || 1);
            case 'torus':    return new THREE.TorusGeometry(data.radius || 0.5, data.tube || 0.2, 16, 32);
            default:         return new THREE.BoxGeometry(1, 1, 1);
        }
    }
}

// ============ UI 管理器 ============
class UIManager {
    constructor(sceneManager, cameraController) {
        this.sm = sceneManager;
        this.cc = cameraController;

        // DOM 引用
        this.toolbar = document.getElementById('toolbar');
        this.hierarchyTree = document.getElementById('hierarchy-tree');
        this.inspectorBody = document.getElementById('inspector-body');
        this.importInput = document.getElementById('import-file-input');
        this.toast = document.getElementById('toast');
        this.statusFps = document.getElementById('status-fps');
        this.statusObjects = document.getElementById('status-objects');
        this.statusFaces = document.getElementById('status-faces');
        this.statusMode = document.getElementById('status-mode');
        this.btnTheme = document.getElementById('btn-theme');
        this.btnGizmo = document.getElementById('btn-gizmo');

        this._toastTimer = null;
        this._fpsTimes = [];
        this._lastFrameTime = performance.now();

        this.bindEvents();
        this.updateHierarchy();
        this.updateInspector();
    }

    bindEvents() {
        // 工具栏按钮
        this.toolbar.addEventListener('click', (e) => {
            const btn = e.target.closest('[data-action]');
            if (!btn) return;
            this._handleAction(btn.dataset.action);
        });

        // 导入文件
        this.importInput.addEventListener('change', (e) => {
            if (e.target.files.length > 0) {
                this._handleImport(e.target.files[0]);
                this.importInput.value = '';
            }
        });

        // 主题切换
        this.btnTheme.addEventListener('click', () => {
            document.body.classList.toggle('dark-mode');
            const icon = this.btnTheme.querySelector('i');
            if (document.body.classList.contains('dark-mode')) {
                icon.className = 'fas fa-sun';
            } else {
                icon.className = 'fas fa-moon';
            }
        });

        // 画布点击选择
        const canvas = this.sm.canvas;
        canvas.addEventListener('click', (e) => {
            // 只响应左键点击（非拖拽）
            if (e.button !== 0) return;
            this._pickObject(e.clientX, e.clientY);
        });

        // 键盘快捷键
        document.addEventListener('keydown', (e) => {
            if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT') return;
            switch (e.key) {
                case 'Delete':
                case 'Backspace':
                    this.sm.deleteSelected();
                    this.refresh();
                    this.showToast('已删除选中对象', 'success');
                    break;
                case 'f':
                    this.sm.focusOnSelected();
                    this.refresh();
                    break;
                case 'g':
                    this._handleAction('toggle-gizmo');
                    break;
                case 'w': this.sm.gizmoMode = 'translate'; this._updateGizmoBtn(); break;
                case 'e': this.sm.gizmoMode = 'rotate'; this._updateGizmoBtn(); break;
                case 'r': this.sm.gizmoMode = 'scale'; this._updateGizmoBtn(); break;
            }
        });
    }

    _handleAction(action) {
        switch (action) {
            case 'add-cube':     this.sm.addPrimitive('cube'); break;
            case 'add-sphere':   this.sm.addPrimitive('sphere'); break;
            case 'add-cylinder': this.sm.addPrimitive('cylinder'); break;
            case 'add-cone':     this.sm.addPrimitive('cone'); break;
            case 'add-plane':    this.sm.addPrimitive('plane'); break;
            case 'add-torus':    this.sm.addPrimitive('torus'); break;
            case 'delete-selected':
                this.sm.deleteSelected();
                this.showToast('已删除选中对象', 'success');
                break;
            case 'duplicate-selected':
                const dup = this.sm.duplicateSelected();
                if (dup) this.showToast('已复制对象', 'success');
                break;
            case 'focus-selected':
                this.sm.focusOnSelected();
                break;
            case 'toggle-gizmo':
                this.sm.gizmoActive = !this.sm.gizmoActive;
                this._updateGizmoBtn();
                this.showToast(this.sm.gizmoActive ? '变换工具：开' : '变换工具：关', 'success');
                break;
            case 'export-scene':
                AssetManager.exportScene(this.sm.objects, {
                    top: '#' + this.sm.skySphere.material.uniforms.topColor.value.getHexString(),
                    mid: '#' + this.sm.skySphere.material.uniforms.midColor.value.getHexString(),
                    bottom: '#' + this.sm.skySphere.material.uniforms.bottomColor.value.getHexString(),
                });
                this.showToast('方案已导出', 'success');
                break;
            case 'import-scene':
                this.importInput.click();
                break;
            case 'view-top':
                this.cc.setView(new THREE.Vector3(0, 10, 0.01), new THREE.Vector3(0, 0, 0));
                break;
            case 'view-front':
                this.cc.setView(new THREE.Vector3(0, 0, 10), new THREE.Vector3(0, 0, 0));
                break;
            case 'view-right':
                this.cc.setView(new THREE.Vector3(10, 0, 0), new THREE.Vector3(0, 0, 0));
                break;
            case 'view-reset':
                this.cc.setView(new THREE.Vector3(5, 4, 8), new THREE.Vector3(0, 0, 0));
                break;
        }
        this.refresh();
    }

    async _handleImport(file) {
        try {
            const data = await AssetManager.importScene(file);
            this._loadSceneData(data);
            this.showToast(`方案已导入：${data.objects.length} 个对象`, 'success');
        } catch (err) {
            this.showToast('导入失败: ' + err.message, 'error');
        }
    }

    _loadSceneData(data) {
        // 清空现有场景
        while (this.sm.objects.length > 0) {
            this.sm.selected = this.sm.objects[0];
            this.sm.deleteSelected();
        }
        this.sm.nextId = 1;

        // 恢复天空盒
        if (data.skybox) {
            this.sm.setSkyboxColors(data.skybox.top, data.skybox.mid, data.skybox.bottom);
        }

        // 恢复对象
        for (const objData of data.objects) {
            let geo;
            if (objData.geometry) {
                geo = AssetManager.deserializeGeometry(objData.geometry);
            } else {
                geo = new THREE.BoxGeometry(1, 1, 1);
            }

            const mat = this.sm._createMaterial(objData.material);
            const mesh = new THREE.Mesh(geo, mat);
            mesh.castShadow = true;
            mesh.receiveShadow = true;
            mesh.position.set(objData.position.x, objData.position.y, objData.position.z);
            mesh.rotation.set(objData.rotation.x, objData.rotation.y, objData.rotation.z);
            mesh.scale.set(objData.scale.x, objData.scale.y, objData.scale.z);
            mesh.userData = {
                id: objData.id || this.sm.nextId,
                name: objData.name || '导入对象',
                type: objData.type || 'imported',
                primitiveType: objData.primitiveType || null,
                materialData: objData.material,
            };
            if (mesh.userData.id >= this.sm.nextId) this.sm.nextId = mesh.userData.id + 1;
            this.sm.scene.add(mesh);
            this.sm.objects.push(mesh);
        }
        this.refresh();
    }

    _pickObject(clientX, clientY) {
        const rect = this.sm.canvas.getBoundingClientRect();
        const mouse = new THREE.Vector2(
            ((clientX - rect.left) / rect.width) * 2 - 1,
            -((clientY - rect.top) / rect.height) * 2 + 1
        );
        this.sm.raycaster.setFromCamera(mouse, this.sm.camera);
        const intersects = this.sm.raycaster.intersectObjects(this.sm.objects, false);
        if (intersects.length > 0) {
            this.sm.select(intersects[0].object);
        } else {
            this.sm.deselect();
        }
        this.refresh();
    }

    _updateGizmoBtn() {
        if (this.sm.gizmoActive) {
            this.btnGizmo.classList.add('active');
        } else {
            this.btnGizmo.classList.remove('active');
        }
    }

    // ============ 层级树 ============
    updateHierarchy() {
        const tree = this.hierarchyTree;
        tree.innerHTML = '';

        if (this.sm.objects.length === 0) {
            tree.innerHTML = `
                <div class="empty-state">
                    <i class="fas fa-cube"></i>
                    <p>暂无对象</p>
                    <small>使用顶部工具栏添加图元</small>
                </div>`;
            return;
        }

        for (const obj of this.sm.objects) {
            const item = document.createElement('div');
            item.className = 'tree-item';
            if (obj === this.sm.selected) item.classList.add('selected');

            const type = obj.userData.primitiveType || obj.userData.type || 'unknown';
            const icon = PRIMITIVES[type] ? PRIMITIVES[type].icon : 'fa-cube';

            item.innerHTML = `
                <i class="fas ${icon}"></i>
                <span class="tree-name">${this._escape(obj.userData.name || '未命名')}</span>
            `;

            item.addEventListener('click', () => {
                this.sm.select(obj);
                this.refresh();
            });

            // 双击聚焦
            item.addEventListener('dblclick', () => {
                this.sm.select(obj);
                this.sm.focusOnSelected();
                this.refresh();
            });

            tree.appendChild(item);
        }
    }

    // ============ 属性检查器 ============
    updateInspector() {
        const body = this.inspectorBody;
        const obj = this.sm.selected;

        if (!obj) {
            body.innerHTML = `
                <div class="empty-state">
                    <i class="fas fa-mouse-pointer"></i>
                    <p>未选中对象</p>
                    <small>点击场景中的物体或左侧层级树</small>
                </div>`;
            return;
        }

        const p = obj.position;
        const r = obj.rotation;
        const s = obj.scale;
        const mat = obj.material;
        const colorHex = mat && mat.color ? '#' + mat.color.getHexString() : '#888888';

        body.innerHTML = `
            <div class="inspector-section">
                <div class="inspector-section-title">基本信息</div>
                <div class="inspector-row">
                    <label>名称</label>
                    <input type="text" id="insp-name" value="${this._escape(obj.userData.name || '')}" style="flex:1">
                </div>
                <div class="inspector-row">
                    <label>类型</label>
                    <span style="font-size:12px;color:var(--text-dim)">${obj.userData.primitiveType || obj.userData.type || '未知'}</span>
                </div>
            </div>

            <div class="inspector-section">
                <div class="inspector-section-title">位置</div>
                <div class="inspector-row">
                    <label>X</label><input type="number" id="insp-pos-x" value="${p.x.toFixed(2)}" step="0.1">
                    <label>Y</label><input type="number" id="insp-pos-y" value="${p.y.toFixed(2)}" step="0.1">
                    <label>Z</label><input type="number" id="insp-pos-z" value="${p.z.toFixed(2)}" step="0.1">
                </div>
            </div>

            <div class="inspector-section">
                <div class="inspector-section-title">旋转</div>
                <div class="inspector-row">
                    <label>X</label><input type="number" id="insp-rot-x" value="${(r.x * 180 / Math.PI).toFixed(1)}" step="1">
                    <label>Y</label><input type="number" id="insp-rot-y" value="${(r.y * 180 / Math.PI).toFixed(1)}" step="1">
                    <label>Z</label><input type="number" id="insp-rot-z" value="${(r.z * 180 / Math.PI).toFixed(1)}" step="1">
                </div>
            </div>

            <div class="inspector-section">
                <div class="inspector-section-title">缩放</div>
                <div class="inspector-row">
                    <label>X</label><input type="number" id="insp-scl-x" value="${s.x.toFixed(2)}" step="0.1">
                    <label>Y</label><input type="number" id="insp-scl-y" value="${s.y.toFixed(2)}" step="0.1">
                    <label>Z</label><input type="number" id="insp-scl-z" value="${s.z.toFixed(2)}" step="0.1">
                </div>
            </div>

            <div class="inspector-section">
                <div class="inspector-section-title">材质</div>
                <div class="inspector-row">
                    <label>颜色</label>
                    <input type="color" id="insp-color" value="${colorHex}">
                </div>
                <div class="inspector-row">
                    <label>粗糙度</label>
                    <input type="range" id="insp-roughness" min="0" max="1" step="0.01" value="${mat ? mat.roughness : 0.5}">
                    <span style="font-size:11px;width:28px;text-align:right">${mat ? mat.roughness.toFixed(2) : '0.50'}</span>
                </div>
                <div class="inspector-row">
                    <label>金属度</label>
                    <input type="range" id="insp-metalness" min="0" max="1" step="0.01" value="${mat ? mat.metalness : 0.1}">
                    <span style="font-size:11px;width:28px;text-align:right">${mat ? mat.metalness.toFixed(2) : '0.10'}</span>
                </div>
                <div class="inspector-row">
                    <label>透明度</label>
                    <input type="range" id="insp-opacity" min="0" max="1" step="0.01" value="${mat ? mat.opacity : 1}">
                    <span style="font-size:11px;width:28px;text-align:right">${mat ? mat.opacity.toFixed(2) : '1.00'}</span>
                </div>
            </div>

            <div class="inspector-section">
                <div class="inspector-section-title">操作</div>
                <button class="btn-glass btn-glass-primary" id="insp-export-model" style="width:100%;margin-bottom:6px;">
                    <i class="fas fa-file-export"></i> 导出模型
                </button>
                <button class="btn-glass btn-glass-primary" id="insp-export-material" style="width:100%;">
                    <i class="fas fa-palette"></i> 导出材质
                </button>
            </div>
        `;

        // 绑定属性变更事件
        this._bindInspectorEvents(obj);
    }

    _bindInspectorEvents(obj) {
        const nameInput = document.getElementById('insp-name');
        if (nameInput) {
            nameInput.addEventListener('input', () => {
                obj.userData.name = nameInput.value;
                this.updateHierarchy();
            });
        }

        // 位置
        ['pos', 'rot', 'scl'].forEach(prop => {
            ['x', 'y', 'z'].forEach(axis => {
                const el = document.getElementById(`insp-${prop}-${axis}`);
                if (!el) return;
                el.addEventListener('input', () => {
                    const val = parseFloat(el.value) || 0;
                    if (prop === 'pos') obj.position[axis] = val;
                    else if (prop === 'rot') obj.rotation[axis] = val * Math.PI / 180;
                    else if (prop === 'scl') obj.scale[axis] = val;
                });
            });
        });

        // 材质颜色
        const colorInput = document.getElementById('insp-color');
        if (colorInput && obj.material) {
            colorInput.addEventListener('input', () => {
                obj.material.color.set(colorInput.value);
            });
        }

        // 粗糙度
        const roughnessInput = document.getElementById('insp-roughness');
        if (roughnessInput && obj.material) {
            roughnessInput.addEventListener('input', () => {
                obj.material.roughness = parseFloat(roughnessInput.value);
                const span = roughnessInput.nextElementSibling;
                if (span) span.textContent = obj.material.roughness.toFixed(2);
            });
        }

        // 金属度
        const metalnessInput = document.getElementById('insp-metalness');
        if (metalnessInput && obj.material) {
            metalnessInput.addEventListener('input', () => {
                obj.material.metalness = parseFloat(metalnessInput.value);
                const span = metalnessInput.nextElementSibling;
                if (span) span.textContent = obj.material.metalness.toFixed(2);
            });
        }

        // 透明度
        const opacityInput = document.getElementById('insp-opacity');
        if (opacityInput && obj.material) {
            opacityInput.addEventListener('input', () => {
                const val = parseFloat(opacityInput.value);
                obj.material.opacity = val;
                obj.material.transparent = val < 1;
                obj.material.needsUpdate = true;
                const span = opacityInput.nextElementSibling;
                if (span) span.textContent = val.toFixed(2);
            });
        }

        // 导出模型
        const exportModelBtn = document.getElementById('insp-export-model');
        if (exportModelBtn) {
            exportModelBtn.addEventListener('click', () => {
                AssetManager.exportModel(obj);
                this.showToast('模型已导出', 'success');
            });
        }

        // 导出材质
        const exportMatBtn = document.getElementById('insp-export-material');
        if (exportMatBtn) {
            exportMatBtn.addEventListener('click', () => {
                AssetManager.exportMaterial(obj);
                this.showToast('材质已导出', 'success');
            });
        }
    }

    // ============ 状态栏 ============
    updateStatus(fps) {
        this.statusFps.textContent = fps;
        this.statusObjects.textContent = this.sm.objects.length;
        this.statusFaces.textContent = this.sm.getTotalTriangles().toLocaleString();
    }

    // ============ Toast ============
    showToast(msg, type) {
        if (this._toastTimer) clearTimeout(this._toastTimer);
        this.toast.textContent = msg;
        this.toast.className = 'toast visible ' + (type || '');
        this._toastTimer = setTimeout(() => {
            this.toast.classList.remove('visible');
        }, 2000);
    }

    // ============ 刷新全部 ============
    refresh() {
        this.updateHierarchy();
        this.updateInspector();
    }

    _escape(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }
}

// ============ 应用入口 ============
class App {
    constructor() {
        const canvas = document.getElementById('render-canvas');
        this.sceneManager = new SceneManager(canvas);
        this.cameraController = new CameraController(this.sceneManager.camera, canvas);
        this.uiManager = new UIManager(this.sceneManager, this.cameraController);

        this._lastTime = performance.now();
        this._fpsAccum = 0;
        this._fpsCount = 0;
        this._fpsTimer = 0;

        this.animate = this.animate.bind(this);
        this.animate();

        console.log('%c『 星月智能 』轻量渲染引擎 已就绪 ✓', 'color:#6c9bcf;font-size:14px;font-weight:bold');
        this.uiManager.showToast('轻量渲染引擎已就绪', 'success');
    }

    animate() {
        requestAnimationFrame(this.animate);

        const now = performance.now();
        const dt = Math.min((now - this._lastTime) / 1000, 0.1);
        this._lastTime = now;

        // FPS 统计
        this._fpsAccum += 1 / dt;
        this._fpsCount++;
        this._fpsTimer += dt;
        if (this._fpsTimer >= 0.5) {
            const fps = Math.round(this._fpsAccum / this._fpsCount);
            this.uiManager.updateStatus(fps);
            this._fpsAccum = 0;
            this._fpsCount = 0;
            this._fpsTimer = 0;
        }

        // 更新控制器
        this.cameraController.update();

        // 渲染
        this.sceneManager.render();
    }
}

// ============ 启动 ============
document.addEventListener('DOMContentLoaded', () => {
    new App();
});