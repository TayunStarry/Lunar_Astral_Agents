/**
 * 格式化与算法工具模块
 * 提供步骤 4 格式校验、段落切分（步骤 7/8 前置）、向量余弦相似度计算等纯函数。
 *
 * 设计契约：
 *   - 段落切分：目标 400 字，下限 200，上限 600（grilling 问题 7）
 *   - 格式校验：正则扫描 + 5% 阈值触发 AI 兜底（grilling 问题 9）
 *   - 余弦相似度：标准内积 / 模长乘积
 */

(function (global) {
    'use strict';

    /** ==================== 余弦相似度 ==================== */

    /**
     * 计算两个向量的余弦相似度。
     *
     * @param {number[]} vecA 向量 A
     * @param {number[]} vecB 向量 B
     * @returns {number} 余弦相似度，范围 [-1, 1]；维度不匹配返回 -1
     */
    function cosineSimilarity(vecA, vecB) {
        if (!vecA || !vecB || vecA.length !== vecB.length || vecA.length === 0) {
            return -1;
        }
        let dot = 0;
        let normA = 0;
        let normB = 0;
        for (let i = 0; i < vecA.length; i++) {
            const a = vecA[i];
            const b = vecB[i];
            dot += a * b;
            normA += a * a;
            normB += b * b;
        }
        const denom = Math.sqrt(normA) * Math.sqrt(normB);
        if (denom === 0) return 0;
        return dot / denom;
    }

    /** ==================== 段落切分（步骤 7/8 前置） ==================== */

    /**
     * 将章节内容切分为评审单元。
     * 算法：按 \n\n 拆分，滑动窗口合并至目标字数，避免过短或过长段落。
     *
     * @param {string} content 章节正文（含 H1 标题）
     * @param {object} options 切分参数
     * @returns {object[]} 段落数组，每项 {index, content, wordCount}
     */
    function segmentParagraphs(content, options) {
        const opt = Object.assign({
            targetLen: 400,
            minLen: 200,
            maxLen: 600
        }, options || {});

        if (!content || !content.trim()) return [];

        const normalized = content.replace(/\r\n/g, '\n').trim();
        // 去除 H1 标题行（不参与段落切分）
        const lines = normalized.split('\n');
        const filtered = lines.filter(function (line, idx) {
            if (idx === 0 && /^#\s+/.test(line)) return false; // 首行 H1 标题
            return true;
        });
        const bodyText = filtered.join('\n').trim();
        if (!bodyText) return [];

        const rawParagraphs = bodyText.split(/\n{2,}/).map(function (p) { return p.trim(); }).filter(function (p) { return p.length > 0; });
        if (rawParagraphs.length === 0) return [];

        const paragraphs = [];
        let buffer = '';

        for (const para of rawParagraphs) {
            if (buffer === '') {
                buffer = para;
                continue;
            }
            // 若当前 buffer 已达上限，提交并开新 buffer
            if (buffer.length >= opt.maxLen) {
                pushParagraph(paragraphs, buffer);
                buffer = para;
                continue;
            }
            // 若合并后超过上限，提交 buffer，para 开新 buffer
            const merged = buffer + '\n\n' + para;
            if (merged.length > opt.maxLen) {
                pushParagraph(paragraphs, buffer);
                buffer = para;
            } else if (buffer.length < opt.minLen) {
                // buffer 未达下限，合并 para
                buffer = merged;
            } else if (merged.length <= opt.targetLen) {
                // 合并后仍在目标范围内，合并
                buffer = merged;
            } else {
                // buffer 已达目标，提交，para 开新 buffer
                pushParagraph(paragraphs, buffer);
                buffer = para;
            }
        }
        if (buffer) pushParagraph(paragraphs, buffer);

        // 修正：若最后一段过短，合并到前一段
        if (paragraphs.length >= 2 && paragraphs[paragraphs.length - 1].wordCount < opt.minLen / 2) {
            const last = paragraphs.pop();
            const prev = paragraphs[paragraphs.length - 1];
            prev.content = prev.content + '\n\n' + last.content;
            prev.wordCount = prev.content.length;
        }

        // 重新编号
        paragraphs.forEach(function (p, i) { p.index = i; });
        return paragraphs;
    }

    function pushParagraph(arr, content) {
        arr.push({
            index: arr.length,
            content: content,
            wordCount: content.length
        });
    }

    /** ==================== 步骤 4 格式校验 ==================== */

    /**
     * 对章节内容执行正则扫描式格式校验。
     * 返回 cleaned 文本与改动比例；若改动比例 > 5%，由调用方触发 AI 兜底。
     *
     * @param {string} rawContent 原始章节内容
     * @returns {object} {cleaned, changeRatio, formatLog, needsAiReview}
     */
    function formatChapterContent(rawContent) {
        const formatLog = [];
        if (!rawContent) {
            return { cleaned: '', changeRatio: 0, formatLog: formatLog, needsAiReview: false };
        }
        const originalLength = rawContent.length;
        let cleaned = rawContent;

        // 1. 移除 AI 思考标签：<think>...</think>、<reasoning>...</reasoning>、<reflection>...</reflection>、<analysis>...</analysis>
        cleaned = cleaned.replace(/<(think|reasoning|reflection|analysis)\b[^>]*>[\s\S]*?<\/\1>/gi, function () {
            formatLog.push('removed_ai_think_tag');
            return '';
        });
        // 处理未闭合的思考标签（只有开标签到末尾）
        cleaned = cleaned.replace(/<(think|reasoning|reflection|analysis)\b[^>]*>[\s\S]*$/gi, function () {
            formatLog.push('removed_unclosed_think_tag');
            return '';
        });

        // 2. 移除代码围栏块（小说正文不应有代码）
        cleaned = cleaned.replace(/```[\w]*\n[\s\S]*?```/g, function () {
            formatLog.push('removed_code_fence');
            return '';
        });

        // 3. 移除未闭合的代码围栏
        cleaned = cleaned.replace(/```[\w]*\n[\s\S]*$/g, function () {
            formatLog.push('removed_unclosed_fence');
            return '';
        });

        // 4. 移除裸 JSON 块（行首开始，连续多行 JSON 语法）
        // 匹配 { ... } 或 [ ... ] 形式，要求至少跨 2 行或单行超过 30 字符
        cleaned = cleaned.replace(/(^|\n)[ \t]*[\{\[][\s\S]{10,}?[\}\]][ \t]*(?=\n|$)/g, function (match, prefix) {
            // 排除小说中合法的简短 JSON 引用（如 "{key}" 在对话中）
            if (match.length < 30 && match.indexOf('\n', 1) === -1) return match;
            formatLog.push('removed_json_block');
            return prefix;
        });

        // 5. 移除 AI 残留的 system prompt 痕迹（启发式：以"作为AI助手"、"我理解"、"好的，我将"开头的独立段）
        cleaned = cleaned.replace(/\n[ \t]*(作为AI助手|我理解你的要求|好的，我将|我将按照|根据你的要求)[^\n]*\n/g, function (match) {
            formatLog.push('removed_ai_preamble');
            return '\n';
        });

        // 6. 规范化 H1 标题：若首行非 H1，且内容包含"第X章"，自动加 H1
        cleaned = cleaned.replace(/^\s*第[一二三四五六七八九十百零\d]+章[^\n]*\n/, function (match) {
            const trimmed = match.trim();
            if (/^#\s+/.test(trimmed)) return match;
            return '# ' + trimmed + '\n';
        });

        // 7. 移除段落中间的标题行（##/### 等），降级为加粗
        cleaned = cleaned.replace(/\n(#{2,6})\s+([^\n]+)\n/g, function (match, hashes, title) {
            formatLog.push('demoted_inline_heading');
            return '\n**' + title.trim() + '**\n';
        });

        // 8. 压缩多余空行（3+ 连续换行 → 2 换行）
        cleaned = cleaned.replace(/\n{3,}/g, '\n\n');

        // 9. 头尾 trim
        cleaned = cleaned.trim() + '\n';

        const changeRatio = originalLength > 0 ? (1 - cleaned.length / originalLength) : 0;
        const needsAiReview = changeRatio > 0.05;

        return {
            cleaned: cleaned,
            changeRatio: changeRatio,
            formatLog: formatLog,
            needsAiReview: needsAiReview
        };
    }

    /** ==================== 工具函数 ==================== */

    /**
     * 统计中文字数（粗略：去除空白与标点后的字符数）。
     *
     * @param {string} text 文本
     * @returns {number} 字数
     */
    function countWords(text) {
        if (!text) return 0;
        // 去除空白与常见标点
        const stripped = text.replace(/[\s，。、；：！？""''（）【】《》\-—…·,.!?;:'"()\[\]{}<>\\\/|`~@#$%^&*_+=]/g, '');
        return stripped.length;
    }

    /**
     * 生成形如 sp-001 / ch-1 的 ID。
     *
     * @param {string} prefix 前缀
     * @param {number} seq 序号
     * @param {number} pad 填充位数
     * @returns {string} ID
     */
    function formatId(prefix, seq, pad) {
        const padLen = pad || 3;
        return prefix + '-' + String(seq).padStart(padLen, '0');
    }

    /**
     * 生成 ISO8601 时间戳。
     */
    function nowIso() {
        return new Date().toISOString();
    }

    // 暴露到全局
    global.NovelStudioProFormatters = {
        cosineSimilarity: cosineSimilarity,
        segmentParagraphs: segmentParagraphs,
        formatChapterContent: formatChapterContent,
        countWords: countWords,
        formatId: formatId,
        nowIso: nowIso
    };
})(typeof window !== 'undefined' ? window : this);
