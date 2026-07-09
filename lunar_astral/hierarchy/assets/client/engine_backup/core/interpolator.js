// ==== interpolator.js — 关键帧插值算法 ====

/**
 * Bedrock 动画插值
 * 支持：linear / step / catmullrom / bezier
 * 参考 Blockbench js/animations/timeline_animators.js:interpolate (行 423)
 */

/**
 * 标量线性插值
 */
export function lerp(a, b, t) {
    return a + (b - a) * t;
}

/**
 * 三维向量线性插值
 * @param {Array<number>} a
 * @param {Array<number>} b
 * @param {number} t
 * @returns {Array<number>}
 */
export function lerpVec3(a, b, t) {
    return [
        lerp(a[0], b[0], t),
        lerp(a[1], b[1], t),
        lerp(a[2], b[2], t)
    ];
}

/**
 * 阶跃插值（保持 prev 值直到到达 next 时间点）
 */
export function stepVec3(a) {
    return [a[0], a[1], a[2]];
}

/**
 * Catmull-Rom 标量插值
 * 在 p1 和 p2 之间插值，p0 和 p3 是控制点
 * @param {number} p0
 * @param {number} p1
 * @param {number} p2
 * @param {number} p3
 * @param {number} t [0,1]
 * @returns {number}
 */
export function catmullRom(p0, p1, p2, p3, t) {
    const t2 = t * t;
    const t3 = t2 * t;
    return 0.5 * (
        (2 * p1) +
        (-p0 + p2) * t +
        (2 * p0 - 5 * p1 + 4 * p2 - p3) * t2 +
        (-p0 + 3 * p1 - 3 * p2 + p3) * t3
    );
}

/**
 * Catmull-Rom 三维向量插值
 * @param {Array<number>} p0
 * @param {Array<number>} p1
 * @param {Array<number>} p2
 * @param {Array<number>} p3
 * @param {number} t
 * @returns {Array<number>}
 */
export function catmullRomVec3(p0, p1, p2, p3, t) {
    return [
        catmullRom(p0[0], p1[0], p2[0], p3[0], t),
        catmullRom(p0[1], p1[1], p2[1], p3[1], t),
        catmullRom(p0[2], p1[2], p2[2], p3[2], t)
    ];
}

/**
 * 三维贝塞尔插值（简化版：使用线性插值代替，因为 Bedrock 极少使用）
 * 真正的 bezier 需要控制点数据，这里降级为 linear
 */
export function bezierVec3(a, b, t) {
    return lerpVec3(a, b, t);
}

/**
 * 对通道在指定时间点进行插值
 * @param {import('./keyframe.js').Channel} channel
 * @param {number} time 当前时间（秒）
 * @param {object} molang MoLang 运行时（用于求值表达式）
 * @returns {Array<number>|null} [x, y, z] 或 null（通道无关键帧）
 */
export function interpolate(channel, time, molang) {
    if (!channel || channel.keyframes.length === 0) return null;

    const kfs = channel.keyframes;
    const { prev, next, alpha, prevIndex, nextIndex } = channel.findPair(time);

    // 时间在第一个关键帧之前
    if (!prev) {
        return next.getPre(molang);
    }
    // 时间在最后一个关键帧之后
    if (!next) {
        return prev.getPost(molang);
    }

    const lerpMode = next.lerpMode || 'linear';
    const prevPost = prev.getPost(molang);
    const nextPre = next.getPre(molang);

    switch (lerpMode) {
        case 'step':
            return stepVec3(prevPost);

        case 'linear':
            return lerpVec3(prevPost, nextPre, alpha);

        case 'catmullrom': {
            // 需要 4 个控制点：p0 (prev 之前), p1 (prev), p2 (next), p3 (next 之后)
            const p0Kf = channel.getKeyframeBefore(prevIndex);
            const p3Kf = channel.getKeyframeAfter(nextIndex);
            const p0 = p0Kf ? p0Kf.getPost(molang) : prevPost;
            const p1 = prevPost;
            const p2 = nextPre;
            const p3 = p3Kf ? p3Kf.getPost(molang) : nextPre;
            return catmullRomVec3(p0, p1, p2, p3, alpha);
        }

        case 'bezier':
        case 'smooth':
            // Bedrock 的 bezier 需要控制点，这里降级为 catmullrom 以获得平滑效果
            {
                const p0Kf = channel.getKeyframeBefore(prevIndex);
                const p3Kf = channel.getKeyframeAfter(nextIndex);
                const p0 = p0Kf ? p0Kf.getPost(molang) : prevPost;
                const p1 = prevPost;
                const p2 = nextPre;
                const p3 = p3Kf ? p3Kf.getPost(molang) : nextPre;
                return catmullRomVec3(p0, p1, p2, p3, alpha);
            }

        default:
            return lerpVec3(prevPost, nextPre, alpha);
    }
}

/**
 * 对所有三个通道进行插值
 * @param {import('./keyframe.js').BoneAnimation} boneAnim
 * @param {number} time
 * @param {object} molang
 * @returns {{rotation: Array<number>|null, position: Array<number>|null, scale: Array<number>|null}}
 */
export function interpolateBone(boneAnim, time, molang) {
    return {
        rotation: boneAnim.rotation ? interpolate(boneAnim.rotation, time, molang) : null,
        position: boneAnim.position ? interpolate(boneAnim.position, time, molang) : null,
        scale: boneAnim.scale ? interpolate(boneAnim.scale, time, molang) : null
    };
}
