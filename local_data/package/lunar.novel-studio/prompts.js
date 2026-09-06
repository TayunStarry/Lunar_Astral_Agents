/**
 * 全局枚举（最先加载，供所有模块引用）
 */
var PHASE = {
    IDLE: 'idle',
    BUILDING: 'building',
    GENERATING_PARAGRAPHS: 'generating_paragraphs',
    POLISHING_PARAGRAPHS: 'polishing_paragraphs',
    WORD_REVIEW: 'word_review',
    CONTENT_REVIEW: 'content_review',
    CHAPTER_REVIEW: 'chapter_review',
    FINAL_POLISH: 'final_polish',
    HUMAN_REVIEW: 'human_review',
    GENERATING_SUMMARY: 'generating_summary',
    INTERRUPTED: 'interrupted'
};
var CHAPTER_STATUS = { DRAFT: 'draft', APPROVED: 'approved' };

/**
 * Prompt 加载器模块
 * 负责 prompts/*.md 文件的加载、缓存、占位符替换
 */

(function(global) {
    'use strict';

    // ==== PromptLoader 类 ====
    class PromptLoader {
        // prompt 文件根路径
        static DIR = 'lunar.studio.novel.pro/prompts';
        static cache = {};

        // ==== 加载 prompt 文件（带缓存） ====
        static async load(name) {
            if (PromptLoader.cache[name]) return PromptLoader.cache[name];
            var url = '/file/read/package/' + PromptLoader.DIR + '/' + name + '.md';
            var resp = await fetch(url);
            if (!resp.ok) throw new Error('加载 prompt 失败: ' + name + ' (' + resp.status + ')');
            var text = await resp.text();
            PromptLoader.cache[name] = text;
            return text;
        }

        // ==== 填充 {{变量名}} 占位符 ====
        static fill(template, vars) {
            if (!template) return '';
            if (!vars) return template;
            return template.replace(/\{\{(\w+)\}\}/g, function(match, key) {
                if (Object.prototype.hasOwnProperty.call(vars, key)) {
                    var val = vars[key];
                    if (val === null || val === undefined) return '';
                    if (typeof val === 'object') return JSON.stringify(val, null, 2);
                    return String(val);
                }
                return match;
            });
        }

        // ==== 加载并填充 ====
        static async loadAndFill(name, vars) {
            var template = await PromptLoader.load(name);
            return PromptLoader.fill(template, vars);
        }

        // ==== 清空缓存 ====
        static clearCache() {
            PromptLoader.cache = {};
        }
    }

    // ==== 暴露到全局 ====
    global.PromptLoader = PromptLoader;
})(typeof window !== 'undefined' ? window : this);
