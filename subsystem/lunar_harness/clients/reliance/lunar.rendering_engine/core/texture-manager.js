// ==== texture-manager.js — 纹理管理器 ====

import * as THREE from '../vendor/three.module.js';

/**
 * TextureManager — 加载 .bbmodel 中的纹理并构建 THREE.Texture
 * 参考 Blockbench js/texturing/textures.js 的 Texture 类，去 Vue 与 canvas 直写
 */
export class TextureManager {
    constructor() {
        /** @type {Map<string, THREE.Texture>} id → texture */
        this.textures = new Map();
        /** @type {Map<string, object>} id → 原始纹理元数据 */
        this.metadata = new Map();
    }

    /**
     * 从 .bbmodel 的 textures 数组加载
     * @param {Array} texturesData .bbmodel.textures
     * @returns {Promise<void>}
     */
    async loadFromBBModel(texturesData) {
        this.clear();

        for (const texData of texturesData) {
            const texture = await TextureManager.loadTexture(texData);
            if (texture) {
                const id = texData.id || '0';
                this.textures.set(id, texture);
                this.metadata.set(id, texData);
            }
        }
    }

    /**
     * 加载单个纹理（支持 base64 data URL 与相对路径）
     * @param {object} texData
     * @returns {Promise<THREE.Texture|null>}
     */
    static async loadTexture(texData) {
        try {
            const source = texData.source;
            if (!source) {
                console.warn(`[TextureManager] 纹理 ${texData.name} 无 source`);
                return null;
            }

            // base64 data URL
            if (source.startsWith('data:')) {
                return await TextureManager.loadFromDataURL(source);
            }

            // 相对路径（暂不支持，需要项目文件系统访问）
            console.warn(`[TextureManager] 纹理 ${texData.name} 使用相对路径 ${texData.relative_path}，暂不支持加载`);
            return null;
        } catch (err) {
            console.error(`[TextureManager] 加载纹理 ${texData.name} 失败:`, err);
            return null;
        }
    }

    /**
     * 从 base64 data URL 加载 THREE.Texture
     * @param {string} dataUrl
     * @returns {Promise<THREE.Texture>}
     */
    static loadFromDataURL(dataUrl) {
        return new Promise((resolve, reject) => {
            const loader = new THREE.TextureLoader();
            loader.load(
                dataUrl,
                (texture) => {
                    // Bedrock 纹理使用最近邻过滤（像素风）
                    texture.magFilter = THREE.NearestFilter;
                    texture.minFilter = THREE.NearestMipmapNearestFilter;
                    texture.generateMipmaps = true;
                    texture.colorSpace = THREE.SRGBColorSpace;
                    texture.flipY = false;  // Bedrock UV 不翻转 Y
                    texture.needsUpdate = true;
                    resolve(texture);
                },
                undefined,
                (err) => reject(err)
            );
        });
    }

    /**
     * 从外部纹理图片文件加载为 id='0' 的默认纹理（月岩引擎 index_texture.png 使用）
     * @param {string} url 纹理文件 URL（如 /file/read/models/bedrock/index_texture.png）
     * @param {{uvWidth?: number, uvHeight?: number, name?: string}} [options] 纹理尺寸与名称
     * @returns {Promise<THREE.Texture|null>}
     */
    async loadFromFile(url, options = {}) {
        this.clear();
        try {
            const texture = await TextureManager.loadFromURL(url);
            const uvWidth = options.uvWidth || texture.image?.width || 16;
            const uvHeight = options.uvHeight || texture.image?.height || 16;
            this.textures.set('0', texture);
            this.metadata.set('0', {
                id: '0',
                name: options.name || 'index_texture',
                uv_width: uvWidth,
                uv_height: uvHeight,
            });
            return texture;
        } catch (err) {
            console.error(`[TextureManager] 纹理文件加载失败: ${url}`, err);
            return null;
        }
    }

    /**
     * 从 URL 加载 THREE.Texture（像素风过滤，flipY=false，sRGB）
     * @param {string} url 图片 URL
     * @returns {Promise<THREE.Texture>}
     * @private
     */
    static loadFromURL(url) {
        return new Promise((resolve, reject) => {
            const loader = new THREE.TextureLoader();
            loader.load(
                url,
                (texture) => {
                    texture.magFilter = THREE.NearestFilter;
                    texture.minFilter = THREE.NearestMipmapNearestFilter;
                    texture.generateMipmaps = true;
                    texture.colorSpace = THREE.SRGBColorSpace;
                    texture.flipY = false;  // Bedrock UV 不翻转 Y
                    texture.needsUpdate = true;
                    resolve(texture);
                },
                undefined,
                (err) => reject(err)
            );
        });
    }

    /**
     * 获取默认纹理（id='0' 或第一个）
     * @returns {THREE.Texture|null}
     */
    getDefault() {
        if (this.textures.has('0')) return this.textures.get('0');
        const first = this.textures.values().next();
        return first.done ? null : first.value;
    }

    /**
     * 按 id 获取纹理
     * @param {string|number} id
     * @returns {THREE.Texture|null}
     */
    get(id) {
        return this.textures.get(String(id)) || null;
    }

    /**
     * 清空所有纹理
     */
    clear() {
        for (const texture of this.textures.values()) {
            texture.dispose();
        }
        this.textures.clear();
        this.metadata.clear();
    }
}
