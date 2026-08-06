import * as THREE from '../vendor/three.module.js';

// ============ 图元定义 ============
const PRIMITIVES = {
    cube: {
        geo: (p) => new THREE.BoxGeometry(p.w || 1, p.h || 1, p.d || 1),
        icon: 'fa-cube', name: '立方体',
        params: [
            { key: 'w', label: '宽度', min: 0.1, max: 10, step: 0.1, default: 1 },
            { key: 'h', label: '高度', min: 0.1, max: 10, step: 0.1, default: 1 },
            { key: 'd', label: '深度', min: 0.1, max: 10, step: 0.1, default: 1 },
        ], multiFace: true
    },
    sphere: {
        geo: (p) => new THREE.SphereGeometry(p.r || 0.5, p.seg || 32, p.seg2 || 32),
        icon: 'fa-circle', name: '球体',
        params: [
            { key: 'r', label: '半径', min: 0.1, max: 10, step: 0.1, default: 0.5 },
            { key: 'seg', label: '分段', min: 4, max: 128, step: 1, default: 32 },
        ]
    },
    cylinder: {
        geo: (p) => new THREE.CylinderGeometry(p.rt || 0.5, p.rb || 0.5, p.h || 1, p.seg || 32),
        icon: 'fa-database', name: '圆柱体',
        params: [
            { key: 'rt', label: '顶半径', min: 0, max: 5, step: 0.1, default: 0.5 },
            { key: 'rb', label: '底半径', min: 0, max: 5, step: 0.1, default: 0.5 },
            { key: 'h', label: '高度', min: 0.1, max: 10, step: 0.1, default: 1 },
            { key: 'seg', label: '分段', min: 4, max: 128, step: 1, default: 32 },
        ], multiFace: ['侧面', '顶面', '底面']
    },
    cone: {
        geo: (p) => new THREE.ConeGeometry(p.r || 0.5, p.h || 1, p.seg || 32),
        icon: 'fa-traffic-cone', name: '圆锥体',
        params: [
            { key: 'r', label: '半径', min: 0.1, max: 5, step: 0.1, default: 0.5 },
            { key: 'h', label: '高度', min: 0.1, max: 10, step: 0.1, default: 1 },
            { key: 'seg', label: '分段', min: 4, max: 128, step: 1, default: 32 },
        ], multiFace: ['侧面', '底面']
    },
    plane: {
        geo: (p) => new THREE.PlaneGeometry(p.w || 1, p.h || 1),
        icon: 'fa-square', name: '平面',
        params: [
            { key: 'w', label: '宽度', min: 0.1, max: 20, step: 0.1, default: 1 },
            { key: 'h', label: '高度', min: 0.1, max: 20, step: 0.1, default: 1 },
        ]
    },
    torus: {
        geo: (p) => new THREE.TorusGeometry(p.r || 0.5, p.t || 0.2, p.rSeg || 16, p.tSeg || 32),
        icon: 'fa-donut', name: '圆环',
        params: [
            { key: 'r', label: '大半径', min: 0.1, max: 5, step: 0.1, default: 0.5 },
            { key: 't', label: '管半径', min: 0.02, max: 2, step: 0.02, default: 0.2 },
            { key: 'rSeg', label: '环分段', min: 4, max: 64, step: 1, default: 16 },
            { key: 'tSeg', label: '管分段', min: 4, max: 64, step: 1, default: 32 },
        ]
    },
    dodecahedron: {
        geo: (p) => new THREE.DodecahedronGeometry(p.r || 0.5, p.detail || 0),
        icon: 'fa-shapes', name: '十二面体',
        params: [
            { key: 'r', label: '半径', min: 0.1, max: 5, step: 0.1, default: 0.5 },
            { key: 'detail', label: '细节', min: 0, max: 3, step: 1, default: 0 },
        ]
    },
    octahedron: {
        geo: (p) => new THREE.OctahedronGeometry(p.r || 0.5, p.detail || 0),
        icon: 'fa-gem', name: '八面体',
        params: [
            { key: 'r', label: '半径', min: 0.1, max: 5, step: 0.1, default: 0.5 },
            { key: 'detail', label: '细节', min: 0, max: 3, step: 1, default: 0 },
        ]
    },
    tetrahedron: {
        geo: (p) => new THREE.TetrahedronGeometry(p.r || 0.5, p.detail || 0),
        icon: 'fa-play', name: '四面体',
        params: [
            { key: 'r', label: '半径', min: 0.1, max: 5, step: 0.1, default: 0.5 },
            { key: 'detail', label: '细节', min: 0, max: 3, step: 1, default: 0 },
        ]
    },
    torusKnot: {
        geo: (p) => new THREE.TorusKnotGeometry(p.r || 0.5, p.t || 0.15, p.tSeg || 64, p.rSeg || 8, p.p || 2, p.q || 3),
        icon: 'fa-infinity', name: '环结',
        params: [
            { key: 'r', label: '大半径', min: 0.1, max: 5, step: 0.1, default: 0.5 },
            { key: 't', label: '管半径', min: 0.02, max: 2, step: 0.02, default: 0.15 },
            { key: 'p', label: 'P 缠绕', min: 1, max: 10, step: 1, default: 2 },
            { key: 'q', label: 'Q 缠绕', min: 1, max: 10, step: 1, default: 3 },
        ]
    },
    ring: {
        geo: (p) => new THREE.RingGeometry(p.inner || 0.3, p.outer || 0.5, p.seg || 32),
        icon: 'fa-circle-notch', name: '圆环面',
        params: [
            { key: 'inner', label: '内半径', min: 0, max: 5, step: 0.05, default: 0.3 },
            { key: 'outer', label: '外半径', min: 0.1, max: 5, step: 0.05, default: 0.5 },
            { key: 'seg', label: '分段', min: 4, max: 128, step: 1, default: 32 },
        ]
    },
};

