import * as THREE from './three.module.js';
import { PRIMITIVES } from './primitives.js';

// ============ 资产管理器 ============
class AssetManager {
    static exportScene(sceneManager, physicsManager = null) {
        const data = {
            version: '2.0', name: '渲染方案', createdAt: new Date().toISOString(),
            skybox: {
                top: '#' + sceneManager.skySphere.material.uniforms.topColor.value.getHexString(),
                mid: '#' + sceneManager.skySphere.material.uniforms.midColor.value.getHexString(),
                bottom: '#' + sceneManager.skySphere.material.uniforms.bottomColor.value.getHexString(),
            },
            lighting: {
                ambient: sceneManager.ambientLight.intensity,
                hemi: sceneManager.hemiLight.intensity,
                sun: sceneManager.sunLight.intensity,
                sunDir: { x: sceneManager.sunLight.position.x, y: sceneManager.sunLight.position.y, z: sceneManager.sunLight.position.z },
                shadows: sceneManager.sunLight.castShadow,
            },
            ground: {
                gridVisible: sceneManager.gridHelper.visible,
                groundVisible: sceneManager.groundPlane.visible,
                size: sceneManager.gridHelper.geometry.parameters?.size || 20,
                color: '#' + sceneManager.gridHelper.material.color.getHexString(),
            },
            physics: physicsManager ? {
                gravity: physicsManager.gravity,
                groundY: physicsManager.groundY,
                massSingle: physicsManager.massSingle,
                linearDamping: physicsManager.linearDamping,
                angularDamping: physicsManager.angularDamping,
            } : null,
            textures: [...sceneManager.texturePool.values()].map(e => ({ name: e.name, base64: e.base64 })),
            objects: sceneManager.objects.map(obj => AssetManager._serialize(obj)),
            keyframes: sceneManager.keyframes,
        };
        const json = JSON.stringify(data, null, 2);
        const blob = new Blob([json], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a'); a.href = url; a.download = `scene_${Date.now()}.json`; a.click();
        URL.revokeObjectURL(url);
    }

    static async importScene(file) {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = e => {
                try {
                    const data = JSON.parse(e.target.result);
                    if (!data.version || !data.objects) { reject(new Error('无效的方案文件格式')); return; }
                    resolve(data);
                } catch (err) { reject(new Error('JSON 解析失败: ' + err.message)); }
            };
            reader.onerror = () => reject(new Error('文件读取失败'));
            reader.readAsText(file);
        });
    }

    static exportModel(mesh) {
        const data = AssetManager._serialize(mesh);
        const json = JSON.stringify(data, null, 2);
        const blob = new Blob([json], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a'); a.href = url; a.download = `${mesh.userData.name || 'model'}.json`; a.click();
        URL.revokeObjectURL(url);
    }

    static exportGroupAsset(group) {
        const data = {
            version: '2.0', type: 'group-asset',
            name: group.userData.name || '组合体',
            createdAt: new Date().toISOString(),
            position: { x: group.position.x, y: group.position.y, z: group.position.z },
            rotation: { x: group.rotation.x, y: group.rotation.y, z: group.rotation.z },
            scale: { x: group.scale.x, y: group.scale.y, z: group.scale.z },
            children: group.children.map(child => AssetManager._serialize(child)),
        };
        const json = JSON.stringify(data, null, 2);
        const blob = new Blob([json], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a'); a.href = url; a.download = `${data.name}.json`; a.click();
        URL.revokeObjectURL(url);
    }

    static async importGroupAsset(file) {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = e => {
                try {
                    const data = JSON.parse(e.target.result);
                    if (!data.version || data.type !== 'group-asset' || !data.children) {
                        reject(new Error('无效的组合体资产文件'));
                        return;
                    }
                    resolve(data);
                } catch (err) { reject(new Error('JSON 解析失败: ' + err.message)); }
            };
            reader.onerror = () => reject(new Error('文件读取失败'));
            reader.readAsText(file);
        });
    }

    static exportMaterial(mesh) {
        if (!mesh || !mesh.material) return;
        const mat = Array.isArray(mesh.material) ? mesh.material[0] : mesh.material;
        const data = {
            version: '1.0', type: 'material', name: `${mesh.userData.name || 'material'}_材质`,
            color: '#' + mat.color.getHexString(), roughness: mat.roughness, metalness: mat.metalness,
            opacity: mat.opacity, transparent: mat.transparent,
            side: mat.side ?? 0, flatShading: mat.flatShading ?? false, wireframe: mat.wireframe ?? false,
            materialType: mat.isMeshStandardMaterial ? 'standard' : 'basic',
        };
        if (mat.map && mat.map.image) {
            const canvas = document.createElement('canvas');
            canvas.width = mat.map.image.width; canvas.height = mat.map.image.height;
            canvas.getContext('2d').drawImage(mat.map.image, 0, 0);
            data.texture = canvas.toDataURL('image/png');
        }
        const json = JSON.stringify(data, null, 2);
        const blob = new Blob([json], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a'); a.href = url; a.download = `${data.name}.json`; a.click();
        URL.revokeObjectURL(url);
    }

    static _serialize(mesh) {
        const data = {
            id: mesh.userData.id, name: mesh.userData.name || '未命名',
            type: mesh.userData.type || 'unknown', primitiveType: mesh.userData.primitiveType || null,
            position: { x: mesh.position.x, y: mesh.position.y, z: mesh.position.z },
            rotation: { x: mesh.rotation.x, y: mesh.rotation.y, z: mesh.rotation.z },
            scale: { x: mesh.scale.x, y: mesh.scale.y, z: mesh.scale.z },
        };

        // 组合体：递归序列化子对象
        if (mesh.userData.type === 'group') {
            data.children = mesh.children.map(child => AssetManager._serialize(child));
            return data;
        }

        const mat = Array.isArray(mesh.material) ? mesh.material[0] : mesh.material;
        if (mat) {
            data.material = {
                color: '#' + mat.color.getHexString(),
                roughness: mat.roughness, metalness: mat.metalness,
                opacity: mat.opacity, transparent: mat.transparent,
                side: mat.side ?? 0,
                flatShading: mat.flatShading ?? false,
                wireframe: mat.wireframe ?? false,
                type: mat.isMeshStandardMaterial ? 'standard' : 'basic',
            };
            if (mesh.userData.textureName) data.material.textureName = mesh.userData.textureName;
        }

        // 多面材质：保存每个面的材质
        if (Array.isArray(mesh.material) && mesh.userData.faceNames) {
            const faceMaterials = [];
            for (let i = 0; i < mesh.material.length; i++) {
                const fm = mesh.material[i];
                faceMaterials.push({
                    color: '#' + fm.color.getHexString(),
                    roughness: fm.roughness, metalness: fm.metalness,
                    opacity: fm.opacity, transparent: fm.transparent,
                    side: fm.side ?? 0,
                    flatShading: fm.flatShading ?? false,
                    wireframe: fm.wireframe ?? false,
                    textureName: fm.map === mat.map ? mesh.userData.textureName : null,
                });
            }
            data.faceMaterials = faceMaterials;
            data.faceNames = mesh.userData.faceNames;
            data.faceVisible = mesh.userData.faceVisible;
        }

        if (mesh.geometry && mesh.userData.primitiveType) {
            const pp = mesh.userData.primitiveParams || {};
            const params = mesh.geometry.parameters || {};
            const gd = { type: mesh.userData.primitiveType };
            const def = PRIMITIVES[mesh.userData.primitiveType];
            if (def && def.params) {
                for (const p of def.params) {
                    gd[p.key] = pp[p.key] !== undefined ? pp[p.key] : (params[p.key] !== undefined ? params[p.key] : p.default);
                }
            }
            data.geometry = gd;
        }
        return data;
    }

    static deserializeGeometry(data) {
        const type = data.type;
        const def = PRIMITIVES[type];
        if (def) {
            const p = {};
            if (def.params) {
                for (const param of def.params) {
                    p[param.key] = data[param.key] !== undefined ? data[param.key] : param.default;
                }
            }
            const geo = def.geo(p);
            if (type === 'cone' && geo.groups) {
                for (const group of geo.groups) {
                    if (group.materialIndex === 2) group.materialIndex = 1;
                }
            }
            return geo;
        }
        return new THREE.BoxGeometry(1, 1, 1);
    }
}

export { AssetManager };