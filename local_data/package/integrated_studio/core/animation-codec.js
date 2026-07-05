// ==== animation-codec.js — 动画 JSON 编解码 ====

import { Animation, BoneAnimation, Channel, Keyframe } from './keyframe.js';

/**
 * Bedrock 动画 JSON 解析器
 * 参考 Blockbench js/animations/animation_codec.ts + bedrock_animation.js
 *
 * 输入格式（format_version 1.8.0）：
 * {
 *   "format_version": "1.8.0",
 *   "animations": {
 *     "animation.name": {
 *       "loop": true | false | "hold_on_last_frame",
 *       "animation_length": 1.25,
 *       "override_previous_animation": false,
 *       "bones": {
 *         "boneName": {
 *           "rotation": { "0.0": [...], "0.5": {pre,post,lerp_mode} },
 *           "position": { ... },
 *           "scale": { ... }
 *         }
 *       }
 *     }
 *   }
 * }
 *
 * 关键帧值支持三种形式：
 *   1. 数组 [x, y, z] — 元素为数字或 MoLang 字符串
 *   2. 对象 { pre: [...], post: [...], lerp_mode: "catmullrom" }
 *   3. 单个数字/字符串（罕见，扩展为 [v,v,v]）
 */
export class AnimationCodec {
    /**
     * 解析动画 JSON
     * @param {object|string} json
     * @returns {Map<string, Animation>} animationName → Animation
     */
    static parse(json) {
        if (typeof json === 'string') {
            json = JSON.parse(json);
        }
        const result = new Map();
        if (!json || !json.animations) {
            return result;
        }
        for (const [name, animData] of Object.entries(json.animations)) {
            try {
                const anim = AnimationCodec.parseAnimation(name, animData);
                result.set(name, anim);
            } catch (e) {
                console.warn(`[AnimationCodec] 解析动画失败: ${name}`, e);
            }
        }
        return result;
    }

    /**
     * 解析单个动画
     * @param {string} name
     * @param {object} data
     * @returns {Animation}
     */
    static parseAnimation(name, data) {
        const anim = new Animation(name);
        anim.loop = data.loop !== undefined ? data.loop : false;
        anim.animationLength = data.animation_length || 0;
        anim.overrideRootAnimation = data.override_previous_animation === true;

        if (data.bones) {
            for (const [boneName, boneData] of Object.entries(data.bones)) {
                const boneAnim = new BoneAnimation(boneName);
                if (boneData.rotation) {
                    boneAnim.rotation = AnimationCodec.parseChannel('rotation', boneData.rotation);
                }
                if (boneData.position) {
                    boneAnim.position = AnimationCodec.parseChannel('position', boneData.position);
                }
                if (boneData.scale) {
                    boneAnim.scale = AnimationCodec.parseChannel('scale', boneData.scale);
                }
                anim.bones.set(boneName, boneAnim);
            }
        }

        // 如果未指定 animation_length，用最后一个关键帧的时间
        if (!anim.animationLength) {
            let maxTime = 0;
            for (const boneAnim of anim.bones.values()) {
                if (boneAnim.rotation) maxTime = Math.max(maxTime, boneAnim.rotation.duration);
                if (boneAnim.position) maxTime = Math.max(maxTime, boneAnim.position.duration);
                if (boneAnim.scale) maxTime = Math.max(maxTime, boneAnim.scale.duration);
            }
            anim.animationLength = maxTime;
        }

        return anim;
    }

