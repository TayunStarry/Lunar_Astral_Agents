// ==== geometry-loader.js — Bedrock 几何加载器（index_geometry.json） ====

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
            console.warn(`[lunar.studio.bedrock] .bbmodel format_version ${json.meta.format_version} 高于支持的 5.0，可能存在兼容性问题`);
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

    /**
     * 解析 Bedrock 运行时几何格式 `index_geometry.json`
     *
     * 月岩引擎模型引用格式翻新：不再使用 `index.bbmodel`（Blockbench 工程），
     * 改用 `index_geometry.json`（几何）+ `index_texture.png`（纹理），
     * 并约束模型 `format_version` 不得低于 1.10。
     *
     * 该格式结构：`{ format_version, "minecraft:geometry": [ { description, bones } ] }`
     *   - bones[].pivot → Bone.origin（骨骼局部原点）
     *   - bones[].rotation → Bone.rotation（度）
     *   - bones[].cubes[].origin/size → CubeElement.from/to（像素，从下前右角起）
     *   - cubes[].uv 为对象形式（{north:{uv,uv_size},...}）→ per-face UV
     *
     * @param {object|string} data index_geometry.json 内容（对象或 JSON 字符串）
     * @returns {{meta: object, outliner: Outliner, textures: Array, metadata: object, format_version: string}}
     */
    static parseBedrock(data) {
        const json = typeof data === 'string' ? JSON.parse(data) : data;

        // ---- 模型版本约束：format_version 不得低于 1.10 ----
        const rawFmt = json.format_version;
        const fmtNum = typeof rawFmt === 'number' ? rawFmt : parseFloat(String(rawFmt));
        if (!Number.isFinite(fmtNum) || fmtNum < 1.1) {
            console.warn(`[lunar.rendering_engine] 模型 format_version ${rawFmt} 低于 1.10，月岩引擎要求不低于 1.10`);
            throw new Error(`模型版本过低：format_version ${rawFmt} < 1.10（月岩引擎要求 ≥1.10）`);
        }

        // ---- 定位几何条目（minecraft:geometry[0]） ----
        let geo = null;
        const geometryList = json['minecraft:geometry'];
        if (Array.isArray(geometryList) && geometryList.length > 0) {
            geo = geometryList[0];
        } else if (Array.isArray(json.geometry) && json.geometry.length > 0) {
            geo = json.geometry[0];
        }
        if (!geo) throw new Error('index_geometry.json 中未找到 minecraft:geometry 条目');

        const desc = geo.description || {};
        const texWidth = desc.texture_width || 16;
        const texHeight = desc.texture_height || 16;

        const metadata = {
            name: desc.identifier || '未命名模型',
            modelIdentifier: desc.identifier || '',
        };
        const meta = {
            formatVersion: String(rawFmt),
            modelFormat: 'bedrock_geometry',
            texWidth,
            texHeight,
            boxUv: false,
        };

        // ---- 先为所有骨骼创建 Bone 节点（用于解析 parent 引用） ----
        const boneNodes = (geo.bones || []).map(boneData => ({
            raw: boneData,
            bone: new Bone({
                name: boneData.name || '未命名骨骼',
                // 坐标翻新：index_geometry.json 相对 .bbmodel 在 X 轴镜像、旋转 rx/ry 取反
                origin: GeometryLoader._bedrockPos(boneData.pivot),
                rotation: GeometryLoader._bedrockRot(boneData.rotation),
            }),
        }));
        const boneByName = new Map();
        for (const { raw, bone } of boneNodes) boneByName.set(raw.name, bone);

        const outliner = new Outliner();

        // ---- 挂载 cubes 到各骨骼 ----
        for (const { raw, bone } of boneNodes) {
            for (const cubeRaw of raw.cubes || []) {
                const cube = GeometryLoader._bedrockCubeToElement(cubeRaw, raw.inflate);
                if (cube) {
                    bone.addCube(cube);
                    outliner.index.set(cube.uuid, cube);
                }
            }
            outliner.index.set(bone.uuid, bone);
        }

        // ---- 按 parent 字段链接骨骼树，无 parent 或 parent 缺失者为根 ----
        for (const { raw, bone } of boneNodes) {
            if (raw.parent && boneByName.has(raw.parent)) {
                boneByName.get(raw.parent).addChildBone(bone);
            } else {
                outliner.roots.push(bone);
            }
        }

        return { meta, outliner, textures: [], metadata, format_version: rawFmt };
    }

    /**
     * 将 Bedrock 运行时 cube 定义转为 CubeElement（含 per-face UV）
     * @param {object} cubeRaw cube 原始数据 {origin, size, rotation, uv}
     * @returns {CubeElement|null}
     * @private
     */
    static _bedrockCubeToElement(cubeRaw, boneInflate) {
        // 严格参照 blockbench js/formats/bedrock/bedrock.js parseCube：
        //   1) from/to 仅 X 镜像（Z/Y 不变）
        //   2) 旋转 pivot origin = cube.pivot（缺省 [0,0,0]），X 镜像
        //   3) rotation 的 rx、ry 取反，rz 不变（_bedrockRot）
        //   4) inflate = cube.inflate ?? bone.inflate（Blockbench s.inflate ?? bone_data.inflate）
        const rawOrigin = cubeRaw.origin || [0, 0, 0];
        const size = cubeRaw.size ? [...cubeRaw.size] : [0, 0, 0];
        const from = [-(rawOrigin[0] + size[0]), rawOrigin[1] || 0, rawOrigin[2] || 0];
        const to = [-rawOrigin[0], (rawOrigin[1] || 0) + size[1], (rawOrigin[2] || 0) + size[2]];
        const pivot = Array.isArray(cubeRaw.pivot) ? cubeRaw.pivot : [0, 0, 0];
        const inflate = (typeof cubeRaw.inflate === 'number')
            ? cubeRaw.inflate
            : (typeof boneInflate === 'number' ? boneInflate : 0);
        const cube = new CubeElement({
            from,
            to,
            origin: [-(pivot[0] || 0), pivot[1] || 0, pivot[2] || 0], // 旋转 pivot（X 镜像）
            rotation: GeometryLoader._bedrockRot(cubeRaw.rotation),
            inflate,
            box_uv: false,
        });

        // per-face 对象形式：{ north: {uv:[u,v], uv_size:[w,h]}, ... }
        if (cubeRaw.uv && typeof cubeRaw.uv === 'object' && !Array.isArray(cubeRaw.uv)) {
            for (const [faceKey, face] of Object.entries(cubeRaw.uv)) {
                if (!face || !face.uv) continue;
                const u = face.uv[0] || 0;
                const v = face.uv[1] || 0;
                const w = (face.uv_size && face.uv_size[0]) || 0;
                const h = (face.uv_size && face.uv_size[1]) || 0;
                // 与 renderer._buildCubeMesh 约定一致：[u1, v1, u2, v2]（像素，flipY=false）
                cube.faces[faceKey] = { texture: 0, uv: [u, v, u + w, v + h] };
            }
        } else if (Array.isArray(cubeRaw.uv)) {
            // 数组形式（box UV 解包）需已知纹理模板，暂不支持：给出明确提示
            console.warn(`[lunar.rendering_engine] cube '${cubeRaw.origin}' 使用数组形式 uv（box UV），请导出为 per-face UV 以正确渲染`);
        }

        return cube;
    }

    /**
     * Bedrock 位置坐标 → 引擎空间（X 轴镜像）
     * index_geometry.json 相对 .bbmodel 在 X 轴镜像（同一立方体：bbmodel from.x=+15…17 ↔ json origin.x=-17）
     * @param {[number, number, number]|undefined} v [x, y, z]
     * @returns {[number, number, number]}
     * @private
     */
    static _bedrockPos(v) {
        if (!v) return [0, 0, 0];
        return [-v[0], v[1] || 0, v[2] || 0];
    }

    /**
     * Bedrock 旋转角 → 引擎空间（rx、ry 取反，rz 保持不变）
     * 与 .bbmodel 旋转符号一致：如 ribbonLeft 在 bbmodel 为 -0.10,-27.26,-10.50，json 为 0.10,27.26,-10.50
     * @param {[number, number, number]|undefined} v [x, y, z] 度
     * @returns {[number, number, number]}
     * @private
     */
    static _bedrockRot(v) {
        if (!v) return [0, 0, 0];
        return [-(v[0] || 0), -(v[1] || 0), v[2] || 0];
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
