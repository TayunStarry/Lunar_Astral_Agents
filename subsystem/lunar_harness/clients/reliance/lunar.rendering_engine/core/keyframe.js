// ==== keyframe.js — 关键帧与通道数据结构 ====

/**
 * 关键帧 — 单个时间点的值
 * 参考 Blockbench js/animations/keyframe.js
 *
 * Bedrock 关键帧支持 pre/post 双值：
 *   - pre：在该时间点之前使用的值（用于 catmullrom/bezier 的左侧控制点）
 *   - post：在该时间点之后使用的值（用于插值的右侧控制点）
 *
 * 值元素可以是数字（常量）或字符串（MoLang 表达式，运行时求值）
 */
export class Keyframe {
    /**
     * @param {number} time 时间（秒）
     * @param {Array<number|string>|null} pre pre 值 [x,y,z]，null 时使用 post
     * @param {Array<number|string>} post post 值 [x,y,z]
     * @param {string} lerpMode linear | step | catmullrom | bezier
     */
    constructor(time, pre, post, lerpMode = 'linear') {
        this.time = time;
        this.pre = pre ? [...pre] : null;
        this.post = [...post];
        this.lerpMode = lerpMode;
    }

    /**
     * 解析关键帧值为数字数组（运行时求值 MoLang 表达式）
     * @param {Array<number|string>} values
     * @param {object} molang MoLang 运行时（可选）
     * @returns {Array<number>}
     */
    static resolveValues(values, molang) {
        if (!values) return [0, 0, 0];
        return values.map(v => {
            if (typeof v === 'number') return v;
            if (typeof v === 'string') {
                if (!molang) return 0;
                return molang.eval(v);
            }
            return 0;
        });
    }

    /**
     * 获取 pre 值（若无 pre，使用 post）
     * @param {object} molang
     * @returns {Array<number>}
     */
    getPre(molang) {
        return Keyframe.resolveValues(this.pre || this.post, molang);
    }

    /**
     * 获取 post 值
     * @param {object} molang
     * @returns {Array<number>}
     */
    getPost(molang) {
        return Keyframe.resolveValues(this.post, molang);
    }
}

/**
 * 通道 — 一个骨骼的某个属性（rotation/position/scale）的所有关键帧
 */
export class Channel {
    /**
     * @param {string} name 'rotation' | 'position' | 'scale'
     */
    constructor(name) {
        this.name = name;
        /** @type {Keyframe[]} 按时间升序 */
        this.keyframes = [];
    }

    /**
     * 添加关键帧并保持时间排序
     * @param {Keyframe} kf
     */
    addKeyframe(kf) {
        this.keyframes.push(kf);
        this.keyframes.sort((a, b) => a.time - b.time);
    }

    /**
     * 查找指定时间点周围的关键帧对
     * @param {number} time
     * @returns {{prev: Keyframe|null, next: Keyframe|null, alpha: number, prevIndex: number, nextIndex: number}}
     */
    findPair(time) {
        const kfs = this.keyframes;
        if (kfs.length === 0) {
            return { prev: null, next: null, alpha: 0, prevIndex: -1, nextIndex: -1 };
        }
        if (time <= kfs[0].time) {
            return { prev: null, next: kfs[0], alpha: 0, prevIndex: -1, nextIndex: 0 };
        }
        if (time >= kfs[kfs.length - 1].time) {
            return { prev: kfs[kfs.length - 1], next: null, alpha: 0, prevIndex: kfs.length - 1, nextIndex: -1 };
        }
        // 二分查找
        let lo = 0, hi = kfs.length - 1;
        while (lo <= hi) {
            const mid = (lo + hi) >> 1;
            if (kfs[mid].time <= time) lo = mid + 1;
            else hi = mid - 1;
        }
        const prevIdx = hi;
        const nextIdx = hi + 1;
        const prev = kfs[prevIdx];
        const next = kfs[nextIdx];
        const span = next.time - prev.time;
        const alpha = span > 0 ? (time - prev.time) / span : 0;
        return { prev, next, alpha, prevIndex: prevIdx, nextIndex: nextIdx };
    }

    /**
     * 获取指定索引之前的关键帧（用于 catmullrom 的 p0）
     * @param {number} index
     * @returns {Keyframe|null}
     */
    getKeyframeBefore(index) {
        return index > 0 ? this.keyframes[index - 1] : null;
    }

    /**
     * 获取指定索引之后的关键帧（用于 catmullrom 的 p3）
     * @param {number} index
     * @returns {Keyframe|null}
     */
    getKeyframeAfter(index) {
        return index < this.keyframes.length - 1 ? this.keyframes[index + 1] : null;
    }

    /**
     * 获取动画总时长（最后一个关键帧的时间）
     */
    get duration() {
        return this.keyframes.length > 0 ? this.keyframes[this.keyframes.length - 1].time : 0;
    }
}

/**
 * 骨骼动画 — 单个骨骼的所有通道
 */
export class BoneAnimation {
    /**
     * @param {string} boneName
     */
    constructor(boneName) {
        this.boneName = boneName;
        /** @type {Channel|null} */
        this.rotation = null;
        /** @type {Channel|null} */
        this.position = null;
        /** @type {Channel|null} */
        this.scale = null;
    }
}

/**
 * 动画 — 完整的动画定义
 * 参考 Blockbench js/animations/animation.js
 */
export class Animation {
    /**
     * @param {string} name
     */
    constructor(name) {
        this.name = name;
        /** @type {boolean|string} true=循环 | false=播放一次 | 'hold_on_last_frame'=停在最后一帧 */
        this.loop = false;
        /** @type {number} 动画时长（秒） */
        this.animationLength = 0;
        /** @type {Map<string, BoneAnimation>} boneName → BoneAnimation */
        this.bones = new Map();
        /** @type {boolean} 是否覆盖根骨骼动画 */
        this.overrideRootAnimation = false;
        /** @type {Array<string>} 依赖的动画（用于 blend_transition 等） */
        this.blendWeight = null;
    }
}