    /**
     * 解析通道
     * 参考 Blockbench bedrock_animation.js getKeyframeDataPoints：
     * Bedrock 动画格式中 rotation 的 X/Y 轴与 Blockbench 内部坐标系符号相反，
     * 加载时需对 rotation 的 X 和 Y 取反（Z 不变）。position 的 X 也需取反。
     * @param {string} channelName
     * @param {object|Array} channelData
     * @returns {Channel}
     */
    static parseChannel(channelName, channelData) {
        const channel = new Channel(channelName);

        // 形式 1：单数组 [x, y, z] — 整个动画使用同一值（无时间维度）
        if (Array.isArray(channelData)) {
            const values = channelData.map(v => AnimationCodec._normalizeValue(v));
            AnimationCodec._applyBedrockAxisFix(channelName, values);
            channel.addKeyframe(new Keyframe(0, values, values, 'linear'));
            return channel;
        }

        // 形式 2：单个数字/字符串 — 扩展为 [v, v, v]
        if (typeof channelData === 'number' || typeof channelData === 'string') {
            const v = AnimationCodec._normalizeValue(channelData);
            const values = [v, v, v];
            AnimationCodec._applyBedrockAxisFix(channelName, values);
            channel.addKeyframe(new Keyframe(0, values, values, 'linear'));
            return channel;
        }

        // 形式 3：时间 → 关键帧对象
        if (typeof channelData === 'object') {
            for (const [timeStr, kfData] of Object.entries(channelData)) {
                const time = parseFloat(timeStr);
                if (isNaN(time)) continue;
                const kf = AnimationCodec.parseKeyframe(time, kfData);
                // 对关键帧的 pre/post 值应用轴取反
                if (kf.post) AnimationCodec._applyBedrockAxisFix(channelName, kf.post);
                if (kf.pre) AnimationCodec._applyBedrockAxisFix(channelName, kf.pre);
                channel.addKeyframe(kf);
            }
        }

        return channel;
    }

    /**
     * Bedrock 轴取反：rotation 的 X/Y 取反，position 的 X 取反
     * 参考 Blockbench bedrock_animation.js L186-189 的 invertMolang 逻辑
     * @param {string} channelName
     * @param {Array} values [x, y, z] 原地修改
     */
    static _applyBedrockAxisFix(channelName, values) {
        if (!values || values.length < 3) return;
        if (channelName === 'rotation') {
            values[0] = AnimationCodec._invertValue(values[0]);  // X 取反
            values[1] = AnimationCodec._invertValue(values[1]);  // Y 取反
        } else if (channelName === 'position') {
            values[0] = AnimationCodec._invertValue(values[0]);  // X 取反
        }
    }

    /**
     * 取反一个值（数字或 MoLang 字符串）
     * @param {number|string} v
     * @returns {number|string}
     */
    static _invertValue(v) {
        if (typeof v === 'number') return -v;
        if (typeof v === 'string') {
            // 纯数字字符串
            const num = parseFloat(v);
            if (!isNaN(num) && /^-?\d+\.?\d*$/.test(v.trim())) return (-num).toString();
            // MoLang 表达式：加负号（简化处理）
            if (v.startsWith('-')) return v.substring(1);
            return `(-${v})`;
        }
        return v;
    }

    /**
     * 解析单个关键帧
     * @param {number} time
     * @param {Array|object} kfData
     * @returns {Keyframe}
     */
    static parseKeyframe(time, kfData) {
        let pre = null;
        let post = null;
        let lerpMode = 'linear';

        if (Array.isArray(kfData)) {
            // [x, y, z]
            post = kfData.map(v => AnimationCodec._normalizeValue(v));
            pre = post;
        } else if (typeof kfData === 'object' && kfData) {
            // { pre, post, lerp_mode }
            if (kfData.pre) {
                pre = kfData.pre.map(v => AnimationCodec._normalizeValue(v));
            }
            if (kfData.post) {
                post = kfData.post.map(v => AnimationCodec._normalizeValue(v));
            }
            if (kfData.lerp_mode) {
                lerpMode = kfData.lerp_mode;
            }
            // 兼容 Blockbench 的 bezier 控制点字段（这里降级为 catmullrom）
            if (lerpMode === 'bezier') {
                lerpMode = 'catmullrom';
            }
        } else if (typeof kfData === 'number' || typeof kfData === 'string') {
            const v = AnimationCodec._normalizeValue(kfData);
            post = [v, v, v];
            pre = post;
        }

        if (!post) post = [0, 0, 0];
        if (!pre) pre = null; // null 时使用 post

        return new Keyframe(time, pre, post, lerpMode);
    }

    /**
     * 归一化关键帧值：数字原样返回，字符串保留为 MoLang 表达式
     * @param {number|string} v
     * @returns {number|string}
     */
    static _normalizeValue(v) {
        if (typeof v === 'number') return v;
        if (typeof v === 'string') {
            // 尝试解析为数字字符串
            const num = parseFloat(v);
            if (!isNaN(num) && /^-?\d+\.?\d*$/.test(v.trim())) return num;
            // 否则作为 MoLang 表达式保留
            return v;
        }
        return 0;
    }
}