// ============ 图元管理器 ============
/**
 * Primitives — 图元创建与管理
 * 负责在 renderer.primitivesRoot 下创建、移除、查询图元网格
 * 由 engine.js 在初始化时构造，panels/elements 面板通过 BroadcastChannel 调用
 */
class Primitives {
    /**
     * @param {import('./renderer.js').Renderer} renderer
     */
    constructor(renderer) {
        this.renderer = renderer;
        /** @type {Map<string, THREE.Mesh>} id → mesh */
        this._meshes = new Map();
        /** @type {Map<string, THREE.Group>} id → 组合体 Group */
        this._compounds = new Map();
        this._nextId = 1;
    }

    /**
     * 根据规格创建图元并添加到场景
     * @param {object} spec 图元规格
     * @param {string} spec.type 图元类型（cube/sphere/cylinder/...）
     * @param {object} spec.params 图元参数（{w,h,d,r,...}）
     * @param {THREE.Vector3} [spec.position] 位置
     * @param {THREE.Euler} [spec.rotation] 旋转
     * @param {THREE.Vector3} [spec.scale] 缩放
     * @param {number} [spec.color] 材质颜色
     * @param {object} [spec.physics] 物理属性（anchored/autoRotate/...）
     * @returns {THREE.Mesh|null} 创建的网格
     */
    addFromSpec(spec) {
        if (!spec || !spec.type) return null;
        const def = PRIMITIVES[spec.type];
        if (!def) {
            console.warn(`[Primitives] 未知图元类型: ${spec.type}`);
            return null;
        }

        const params = spec.params || {};
        const geometry = def.geo(params);
        const material = new THREE.MeshStandardMaterial({
            color: spec.color ?? Primitives._randomColor(),
            roughness: 0.6,
            metalness: 0.1,
        });
        const mesh = new THREE.Mesh(geometry, material);

        // 位置/旋转/缩放
        if (spec.position) mesh.position.copy(spec.position);
        if (spec.rotation) {
            mesh.rotation.set(spec.rotation.x, spec.rotation.y, spec.rotation.z, spec.rotation.order || 'XYZ');
        }
        if (spec.scale) mesh.scale.copy(spec.scale);

        // 元数据
        const id = `prim-${this._nextId++}`;
        mesh.userData.id = id;
        mesh.userData.type = spec.type;
        mesh.userData.primitiveParams = params;
        if (spec.physics) mesh.userData.physics = spec.physics;

        // 添加到场景
        this.renderer.primitivesRoot.add(mesh);
        this._meshes.set(id, mesh);

        return mesh;
    }

