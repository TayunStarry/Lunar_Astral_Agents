// ==== animation-classifier.js — 动画分类识别系统 ====
//
// 根据动画名称自动分类：
//   .blink     → 眨眼动画（随机间隔 3-5 秒触发，播放一次）
//   .move      → 移动动画（模型移动时触发，循环播放）
//   .fast_move → 长距离移动动画（快速移动时触发，循环播放）
//   其他       → 普通动画（显示在可用动画列表中，可加入动画组）

/**
 * 动画类别枚举
 */
export const AnimationCategory = {
    /** 普通动画（可加入动画组） */
    NORMAL: 'normal',
    /** 眨眼动画（随机触发） */
    BLINK: 'blink',
    /** 移动动画（移动时触发） */
    MOVE: 'move',
    /** 长距离移动动画（快速移动时触发） */
    FAST_MOVE: 'fast_move'
};

/**
 * 动画分类器
 *
 * 分类规则（按优先级从高到低）：
 *   1. 名称包含 '.fast_move' → FAST_MOVE
 *   2. 名称包含 '.move'      → MOVE（注意：'.fast_move' 已被上面的规则捕获）
 *   3. 名称包含 '.blink'     → BLINK
 *   4. 其他                   → NORMAL
 */
export class AnimationClassifier {
    /**
     * 分类单个动画
     * @param {string} fullName 动画全名（如 animation.entity.blink）
     * @returns {AnimationCategory}
     */
    static classify(fullName) {
        if (!fullName || typeof fullName !== 'string') return AnimationCategory.NORMAL;
        const name = fullName.toLowerCase();
        // 注意优先级：先检查 fast_move，再检查 move
        if (name.includes('.fast_move') || name.includes('_fast_move')) {
            return AnimationCategory.FAST_MOVE;
        }
        if (name.includes('.move') || name.includes('_move')) {
            return AnimationCategory.MOVE;
        }
        if (name.includes('.blink') || name.includes('_blink')) {
            return AnimationCategory.BLINK;
        }
        return AnimationCategory.NORMAL;
    }

    /**
     * 判断是否为特殊动画（不显示在可用动画列表中）
     * @param {string} fullName
     * @returns {boolean}
     */
    static isSpecial(fullName) {
        return this.classify(fullName) !== AnimationCategory.NORMAL;
    }

    /**
     * 过滤出可显示的动画（排除特殊动画）
     * @param {Map<string, *>|Array<string>} animations
     * @returns {Map<string, *>|Array<string>} 与输入同类型
     */
    static filterDisplayAnimations(animations) {
        if (animations instanceof Map) {
            const result = new Map();
            for (const [name, anim] of animations) {
                if (!this.isSpecial(name)) {
                    result.set(name, anim);
                }
            }
            return result;
        }
        if (Array.isArray(animations)) {
            return animations.filter(name => !this.isSpecial(name));
        }
        return animations;
    }

    /**
     * 获取指定类别的动画名列表
     * @param {Map<string, *>|Array<string>} animations
     * @param {AnimationCategory} category
     * @returns {string[]}
     */
    static getByCategory(animations, category) {
        const names = animations instanceof Map
            ? Array.from(animations.keys())
            : animations;
        return names.filter(name => this.classify(name) === category);
    }

    /**
     * 对动画表进行完整分类
     * @param {Map<string, *>} animations
     * @returns {{normal: string[], blink: string[], move: string[], fastMove: string[]}}
     */
    static categorize(animations) {
        const names = animations instanceof Map
            ? Array.from(animations.keys())
            : animations;
        return {
            normal: names.filter(n => this.classify(n) === AnimationCategory.NORMAL),
            blink: names.filter(n => this.classify(n) === AnimationCategory.BLINK),
            move: names.filter(n => this.classify(n) === AnimationCategory.MOVE),
            fastMove: names.filter(n => this.classify(n) === AnimationCategory.FAST_MOVE)
        };
    }
}
