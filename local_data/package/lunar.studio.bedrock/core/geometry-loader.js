// ==== geometry-loader.js — .bbmodel 几何加载器 ====

import { Outliner, Bone, CubeElement } from './outliner.js';

/**
 * GeometryLoader — 解析 .bbmodel JSON
 * 参考 Blockbench js/formats/bbmodel.js + js/formats/bedrock/bedrock.js:parseGeometry
 * 不复制代码，仅参考算法（轴翻转、版本迁移）
 */
export class GeometryLoader {
    /**
     * 解析 .bbmodel JSON
     * @param {object|string} data .bbmodel 内容（对象或 JSON 字符串）
     * @returns {{meta: object, outliner: Outliner, textures: Array, metadata: object}}
     */
    static parse(data) {
        const json = typeof data === 'string' ? JSON.parse(data) : data;

        // 版本兼容性检查（参考 bbmodel.js:processHeader）
        if (json.meta?.format_version && parseFloat(json.meta.format_version) > 5.0) {
            console.warn(`[bedrock_render_engine] .bbmodel format_version ${json.meta.format_version} 高于支持的 5.0，可能存在兼容性问题`);
        }

        // 元信息提取
        const meta = {
            formatVersion: json.meta?.format_version || '5.0',
            modelFormat: json.meta?.model_format || 'bedrock',
            boxUv: json.meta?.box_uv || false,
            resolution: json.resolution || { width: 16, height: 16 }
        };

        // 构建 Outliner 骨骼树
        // .bbmodel format_version 5.0：骨骼元数据（name/origin/rotation）存储在顶层 groups 数组，
        // outliner 仅存树结构（uuid 引用）。需同时传入 groups 供查找。
        const outliner = Outliner.fromBBModel(json.outliner || [], json.elements || [], json.groups || []);

        // 纹理列表（保留原始结构，由 TextureManager 处理）
        const textures = json.textures || [];

        // 模型元数据
        const metadata = {
            name: json.name || '未命名模型',
            modelIdentifier: json.model_identifier || '',
            visibleBox: json.visible_box || [0, 0, 0],
            bedrockAnimationMode: json.bedrock_animation_mode || 'entity'
        };

        return { meta, outliner, textures, metadata };
    }

    /**
     * 从 File 对象加载 .bbmodel
     * @param {File} file
     * @returns {Promise<object>}
     */
    static async loadFromFile(file) {
        const text = await file.text();
        return GeometryLoader.parse(text);
    }
}

/**
 * 坐标系转换工具
 *
 * 重要：.bbmodel 文件存储的是 Blockbench 内部坐标系（Y-up 右手系，与 Three.js 兼容）。
 * Bedrock 几何格式导入到 Blockbench 时会执行轴翻转（见 bedrock.js parseBone/parseCube），
 * 但 .bbmodel 文件已是翻转后的结果，加载时不需要任何轴翻转。
 */
export class BedrockCoordinate {
    /**
     * 旋转度数 → Three.js 旋转弧度
     * .bbmodel 中的旋转值已是 Blockbench 内部坐标系，直接转弧度即可，不做轴取反
     * @param {[number, number, number]} rotationDegrees [x, y, z] 度
     * @returns {[number, number, number]} [x, y, z] 弧度
     */
    static rotationToThree(rotationDegrees) {
        if (!rotationDegrees) return [0, 0, 0];
        const deg2rad = Math.PI / 180;
        return [
            rotationDegrees[0] * deg2rad,
            rotationDegrees[1] * deg2rad,
            rotationDegrees[2] * deg2rad
        ];
    }

    /**
     * Bedrock 像素坐标 → Three.js 世界坐标
     * Bedrock 1 像素 = 1 单位，Three.js 通常用 1/16 单位
     * 这里使用 1:1 缩放，由相机距离适配
     * @param {[number, number, number]} pixelCoord
     * @param {number} scale 缩放因子，默认 1
     * @returns {THREE.Vector3}
     */
    static pixelToWorld(pixelCoord, scale = 1) {
        // Bedrock 原点在左下前，Three.js 原点在中心
        // 这里只做缩放，不做平移（平移由骨骼 origin 处理）
        return [
            pixelCoord[0] * scale,
            pixelCoord[1] * scale,
            pixelCoord[2] * scale
        ];
    }
}