    /**
     * 根据 ID 移除图元
     * @param {string} id
     * @returns {boolean} 是否成功移除
     */
    removeById(id) {
        const mesh = this._meshes.get(id);
        if (!mesh) return false;

        this.renderer.primitivesRoot.remove(mesh);
        if (mesh.geometry) mesh.geometry.dispose();
        if (mesh.material) mesh.material.dispose();
        this._meshes.delete(id);
        return true;
    }

    /**
     * 获取所有图元网格
     * @returns {THREE.Mesh[]}
     */
    getAll() {
        return Array.from(this._meshes.values());
    }

    /**
     * 根据 ID 获取图元
     * @param {string} id
     * @returns {THREE.Mesh|undefined}
     */
    getById(id) {
        return this._meshes.get(id);
    }

    /**
     * 清空所有图元
     */
    clear() {
        for (const mesh of this._meshes.values()) {
            this.renderer.primitivesRoot.remove(mesh);
            if (mesh.geometry) mesh.geometry.dispose();
            if (mesh.material) mesh.material.dispose();
        }
        this._meshes.clear();
        this._compounds.clear();
    }

    // ============ 组合体管理 ============

    /**
     * 创建组合体：将多个图元 reparent 到一个新的 THREE.Group 下
     * - 保留各子图元的相对世界位置（重新计算 group.position 与子图元 local position）
     * - group.userData.type = 'group'，由 physics-manager 作为 Compound 刚体处理
     * - 物理体由调用方（engine.js）通过 physicsManager 添加
     * @param {string[]} meshIds 子图元 ID 列表
     * @param {object} [options] 可选参数
     * @param {string} [options.name] 组合体名（仅作为 userData.name）
     * @param {boolean} [options.anchored] 是否锚定（静态）
     * @returns {THREE.Group|null} 创建的组合体（已加入场景），失败返回 null
     */
    createCompound(meshIds, options = {}) {
        if (!Array.isArray(meshIds) || meshIds.length === 0) return null;
        const meshes = meshIds.map(id => this._meshes.get(id)).filter(Boolean);
        if (meshes.length === 0) return null;

        // 计算组合体中心：所有子图元位置的算术平均
        const center = new THREE.Vector3();
        for (const m of meshes) center.add(m.position);
        center.divideScalar(meshes.length);

        // 创建组合体 Group
        const group = new THREE.Group();
        group.position.copy(center);
        group.userData.id = `compound-${this._nextId++}`;
        group.userData.type = 'group';
        group.userData.name = options.name || `组合体 ${this._compounds.size + 1}`;
        group.userData.compoundMemberIds = meshes.map(m => m.userData.id);
        group.userData.physics = { anchored: !!options.anchored };

        // 把子图元从 primitivesRoot 转移到 group 下，并调整 local position
        for (const m of meshes) {
            this.renderer.primitivesRoot.remove(m);
            // local position = world position - group position
            m.position.sub(center);
            // 旋转/缩放保留（local 与 world 一致，因 group 无旋转）
            group.add(m);
        }

        this.renderer.primitivesRoot.add(group);
        this._meshes.set(group.userData.id, group);
        this._compounds.set(group.userData.id, group);

        return group;
    }

    /**
     * 解散组合体：将子图元 reparent 回 primitivesRoot，并移除组合体 Group
     * 物理体由调用方（engine.js）通过 physicsManager 移除
     * @param {string} compoundId 组合体 ID
     * @returns {string[]} 释放出的子图元 ID 列表
     */
    dissolveCompound(compoundId) {
        const group = this._compounds.get(compoundId);
        if (!group) return [];

        const childIds = [];
        const worldPos = new THREE.Vector3();
        const worldQuat = new THREE.Quaternion();
        const worldScale = new THREE.Vector3();

        for (const child of [...group.children]) {
            // 提取世界变换
            child.updateMatrixWorld(true);
            child.matrixWorld.decompose(worldPos, worldQuat, worldScale);
            group.remove(child);
            child.position.copy(worldPos);
            child.quaternion.copy(worldQuat);
            child.scale.copy(worldScale);
            this.renderer.primitivesRoot.add(child);
            childIds.push(child.userData.id);
        }

        this.renderer.primitivesRoot.remove(group);
        this._meshes.delete(compoundId);
        this._compounds.delete(compoundId);
        return childIds;
    }

