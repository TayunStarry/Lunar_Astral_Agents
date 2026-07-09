/**
 * Prompt 加载与填充模块
 * 负责 prompts/*.md 文件的加载、缓存、占位符替换。
 *
 * 设计契约（grilling 问题 11）：
 *   - prompt 文件使用 {{变量名}} 占位符
 *   - JS 端字符串替换，沿用 lunar.studio.novel 的 fillPrompt 模式
 *   - 内存缓存避免重复 fetch
 */

(function (global) {
    'use strict';

    /** prompt 文件根路径（相对 /file/read/package/） */
    const PROMPT_DIR = 'lunar.studio.novel_pro/prompts';

    /** 已加载的 prompt 缓存 */
    const promptCache = {};

    /**
     * PromptLoader 类
     * 负责加载 prompt 文件、填充占位符。
     */
    class PromptLoader {
        /**
         * 加载 prompt 文件内容（带缓存）。
         *
         * @param {string} name prompt 名（不含扩展名，如 process_outline）
         * @returns {Promise<string>} prompt 模板文本
         */
        static async load(name) {
            if (promptCache[name]) return promptCache[name];
            const url = '/file/read/package/' + PROMPT_DIR + '/' + name + '.md';
            const response = await fetch(url);
            if (!response.ok) {
                throw new Error('加载 prompt 失败: ' + name + ' (' + response.status + ')');
            }
            const text = await response.text();
            promptCache[name] = text;
            return text;
        }

        /**
         * 预加载所有 prompt（启动时调用）。
         */
        static async preloadAll() {
            const names = [
                'process_outline',
                'build_paragraph_outlines',
                'generate_paragraph',
                'format_check',
                'review_chapter_outline',
                'review_total_outline',
                'review_paragraph',
                'rewrite_paragraph',
                'generate_summary'
            ];
            const results = await Promise.allSettled(names.map(function (n) { return PromptLoader.load(n); }));
            const failed = [];
            results.forEach(function (r, i) {
                if (r.status === 'rejected') failed.push(names[i] + ': ' + r.reason.message);
            });
            if (failed.length > 0) {
                console.warn('[PromptLoader] 部分 prompt 加载失败:', failed);
            }
            return { loaded: names.length - failed.length, failed: failed };
        }

        /**
         * 填充占位符。
         * 模板中使用 {{变量名}} 形式，本函数将其替换为 vars 中的对应值。
         *
         * @param {string} template prompt 模板
         * @param {object} vars 变量键值对
         * @returns {string} 填充后的文本
         */
        static fill(template, vars) {
            if (!template) return '';
            if (!vars) return template;
            return template.replace(/\{\{(\w+)\}\}/g, function (match, key) {
                if (Object.prototype.hasOwnProperty.call(vars, key)) {
                    const val = vars[key];
                    if (val === null || val === undefined) return '';
                    if (typeof val === 'object') return JSON.stringify(val, null, 2);
                    return String(val);
                }
                return match; // 未找到变量，保留原占位符
            });
        }

        /**
         * 加载并填充 prompt（便捷方法）。
         *
         * @param {string} name prompt 名
         * @param {object} vars 变量键值对
         * @returns {Promise<string>} 填充后的文本
         */
        static async loadAndFill(name, vars) {
            const template = await PromptLoader.load(name);
            return PromptLoader.fill(template, vars);
        }

        /**
         * 清空缓存（调试用）。
         */
        static clearCache() {
            Object.keys(promptCache).forEach(function (k) { delete promptCache[k]; });
        }
    }

    // 暴露到全局
    global.PromptLoader = PromptLoader;
})(typeof window !== 'undefined' ? window : this);
