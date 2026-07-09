// ==== material-system.js — Bedrock 材质系统 ====

import * as THREE from '../three.module.js';

/**
 * MaterialSystem — 4 种 Bedrock render method 的材质管理
 * 参考 Blockbench js/preview/preview.js + texture.frag
 *
 * render method 说明：
 *   - opaque：完全不透明，alpha=1 的像素渲染，其余丢弃
 *   - alphatest：alpha 测试，alpha < 阈值的像素丢弃（硬边缘）
 *   - translucent：alpha 混合，半透明像素正常混合（软边缘）
 *   - double_sided：双面渲染，不剔除背面
 *
 * 设计：
 *   - 材质按纹理缓存，避免重复创建
 *   - render method 切换时清空缓存并重建
 *   - 支持光照开关（unlit/lit）
 */
export class MaterialSystem {
    constructor() {
        /** @type {string} 当前 render method */
        this.renderMethod = 'alphatest';

        /** @type {THREE.Texture|null} 默认纹理 */
        this.defaultTexture = null;

        /** @type {Map<string, THREE.Material>} 纹理 uuid → 材质 缓存 */
        this._cache = new Map();

        /** @type {boolean} 是否启用光照（false=MeshBasicMaterial, true=MeshLambertMaterial） */
        this.lit = false;

        /** @type {number} alphatest 阈值 */
        this.alphaTestThreshold = 0.5;

        /** @type {number} translucent 全局不透明度倍率（1.0=使用纹理 alpha） */
        this.translucentOpacity = 1.0;
    }

    /**
     * 设置 render method
     * @param {string} method opaque | alphatest | translucent | double_sided
     */
    setRenderMethod(method) {
        if (!['opaque', 'alphatest', 'translucent', 'double_sided'].includes(method)) {
            console.warn(`[MaterialSystem] 未知 render method: ${method}`);
            return;
        }
        this.renderMethod = method;
        this._cache.clear();
    }

    /**
     * 设置默认纹理
     * @param {THREE.Texture|null} texture
     */
    setDefaultTexture(texture) {
        this.defaultTexture = texture;
        this._cache.clear();
    }

    /**
     * 切换光照模式
     * @param {boolean} lit
     */
    setLit(lit) {
        this.lit = lit;
        this._cache.clear();
    }

    /**
     * 设置 alphatest 阈值
     * @param {number} threshold 0-1
     */
    setAlphaTestThreshold(threshold) {
        this.alphaTestThreshold = Math.max(0, Math.min(1, threshold));
        this._cache.clear();
    }

    /**
     * 设置 translucent 全局不透明度
     * @param {number} opacity 0-1
     */
    setTranslucentOpacity(opacity) {
        this.translucentOpacity = Math.max(0, Math.min(1, opacity));
        this._cache.clear();
    }

    /**
     * 创建材质
     * @param {THREE.Texture|null} texture
     * @returns {THREE.Material}
     */
    createMaterial(texture) {
        const tex = texture || this.defaultTexture;
        const cacheKey = this._cacheKey(tex);

        // 命中缓存
        if (this._cache.has(cacheKey)) {
            return this._cache.get(cacheKey);
        }

        const material = this._buildMaterial(tex);
        this._cache.set(cacheKey, material);
        return material;
    }

    /**
     * 构建材质（根据 render method）
     * @param {THREE.Texture|null} texture
     * @returns {THREE.Material}
     * @private
     */
    _buildMaterial(texture) {
        // Bedrock 模型默认双面渲染：cube 旋转后或 from/to 非标准顺序会导致面法线反转，
        // 且 Minecraft 模型需要从内部观察时也能看到面，因此所有 render method 均使用 DoubleSide
        const baseParams = {
            map: texture,
            side: THREE.DoubleSide
        };

        // 选择材质类型
        const MaterialClass = this.lit ? THREE.MeshLambertMaterial : THREE.MeshBasicMaterial;

        switch (this.renderMethod) {
            case 'opaque':
                // 完全不透明：alpha=1 渲染，其余丢弃
                return new MaterialClass({
                    ...baseParams,
                    transparent: false,
                    alphaTest: 0.5,
                    depthWrite: true
                });

            case 'alphatest':
                // alpha 测试：硬边缘，无混合
                return new MaterialClass({
                    ...baseParams,
                    transparent: false,
                    alphaTest: this.alphaTestThreshold,
                    depthWrite: true
                });

            case 'translucent':
                // alpha 混合：软边缘，使用纹理 alpha
                return new MaterialClass({
                    ...baseParams,
                    transparent: true,
                    opacity: this.translucentOpacity,
                    alphaTest: 0,
                    depthWrite: false,
                    blending: THREE.NormalBlending
                });

            case 'double_sided':
                // 双面渲染：不剔除背面（baseParams 已是 DoubleSide）
                return new MaterialClass({
                    ...baseParams,
                    transparent: false,
                    alphaTest: 0.5,
                    depthWrite: true
                });

            default:
                return new MaterialClass(baseParams);
        }
    }

    /**
     * 生成缓存键
     * @param {THREE.Texture|null} texture
     * @returns {string}
     * @private
     */
    _cacheKey(texture) {
        const texId = texture ? (texture.uuid || 'tex') : 'notex';
        return `${this.renderMethod}:${this.lit ? 'lit' : 'unlit'}:${texId}`;
    }

    /**
     * 更新场景图中所有 mesh 的材质
     * @param {THREE.Object3D} root
     */
    updateAllMaterials(root) {
        if (!root) return;
        root.traverse(obj => {
            if (obj.isMesh && obj.material) {
                const oldMat = obj.material;
                const tex = oldMat.map || this.defaultTexture;
                obj.material = this.createMaterial(tex);
                if (oldMat && oldMat !== obj.material) {
                    oldMat.dispose();
                }
            }
        });
    }

    /**
     * 为单个 mesh 设置材质
     * @param {THREE.Mesh} mesh
     * @param {THREE.Texture|null} texture
     */
    setMeshMaterial(mesh, texture) {
        if (!mesh) return;
        const oldMat = mesh.material;
        mesh.material = this.createMaterial(texture);
        if (oldMat && oldMat !== mesh.material) {
            oldMat.dispose();
        }
    }

    /**
     * 释放所有缓存材质
     */
    dispose() {
        for (const material of this._cache.values()) {
            material.dispose();
        }
        this._cache.clear();
    }

    /**
     * 获取当前材质配置描述
     */
    get description() {
        const methodNames = {
            opaque: '不透明',
            alphatest: 'Alpha 测试',
            translucent: '半透明',
            double_sided: '双面'
        };
        return `${methodNames[this.renderMethod] || this.renderMethod} · ${this.lit ? '光照' : '无光照'}`;
    }
}