    /**
     * 获取所有组合体
     * @returns {Array<{id:string, name:string, memberIds:string[], anchored:boolean}>}
     */
    getCompounds() {
        const result = [];
        for (const [id, group] of this._compounds) {
            result.push({
                id,
                name: group.userData.name || '组合体',
                memberIds: [...(group.userData.compoundMemberIds || [])],
                anchored: !!group.userData.physics?.anchored,
            });
        }
        return result;
    }

    /**
     * 判断 ID 是否为组合体
     * @param {string} id
     * @returns {boolean}
     */
    isCompound(id) {
        return this._compounds.has(id);
    }

    /**
     * 生成随机颜色（用于无纹理图元）
     * @returns {number} hex color
     */
    static _randomColor() {
        const hue = Math.random() * 360;
        const s = 0.5 + Math.random() * 0.3;
        const l = 0.4 + Math.random() * 0.25;
        // HSL to hex
        const c = (1 - Math.abs(2 * l - 1)) * s;
        const x = c * (1 - Math.abs((hue / 60) % 2 - 1));
        const m = l - c / 2;
        let r, g, b;
        if (hue < 60) { r = c; g = x; b = 0; }
        else if (hue < 120) { r = x; g = c; b = 0; }
        else if (hue < 180) { r = 0; g = c; b = x; }
        else if (hue < 240) { r = 0; g = x; b = c; }
        else if (hue < 300) { r = x; g = 0; b = c; }
        else { r = c; g = 0; b = x; }
        const toHex = (v) => Math.round((v + m) * 255);
        return (toHex(r) << 16) | (toHex(g) << 8) | toHex(b);
    }

    /**
     * 为图元应用纹理
     * @param {string} id 图元或组合体 ID
     * @param {string} dataUrl 纹理数据 URL（data:image/...）
     * @param {object} [options] 选项 {repeat:{x,y}, repeatPerFace:boolean}
     * @returns {boolean} 是否成功
     */
    applyTexture(id, dataUrl, options = {}) {
        const target = this._meshes.get(id);
        if (!target) return false;

        const loader = new THREE.TextureLoader();
        loader.load(dataUrl, (texture) => {
            const repeat = options.repeat || { x: 1, y: 1 };
            texture.wrapS = THREE.RepeatWrapping;
            texture.wrapT = THREE.RepeatWrapping;
            texture.repeat.set(repeat.x, repeat.y);

            const applyToMesh = (mesh) => {
                if (!mesh.material) return;
                // 完全替换材质：纯白底色 + 纹理贴图，确保不叠加颜色
                const oldMat = mesh.material;
                const newMat = new THREE.MeshStandardMaterial({
                    map: texture,
                    color: 0xffffff,
                    roughness: 0.8,
                    metalness: 0.0,
                    transparent: false,
                    opacity: 1.0,
                });
                if (oldMat.map && oldMat.map !== texture) oldMat.map.dispose();
                oldMat.dispose();
                mesh.material = newMat;
            };

            if (target.isMesh) {
                applyToMesh(target);
            } else if (target.isGroup) {
                target.traverse(child => { if (child.isMesh) applyToMesh(child); });
            }
        });
        return true;
    }

    /**
     * 清除图元纹理（恢复为纯色材质）
     * @param {string} id 图元或组合体 ID
     * @returns {boolean}
     */
    clearTexture(id) {
        const target = this._meshes.get(id);
        if (!target) return false;
        const clearOnMesh = (mesh) => {
            if (!mesh.material) return;
            if (mesh.material.map) {
                mesh.material.map.dispose();
                mesh.material.map = null;
            }
            // 清除纹理后恢复为随机颜色纯色材质
            mesh.material.color.setHex(Primitives._randomColor());
            mesh.material.transparent = mesh.userData._customTransparent || false;
            mesh.material.opacity = mesh.userData._customOpacity ?? 1.0;
            mesh.material.needsUpdate = true;
        };
        if (target.isMesh) clearOnMesh(target);
        else if (target.isGroup) target.traverse(c => { if (c.isMesh) clearOnMesh(c); });
        return true;
    }

