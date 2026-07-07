import * as THREE from './three.module.js';
import { PRIMITIVES } from './primitives.js';

// ============ 场景管理器 ============
class SceneManager {
    constructor(canvas) {
        this.canvas = canvas;
        this.objects = [];
        this.groups = [];
        this.selected = null;
        this._selectionSet = new Set();
        this.nextId = 1;
        this.texturePool = new Map(); // name -> { name, base64, threeTexture }
        this.keyframes = []; // { index, delay: 1.0, state: { objects: [], lighting: {} } }
        this.isPlaying = false;
        this._playFrameIdx = 0;
        this._interpTimer = 0;

        this.scene = new THREE.Scene();
        this.scene.background = new THREE.Color('#ffffff');

        this.renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true });
        this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
        this.renderer.shadowMap.enabled = true;
        this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
        this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
        this.renderer.toneMappingExposure = 1.0;

        this.camera = new THREE.PerspectiveCamera(55, 2, 0.1, 500);
        this.camera.position.set(5, 4, 8);
        this.camera.lookAt(0, 0, 0);

        this.raycaster = new THREE.Raycaster();

        // 高亮边框线集合：mesh -> LineSegments
        this._highlightLines = new Map();

        this.setupLights();
        this.setupGrid();
        this.setupSkybox();

        this.onResize = this.onResize.bind(this);
        window.addEventListener('resize', this.onResize);
        this.onResize();
    }

    // ============ 光照系统 ============
    setupLights() {
        this.ambientLight = new THREE.AmbientLight(0x404060, 0.6);
        this.scene.add(this.ambientLight);

        this.hemiLight = new THREE.HemisphereLight(0x606080, 0x303040, 0.5);
        this.scene.add(this.hemiLight);

        this.sunLight = new THREE.DirectionalLight(0xfff5e8, 1.2);
        this.sunLight.position.set(10, 15, 8);
        this.sunLight.castShadow = true;
        this.sunLight.shadow.mapSize.set(2048, 2048);
        this.sunLight.shadow.camera.near = 0.5;
        this.sunLight.shadow.camera.far = 100;
        this.sunLight.shadow.camera.left = -25;
        this.sunLight.shadow.camera.right = 25;
        this.sunLight.shadow.camera.top = 25;
        this.sunLight.shadow.camera.bottom = -25;
        this.sunLight.shadow.bias = -0.0003;
        this.sunLight.shadow.normalBias = 0.04;
        this.scene.add(this.sunLight);
        this.scene.add(this.sunLight.target);
    }

    setAmbientIntensity(v) { this.ambientLight.intensity = v; }
    setHemiIntensity(v) { this.hemiLight.intensity = v; }
    setSunIntensity(v) { this.sunLight.intensity = v; }
    setSunDirection(x, y, z) {
        this.sunLight.position.set(x, y, z);
        this.sunLight.target.position.set(0, 0, 0);
    }
    setShadowsEnabled(enabled) {
        this.sunLight.castShadow = enabled;
        this.renderer.shadowMap.enabled = enabled;
    }

    // ============ 地面 ============
    setupGrid() {
        this._gridColor = '#444466';
        this.gridHelper = new THREE.GridHelper(20, 20, 0x444466, 0x222244);
        this.scene.add(this.gridHelper);

        this.groundPlane = new THREE.Mesh(
            new THREE.PlaneGeometry(20, 20),
            new THREE.ShadowMaterial({ opacity: 0.3 })
        );
        this.groundPlane.rotation.x = -Math.PI / 2;
        this.groundPlane.receiveShadow = true;
        this.scene.add(this.groundPlane);
    }

    setGridVisible(v) { this.gridHelper.visible = v; }
    setGroundVisible(v) { this.groundPlane.visible = v; }
    setGroundSize(size) {
        this.scene.remove(this.gridHelper);
        this.scene.remove(this.groundPlane);
        const c = new THREE.Color(this._gridColor);
        this.gridHelper = new THREE.GridHelper(size, size, c.getHex(), c.getHex());
        this.scene.add(this.gridHelper);
        this.groundPlane.geometry.dispose();
        this.groundPlane.geometry = new THREE.PlaneGeometry(size, size);
        this.groundPlane.receiveShadow = true;
        this.scene.add(this.groundPlane);
    }
    setGridColor(hex) {
        this._gridColor = hex;
        const c = new THREE.Color(hex);
        this.scene.remove(this.gridHelper);
        const size = this.gridHelper.geometry.parameters?.size || 20;
        const div = this.gridHelper.geometry.parameters?.divisions || 20;
        this.gridHelper = new THREE.GridHelper(size, div, c.getHex(), c.getHex());
        this.scene.add(this.gridHelper);
    }
    get gridColor() { return this._gridColor; }

    // ============ 天空盒 ============
    setupSkybox() {
        const skyGeo = new THREE.SphereGeometry(100, 32, 16);
        const skyMat = new THREE.ShaderMaterial({
            side: THREE.BackSide, depthWrite: false,
            uniforms: {
                topColor: { value: new THREE.Color('#ffffff') },
                midColor: { value: new THREE.Color('#ffffff') },
                bottomColor: { value: new THREE.Color('#ffffff') },
            },
            vertexShader: `
                varying vec3 vWorldPos;
                void main() {
                    vec4 wp = modelMatrix * vec4(position, 1.0);
                    vWorldPos = wp.xyz;
                    gl_Position = projectionMatrix * viewMatrix * wp;
                }`,
            fragmentShader: `
                uniform vec3 topColor, midColor, bottomColor;
                varying vec3 vWorldPos;
                void main() {
                    float h = normalize(vWorldPos).y;
                    vec3 col = h > 0.0 ? mix(midColor, topColor, h) : mix(midColor, bottomColor, -h);
                    gl_FragColor = vec4(col, 1.0);
                }`
        });
        this.skySphere = new THREE.Mesh(skyGeo, skyMat);
        this.scene.add(this.skySphere);
    }

    setSkyboxColors(top, mid, bottom) {
        this.skySphere.material.uniforms.topColor.value.set(top);
        this.skySphere.material.uniforms.midColor.value.set(mid);
        this.skySphere.material.uniforms.bottomColor.value.set(bottom);
    }

    // ============ 高亮边框 ============
    _createHighlightLine(mesh) {
        const color = new THREE.Color().setHSL(Math.random(), 0.8, 0.55);
        mesh.userData.highlightColor = '#' + color.getHexString();
        const mat = new THREE.LineBasicMaterial({
            color: color,
            linewidth: 1,
            depthTest: false,
            transparent: true,
            opacity: 0.9,
        });
        let line;
        if (mesh.isGroup || !mesh.geometry) {
            mesh.updateWorldMatrix(true, true);
            const box3 = new THREE.Box3().setFromObject(mesh);
            if (box3.isEmpty()) {
                const geo = new THREE.BoxGeometry(0.2, 0.2, 0.2);
                line = new THREE.LineSegments(new THREE.EdgesGeometry(geo), mat);
            } else {
                const size = new THREE.Vector3(); box3.getSize(size);
                const center = new THREE.Vector3(); box3.getCenter(center);
                mesh.worldToLocal(center);
                const geo = new THREE.BoxGeometry(size.x, size.y, size.z);
                line = new THREE.LineSegments(new THREE.EdgesGeometry(geo), mat);
                line.position.copy(center);
            }
        } else {
            const edges = new THREE.EdgesGeometry(mesh.geometry);
            line = new THREE.LineSegments(edges, mat);
        }
        line.renderOrder = 999;
        line.material.depthTest = false;
        line.raycast = () => { };
        return line;
    }

    _addHighlight(mesh) {
        if (this._highlightLines.has(mesh)) return;
        const line = this._createHighlightLine(mesh);
        mesh.add(line);
        this._highlightLines.set(mesh, line);
    }

    _removeHighlight(mesh) {
        const line = this._highlightLines.get(mesh);
        if (line) {
            line.geometry?.dispose();
            line.material?.dispose();
            mesh.remove(line);
            this._highlightLines.delete(mesh);
        }
    }

    _clearAllHighlights() {
        for (const [mesh, line] of this._highlightLines) {
            line.geometry?.dispose();
            line.material?.dispose();
            mesh.remove(line);
        }
        this._highlightLines.clear();
    }

    // ============ 残留图元清理 ============
    _cleanupUnnamed() {
        const toRemove = [];
        for (const obj of this.objects) {
            if (obj.userData.type === 'group') {
                const children = [...obj.children];
                for (const child of children) {
                    if (!child.userData.id) continue;
                    if (!child.userData.name || child.userData.name === '未命名') {
                        obj.remove(child);
                        if (child.geometry) child.geometry.dispose();
                        if (child.material) {
                            if (Array.isArray(child.material)) {
                                child.material.forEach(m => m.dispose());
                            } else {
                                child.material.dispose();
                            }
                        }
                    }
                }
            } else if (obj.userData.id && (!obj.userData.name || obj.userData.name === '未命名')) {
                toRemove.push(obj);
            }
        }
        for (const obj of toRemove) {
            this.scene.remove(obj);
            if (obj.geometry) obj.geometry.dispose();
            if (obj.material) {
                if (Array.isArray(obj.material)) {
                    obj.material.forEach(m => m.dispose());
                } else {
                    obj.material.dispose();
                }
            }
            const idx = this.objects.indexOf(obj);
            if (idx !== -1) this.objects.splice(idx, 1);
        }
    }

    // ============ 对象管理 ============
    addPrimitive(type, params = {}) {
        const def = PRIMITIVES[type];
        if (!def) return null;
        const geo = def.geo(params);

        if (type === 'cone' && geo.groups) {
            for (const group of geo.groups) {
                if (group.materialIndex === 2) group.materialIndex = 1;
            }
        }

        let mat;
        let faceNames = null;
        let faceVisible = null;

        if (def.multiFace === true) {
            const mats = [];
            for (let i = 0; i < 6; i++) {
                mats.push(new THREE.MeshStandardMaterial({ color: this._randomColor(), roughness: 0.5, metalness: 0.1 }));
            }
            mat = mats;
            faceNames = ['右', '左', '上', '下', '前', '后'];
            faceVisible = [true, true, true, true, true, true];
        } else if (Array.isArray(def.multiFace)) {
            const n = def.multiFace.length;
            const mats = [];
            for (let i = 0; i < n; i++) {
                mats.push(new THREE.MeshStandardMaterial({ color: this._randomColor(), roughness: 0.5, metalness: 0.1 }));
            }
            mat = mats;
            faceNames = def.multiFace;
            faceVisible = new Array(n).fill(true);
        } else {
            mat = new THREE.MeshStandardMaterial({ color: this._randomColor(), roughness: 0.5, metalness: 0.1 });
        }

        const mesh = new THREE.Mesh(geo, mat);
        mesh.castShadow = true;
        mesh.receiveShadow = true;
        mesh.position.set(0, 0.5, 0);
        mesh.userData = {
            id: this.nextId++,
            name: `${def.name} ${this.nextId - 1}`,
            type: type,
            primitiveType: type,
            primitiveParams: { ...params },
            faceNames: faceNames,
            faceVisible: faceVisible,
            _faceMaterials: faceNames ? (Array.isArray(mat) ? mat.map(m => m.clone()) : null) : null,
            physics: { enabled: false, anchored: false, autoRotate: { enabled: false, axis: 'y', speed: 1 }, initialKinetic: { enabled: false, velocity: { x: 0, y: 0, z: 0 } }, attraction: { enabled: false, targetId: '', strength: 1 }, repulsion: { enabled: false, targetId: '', strength: 1 } },
        };
        this.scene.add(mesh);
        this.objects.push(mesh);
        this.select(mesh);
        return mesh;
    }

    select(mesh) {
        if (this.selected === mesh) return;
        const prevSelected = this.selected;
        this._clearAllHighlights();
        this._selectionSet.clear();
        this._selectionSet.add(mesh);
        this.selected = mesh;
        this._addHighlight(mesh);
        if (prevSelected && prevSelected.userData.type === 'group') {
            setTimeout(() => this._cleanupUnnamed(), 100);
        }
    }

    _toggleSelection(mesh) {
        if (this._selectionSet.has(mesh)) {
            this._selectionSet.delete(mesh);
            this._removeHighlight(mesh);
            if (this.selected === mesh) {
                this.selected = this._selectionSet.size > 0 ? [...this._selectionSet][this._selectionSet.size - 1] : null;
            }
        } else {
            this._selectionSet.add(mesh);
            this._addHighlight(mesh);
            this.selected = mesh;
        }
    }

    deselect() {
        const prevSelected = this.selected;
        this._clearAllHighlights();
        this._selectionSet.clear();
        this.selected = null;
        if (prevSelected && prevSelected.userData.type === 'group') {
            setTimeout(() => this._cleanupUnnamed(), 100);
        }
    }

    deleteSelected() {
        if (!this.selected) return;
        const obj = this.selected;

        if (obj.userData.type === 'group') {
            const children = [...obj.children];
            this.deselect();
            for (const child of children) {
                obj.remove(child);
                if (child.geometry) child.geometry.dispose();
                if (child.material) {
                    if (Array.isArray(child.material)) {
                        child.material.forEach(m => m.dispose());
                    } else {
                        child.material.dispose();
                    }
                }
            }
            this.scene.remove(obj);
            const idx = this.objects.indexOf(obj);
            if (idx !== -1) this.objects.splice(idx, 1);
            const gidx = this.groups.indexOf(obj);
            if (gidx !== -1) this.groups.splice(gidx, 1);
        } else {
            this.deselect();
            this.scene.remove(obj);
            if (obj.geometry) obj.geometry.dispose();
            if (obj.material) {
                if (Array.isArray(obj.material)) {
                    obj.material.forEach(m => m.dispose());
                } else {
                    obj.material.dispose();
                }
            }
            const idx = this.objects.indexOf(obj);
            if (idx !== -1) this.objects.splice(idx, 1);
        }
    }

    duplicateSelected() {
        if (!this.selected) return null;
        const src = this.selected;

        if (src.userData.type === 'group') {
            const group = new THREE.Group();
            group.position.copy(src.position).add(new THREE.Vector3(0.8, 0, 0.8));
            group.rotation.copy(src.rotation);
            group.scale.copy(src.scale);
            group.userData = {
                id: this.nextId++,
                name: `${src.userData.name} (副本)`,
                type: 'group',
                children: [],
                physics: { enabled: false, anchored: false, autoRotate: { enabled: false, axis: 'y', speed: 1 }, initialKinetic: { enabled: false, velocity: { x: 0, y: 0, z: 0 } }, attraction: { enabled: false, targetId: '', strength: 1 }, repulsion: { enabled: false, targetId: '', strength: 1 } },
            };
            for (const child of src.children) {
                const childGeo = child.geometry.clone();
                const childMat = Array.isArray(child.material)
                    ? child.material.map(m => m.clone())
                    : child.material.clone();
                const childMesh = new THREE.Mesh(childGeo, childMat);
                childMesh.castShadow = true; childMesh.receiveShadow = true;
                childMesh.position.copy(child.position);
                childMesh.rotation.copy(child.rotation);
                childMesh.scale.copy(child.scale);
                childMesh.userData = { ...child.userData, id: this.nextId++ };
                group.add(childMesh);
                group.userData.children.push(childMesh.userData.id);
            }
            this.scene.add(group);
            this.objects.push(group);
            this.groups.push(group);
            this.select(group);
            return group;
        }

        const geo = src.geometry.clone();
        const mat = Array.isArray(src.material)
            ? src.material.map(m => m.clone())
            : src.material.clone();
        const mesh = new THREE.Mesh(geo, mat);
        mesh.castShadow = true; mesh.receiveShadow = true;
        mesh.position.copy(src.position).add(new THREE.Vector3(0.8, 0, 0.8));
        mesh.rotation.copy(src.rotation);
        mesh.scale.copy(src.scale);
        mesh.userData = { ...src.userData, id: this.nextId++, name: `${src.userData.name} (副本)` };
        if (src.userData._faceMaterials) {
            mesh.userData._faceMaterials = Array.isArray(mat) ? mat.map(m => m.clone()) : null;
        }
        this.scene.add(mesh);
        this.objects.push(mesh);
        this.select(mesh);
        return mesh;
    }

    // ============ 组合体管理 ============
    groupSelected() {
        if (this._selectionSet.size < 2) return null;
        const group = new THREE.Group();

        const box = new THREE.Box3();
        const selectedObjs = [...this._selectionSet];
        for (const obj of selectedObjs) {
            box.expandByObject(obj);
        }
        const center = new THREE.Vector3();
        box.getCenter(center);

        group.position.copy(center);
        group.userData = {
            id: this.nextId++,
            name: `组合体 ${this.nextId - 1}`,
            type: 'group',
            children: [],
            physics: { enabled: false, anchored: false, autoRotate: { enabled: false, axis: 'y', speed: 1 }, initialKinetic: { enabled: false, velocity: { x: 0, y: 0, z: 0 } }, attraction: { enabled: false, targetId: '', strength: 1 }, repulsion: { enabled: false, targetId: '', strength: 1 } },
        };

        this._clearAllHighlights();
        this._selectionSet.clear();

        for (const obj of selectedObjs) {
            this.scene.remove(obj);
            const idx = this.objects.indexOf(obj);
            if (idx !== -1) this.objects.splice(idx, 1);
            obj.position.sub(center);
            group.add(obj);
            group.userData.children.push(obj.userData.id);
            if (obj.material) {
                if (Array.isArray(obj.material)) {
                    obj.material.forEach(m => { m.side = THREE.FrontSide; m.needsUpdate = true; });
                } else {
                    obj.material.side = THREE.FrontSide;
                    obj.material.needsUpdate = true;
                }
            }
        }

        this.scene.add(group);
        this.objects.push(group);
        this.groups.push(group);
        this.select(group);
        return group;
    }

    ungroupSelected() {
        if (!this.selected || this.selected.userData.type !== 'group') return null;
        const group = this.selected;
        this.deselect();
        const children = [...group.children];

        for (const child of children) {
            const worldPos = new THREE.Vector3();
            child.getWorldPosition(worldPos);
            const worldQuat = new THREE.Quaternion();
            child.getWorldQuaternion(worldQuat);
            const worldScale = new THREE.Vector3();
            child.getWorldScale(worldScale);

            group.remove(child);
            child.position.copy(worldPos);
            child.quaternion.copy(worldQuat);
            child.scale.copy(worldScale);

            this.scene.add(child);
            this.objects.push(child);
        }

        this.scene.remove(group);
        const idx = this.objects.indexOf(group);
        if (idx !== -1) this.objects.splice(idx, 1);
        const gidx = this.groups.indexOf(group);
        if (gidx !== -1) this.groups.splice(gidx, 1);

        return children;
    }

    getTotalTriangles() {
        let count = 0;
        const countObj = (obj) => {
            if (obj.geometry && obj.geometry.index) count += obj.geometry.index.count / 3;
            else if (obj.geometry && obj.geometry.attributes.position) count += obj.geometry.attributes.position.count / 3;
        };
        for (const obj of this.objects) {
            if (obj.userData.type === 'group') {
                obj.children.forEach(child => countObj(child));
            } else {
                countObj(obj);
            }
        }
        return Math.floor(count);
    }

    // ============ 纹理池 ============
    addTexture(name, base64) {
        let finalName = name;
        let counter = 2;
        while (this.texturePool.has(finalName)) {
            finalName = `${name} (${counter})`;
            counter++;
        }
        const img = new Image();
        img.src = base64;
        const tex = new THREE.Texture(img);
        tex.wrapS = tex.wrapT = THREE.RepeatWrapping;
        tex.colorSpace = THREE.SRGBColorSpace;
        tex.magFilter = THREE.LinearFilter;
        tex.minFilter = THREE.LinearMipmapLinearFilter;
        img.onload = () => { tex.needsUpdate = true; };
        this.texturePool.set(finalName, { name: finalName, base64, threeTexture: tex });
        return tex;
    }

    removeTexture(name) {
        const entry = this.texturePool.get(name);
        if (entry) { entry.threeTexture.dispose(); this.texturePool.delete(name); }
    }

    applyTextureToSelected(textureName) {
        if (!this.selected || !this.selected.material) return false;
        const entry = this.texturePool.get(textureName);
        if (!entry) return false;
        if (Array.isArray(this.selected.material)) {
            this.selected.material.forEach(m => { m.map = entry.threeTexture; m.needsUpdate = true; });
        } else {
            this.selected.material.map = entry.threeTexture;
            this.selected.material.needsUpdate = true;
        }
        this.selected.userData.textureName = textureName;
        return true;
    }

    removeTextureFromSelected() {
        if (!this.selected || !this.selected.material) return;
        if (Array.isArray(this.selected.material)) {
            this.selected.material.forEach(m => { m.map = null; m.needsUpdate = true; });
        } else {
            this.selected.material.map = null;
            this.selected.material.needsUpdate = true;
        }
        this.selected.userData.textureName = null;
    }

    // ============ 渲染 ============
    onResize() {
        this.camera.aspect = window.innerWidth / window.innerHeight;
        this.camera.updateProjectionMatrix();
        this.renderer.setSize(window.innerWidth, window.innerHeight);
    }

    render() {
        this.renderer.render(this.scene, this.camera);
    }

    dispose() {
        window.removeEventListener('resize', this.onResize);
        for (const obj of this.objects) {
            if (obj.userData.type === 'group') {
                obj.children.forEach(child => {
                    if (child.geometry) child.geometry.dispose();
                    if (child.material) {
                        if (Array.isArray(child.material)) {
                            child.material.forEach(m => m.dispose());
                        } else {
                            child.material.dispose();
                        }
                    }
                });
            } else {
                if (obj.geometry) obj.geometry.dispose();
                if (obj.material) {
                    if (Array.isArray(obj.material)) {
                        obj.material.forEach(m => m.dispose());
                    } else {
                        obj.material.dispose();
                    }
                }
            }
        }
        for (const [, entry] of this.texturePool) entry.threeTexture.dispose();
        this.renderer.dispose();
    }

    _randomColor() {
        return new THREE.Color().setHSL(Math.random(), 0.4 + Math.random() * 0.3, 0.35 + Math.random() * 0.25);
    }

    _materialToData(mat) {
        if (!mat || !mat.isMaterial) return null;
        const data = {
            color: '#' + mat.color.getHexString(),
            roughness: mat.roughness, metalness: mat.metalness,
            opacity: mat.opacity, transparent: mat.transparent,
            type: mat.isMeshStandardMaterial ? 'standard' : 'basic',
        };
        if (mat.map) {
            const entry = [...this.texturePool.values()].find(e => e.threeTexture === mat.map);
            if (entry) data.textureName = entry.name;
            else data.texture = this._textureToBase64(mat.map);
        }
        return data;
    }

    _createMaterial(data) {
        if (!data) return new THREE.MeshStandardMaterial({ color: 0x888888, roughness: 0.5, metalness: 0.1 });
        const isBasic = data.type === 'basic';
        const opts = {
            color: new THREE.Color(data.color || '#888888'),
            opacity: data.opacity ?? 1, transparent: data.transparent ?? false,
            side: data.side ?? 0,
            flatShading: data.flatShading ?? false,
            wireframe: data.wireframe ?? false,
        };
        if (!isBasic) {
            opts.roughness = data.roughness ?? 0.5;
            opts.metalness = data.metalness ?? 0.1;
        }
        if (data.textureName) {
            const entry = this.texturePool.get(data.textureName);
            if (entry) opts.map = entry.threeTexture;
        } else if (data.texture) {
            opts.map = this._base64ToTexture(data.texture);
        }
        return isBasic ? new THREE.MeshBasicMaterial(opts) : new THREE.MeshStandardMaterial(opts);
    }

    _textureToBase64(texture) {
        if (!texture || !texture.image) return null;
        const canvas = document.createElement('canvas');
        canvas.width = texture.image.width; canvas.height = texture.image.height;
        canvas.getContext('2d').drawImage(texture.image, 0, 0);
        return canvas.toDataURL('image/png');
    }

    _base64ToTexture(base64) {
        const img = new Image(); img.src = base64;
        const tex = new THREE.Texture(img);
        tex.wrapS = tex.wrapT = THREE.RepeatWrapping;
        tex.colorSpace = THREE.SRGBColorSpace;
        img.onload = () => tex.needsUpdate = true;
        return tex;
    }

    // ============ 关键帧系统 ============
    captureState() {
        const state = {
            objects: this.objects.map(obj => ({
                id: obj.userData.id,
                name: obj.userData.name,
                pos: { x: obj.position.x, y: obj.position.y, z: obj.position.z },
                rot: { x: obj.rotation.x, y: obj.rotation.y, z: obj.rotation.z },
                scl: { x: obj.scale.x, y: obj.scale.y, z: obj.scale.z },
                physics: obj.userData.physics ? JSON.parse(JSON.stringify(obj.userData.physics)) : null,
            })),
            lighting: {
                sunDir: { x: this.sunLight.position.x, y: this.sunLight.position.y, z: this.sunLight.position.z },
                ambient: this.ambientLight.intensity,
                hemi: this.hemiLight.intensity,
                sun: this.sunLight.intensity,
            },
        };
        return state;
    }

    addKeyframe() {
        const idx = this.keyframes.length;
        const currState = this.captureState();
        if (idx === 0) {
            this.keyframes.push({ index: 0, delay: 1.0, state: currState });
            return 0;
        }
        const prevState = this.keyframes[idx - 1].state;
        const changedObjects = [];
        for (const obj of currState.objects) {
            const prev = prevState.objects.find(o => o.id === obj.id);
            if (!prev) {
                changedObjects.push(obj);
                continue;
            }
            const changed = obj.pos.x !== prev.pos.x || obj.pos.y !== prev.pos.y || obj.pos.z !== prev.pos.z
                || obj.rot.x !== prev.rot.x || obj.rot.y !== prev.rot.y || obj.rot.z !== prev.rot.z
                || obj.scl.x !== prev.scl.x || obj.scl.y !== prev.scl.y || obj.scl.z !== prev.scl.z
                || JSON.stringify(obj.physics) !== JSON.stringify(prev.physics);
            if (changed) changedObjects.push(obj);
        }
        const prevLight = prevState.lighting;
        const currLight = currState.lighting;
        const lightChanged = !prevLight || !prevLight.sunDir || (
            currLight.sunDir.x !== prevLight.sunDir.x
            || currLight.sunDir.y !== prevLight.sunDir.y
            || currLight.sunDir.z !== prevLight.sunDir.z
            || currLight.ambient !== prevLight.ambient
            || currLight.hemi !== prevLight.hemi
            || currLight.sun !== prevLight.sun
        );
        this.keyframes.push({
            index: idx,
            delay: 1.0,
            state: {
                objects: changedObjects,
                lighting: lightChanged ? currLight : null,
            },
        });
        return idx;
    }

    deleteKeyframe(index) {
        this.keyframes.splice(index, 1);
        this.keyframes.forEach((kf, i) => kf.index = i);
    }

    applyKeyframeState(index) {
        if (index < 0 || index >= this.keyframes.length) return;
        for (let i = 0; i <= index; i++) {
            const kf = this.keyframes[i];
            const st = kf.state;
            if (st.objects && st.objects.length > 0) {
                for (const od of st.objects) {
                    const obj = this.objects.find(o => o.userData.id === od.id);
                    if (obj) {
                        obj.position.set(od.pos.x, od.pos.y, od.pos.z);
                        obj.rotation.set(od.rot.x, od.rot.y, od.rot.z);
                        obj.scale.set(od.scl.x, od.scl.y, od.scl.z);
                        if (od.physics) obj.userData.physics = JSON.parse(JSON.stringify(od.physics));
                    }
                }
            }
            if (st.lighting && st.lighting.sunDir) {
                this.sunLight.position.set(st.lighting.sunDir.x, st.lighting.sunDir.y, st.lighting.sunDir.z);
                this.ambientLight.intensity = st.lighting.ambient;
                this.hemiLight.intensity = st.lighting.hemi;
                this.sunLight.intensity = st.lighting.sun;
            }
        }
    }

    resolveState(index) {
        const state = { objects: new Map(), lighting: null };
        for (let i = 0; i <= index; i++) {
            const kf = this.keyframes[i];
            const st = kf.state;
            if (st.objects) {
                for (const od of st.objects) {
                    state.objects.set(od.id, { ...od });
                }
            }
            if (st.lighting && st.lighting.sunDir) {
                state.lighting = { ...st.lighting };
            }
        }
        return state;
    }

    interpolateFrames(fromIdx, toIdx, t) {
        const _lerp = (a, b, t) => a + (b - a) * t;
        const fromState = this.resolveState(fromIdx);
        const toState = this.resolveState(toIdx);
        for (const [id, toObj] of toState.objects) {
            const fromObj = fromState.objects.get(id);
            const obj = this.objects.find(o => o.userData.id === id);
            if (!obj) continue;
            if (fromObj) {
                obj.position.set(
                    _lerp(fromObj.pos.x, toObj.pos.x, t),
                    _lerp(fromObj.pos.y, toObj.pos.y, t),
                    _lerp(fromObj.pos.z, toObj.pos.z, t)
                );
                obj.rotation.set(
                    _lerp(fromObj.rot.x, toObj.rot.x, t),
                    _lerp(fromObj.rot.y, toObj.rot.y, t),
                    _lerp(fromObj.rot.z, toObj.rot.z, t)
                );
                obj.scale.set(
                    _lerp(fromObj.scl.x, toObj.scl.x, t),
                    _lerp(fromObj.scl.y, toObj.scl.y, t),
                    _lerp(fromObj.scl.z, toObj.scl.z, t)
                );
            } else {
                obj.position.set(toObj.pos.x, toObj.pos.y, toObj.pos.z);
                obj.rotation.set(toObj.rot.x, toObj.rot.y, toObj.rot.z);
                obj.scale.set(toObj.scl.x, toObj.scl.y, toObj.scl.z);
            }
            // 物理参数 snap 到目标帧（非空间属性不插值）
            if (toObj.physics) {
                obj.userData.physics = JSON.parse(JSON.stringify(toObj.physics));
            }
        }
        if (fromState.lighting && toState.lighting) {
            const fl = fromState.lighting, tl = toState.lighting;
            this.sunLight.position.set(
                _lerp(fl.sunDir.x, tl.sunDir.x, t),
                _lerp(fl.sunDir.y, tl.sunDir.y, t),
                _lerp(fl.sunDir.z, tl.sunDir.z, t)
            );
            this.ambientLight.intensity = _lerp(fl.ambient, tl.ambient, t);
            this.hemiLight.intensity = _lerp(fl.hemi, tl.hemi, t);
            this.sunLight.intensity = _lerp(fl.sun, tl.sun, t);
        }
    }
}

export { SceneManager };