    /**
     * 导出指定资产为 JSON
     * @param {string} id 图元/组合体 ID
     * @returns {object|null}
     */
    exportAsset(id) {
        const target = this._meshes.get(id);
        if (!target) return null;

        const serializeMesh = (mesh) => ({
            type: mesh.userData.type,
            params: { ...(mesh.userData.primitiveParams || {}) },
            position: { x: mesh.position.x, y: mesh.position.y, z: mesh.position.z },
            rotation: { x: mesh.rotation.x, y: mesh.rotation.y, z: mesh.rotation.z, order: mesh.rotation.order },
            scale: { x: mesh.scale.x, y: mesh.scale.y, z: mesh.scale.z },
            color: mesh.material?.color?.getHex?.() ?? 0x9d6bff,
            hasTexture: !!mesh.material?.map,
            textureUUID: mesh.userData.textureUUID || null,
            physics: { ...(mesh.userData.physics || {}) },
        });

        if (target.isMesh) {
            return {
                format_version: '1.0.0',
                asset_type: 'primitive',
                data: serializeMesh(target),
            };
        }
        if (target.isGroup && target.userData.type === 'group') {
            const children = target.children.filter(c => c.isMesh).map(serializeMesh);
            return {
                format_version: '1.0.0',
                asset_type: 'compound',
                name: target.userData.name || '组合体',
                anchored: !!target.userData.physics?.anchored,
                data: children,
            };
        }
        return null;
    }

    /**
     * 从 JSON 导入资产
     * @param {object} asset 资产对象（由 exportAsset 生成）
     * @returns {THREE.Object3D|null} 创建的对象
     */
    importAsset(asset, imageAssetStore = null) {
        if (!asset || !asset.format_version || !asset.asset_type) return null;

        if (asset.asset_type === 'primitive') {
            const d = asset.data;
            const mesh = this.addFromSpec({
                type: d.type,
                params: d.params,
                position: d.position,
                rotation: d.rotation,
                scale: d.scale,
                color: d.color,
                physics: d.physics,
            });
            if (mesh && d.hasTexture) {
                let dataUrl = d.textureDataUrl || null;
                if (!dataUrl && d.textureUUID && imageAssetStore) {
                    const imgAsset = imageAssetStore.get(d.textureUUID);
                    if (imgAsset) dataUrl = imgAsset.base64;
                }
                if (dataUrl) {
                    this.applyTexture(mesh.userData.id, dataUrl);
                    if (d.textureUUID) mesh.userData.textureUUID = d.textureUUID;
                }
            }
            return mesh;
        }
        if (asset.asset_type === 'compound') {
            const meshes = [];
            for (const d of (asset.data || [])) {
                const mesh = this.addFromSpec({
                    type: d.type,
                    params: d.params,
                    position: d.position,
                    rotation: d.rotation,
                    scale: d.scale,
                    color: d.color,
                    physics: d.physics,
                });
                if (mesh) {
                    if (d.hasTexture) {
                        let dataUrl = d.textureDataUrl || null;
                        if (!dataUrl && d.textureUUID && imageAssetStore) {
                            const imgAsset = imageAssetStore.get(d.textureUUID);
                            if (imgAsset) dataUrl = imgAsset.base64;
                        }
                        if (dataUrl) {
                            this.applyTexture(mesh.userData.id, dataUrl);
                            if (d.textureUUID) mesh.userData.textureUUID = d.textureUUID;
                        }
                    }
                    meshes.push(mesh);
                }
            }
            if (meshes.length === 0) return null;
            const group = this.createCompound(meshes.map(m => m.userData.id), {
                name: asset.name,
                anchored: asset.anchored,
            });
            return group;
        }
        return null;
    }
}

export { PRIMITIVES, Primitives };
