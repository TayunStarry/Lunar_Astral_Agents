/**
 * 文本分割模块 - 移植自 lunar_astral/server_side/file/split.ts
 * 提供按 Markdown 标题层级或普通文本规则拆分长文本的能力，供知识点批量入库时切片使用。
 *
 * 设计契约（见 grilling 问题 5）：
 *   - idealLen 默认 800（中文约 1200 token，匹配嵌入模型最佳区间）
 *   - pathPrefix / pathOnNewLine / skipTitleOnly / includeOriginalTitle 沿用 split.ts 默认值
 *   - 暴露全局 window.splitTextToStrings 供主程序调用
 */

(function (global) {
    'use strict';

    /** 文本分割选项 */
    const DEFAULT_OPTIONS = {
        idealLen: 800,
        pathPrefix: '*标题> ',
        pathOnNewLine: true,
        skipTitleOnly: true,
        includeOriginalTitle: false
    };

    /** 合并默认选项 */
    function mergeOptions(options) {
        const merged = Object.assign({}, DEFAULT_OPTIONS, options || {});
        return merged;
    }

    /**
     * 将输入文本按指定长度拆分成若干字符串片段。
     * 若文本像 Markdown，则按标题层级拆分并附带路径前缀；否则按普通文本规则拆分。
     *
     * @param {string} input 原始文本
     * @param {object} options 拆分行为选项
     * @returns {string[]} 拆分后的字符串数组
     */
    function splitTextToStrings(input, options) {
        const option = mergeOptions(options);
        const text = (input || '').replace(/\r\n/g, '\n');
        if (!text.trim()) return [];

        const isMarkdown = looksLikeMarkdown(text);
        if (!isMarkdown) {
            return splitPlainText(text, option.idealLen);
        }
        return splitMarkdown(text, option);
    }

    /** ==================== Plain Text ==================== */

    /**
     * 将普通文本按指定长度拆分成若干字符串片段。
     *
     * @param {string} text 原始普通文本
     * @param {number} idealLen 理想单段长度
     * @returns {string[]} 拆分后的字符串数组
     */
    function splitPlainText(text, idealLen) {
        const results = [];
        let currentIndex = 0;

        const isPreferredBreak = function (char) {
            return char === '\n' ||
                char === '。' || char === '；' || char === ';' ||
                char === '.' || char === '!' || char === '?' ||
                char === '？' || char === '！' || char === '…' ||
                char === '、' || char === ':' || char === '：';
        };

        while (currentIndex < text.length) {
            const remainingLength = text.length - currentIndex;
            if (remainingLength <= idealLen) {
                const tailText = text.slice(currentIndex).trim();
                if (tailText) results.push(tailText);
                break;
            }

            const endPosition = currentIndex + idealLen;
            const backwardWindow = Math.min(idealLen, 256);
            let cutPosition = -1;

            // 在回退窗口内查找首选断点
            for (let position = endPosition; position >= Math.max(currentIndex + 1, endPosition - backwardWindow); position--) {
                const char = text[position - 1];
                if (isPreferredBreak(char)) {
                    cutPosition = position;
                    break;
                }
            }

            // 回退窗口内未找到首选断点，扩大回退范围查找普通断点
            if (cutPosition === -1) {
                for (let position = endPosition; position > currentIndex; position--) {
                    const char = text[position - 1];
                    if (isPreferredBreak(char)) {
                        cutPosition = position;
                        break;
                    }
                }
            }

            // 仍未找到断点，硬切
            if (cutPosition === -1 || cutPosition <= currentIndex) cutPosition = endPosition;

            const chunkText = text.slice(currentIndex, cutPosition).trim();
            if (chunkText) results.push(chunkText);
            currentIndex = cutPosition;
        }

        return results;
    }

    /** ==================== Markdown ==================== */

    /**
     * 将 Markdown 文本按标题层级拆分成若干片段，每段不超过理想长度。
     *
     * @param {string} text 原始 Markdown 文本
     * @param {object} option 已合并默认值的拆分选项
     * @returns {string[]} 拆分后的字符串数组
     */
    function splitMarkdown(text, option) {
        const sections = parseMarkdownSections(text);
        if (sections.length === 0) {
            return splitPlainText(text, option.idealLen);
        }

        const output = [];
        for (const sec of sections) {
            if (option.skipTitleOnly && sec.content.trim() === '') {
                continue;
            }
            const header = formatPath(sec.path, option);
            const body = option.includeOriginalTitle
                ? (sec.title ? '#'.repeat(sec.level) + ' ' + sec.title + '\n' : '') + sec.content
                : sec.content;

            if (body.length <= option.idealLen) {
                const piece = (header + body).trimEnd();
                if (piece.trim()) output.push(piece);
                continue;
            }

            const pieces = splitByNewlinePrefer(body, option.idealLen);
            for (const current of pieces) {
                const piece = (header + current).trimEnd();
                if (piece.trim()) output.push(piece);
            }
        }

        return output;
    }

    /**
     * 解析 Markdown 文本，将其按标题层级拆分成若干段落。
     *
     * @param {string} text 原始 Markdown 文本
     * @returns {object[]} 解析后的段落数组
     */
    function parseMarkdownSections(text) {
        const normalizedText = text.replace(/\r\n/g, '\n');
        const lines = normalizedText.split('\n');
        const headingRe = /^(#{1,6})\s+(.*)\s*$/;
        const sections = [];
        const stack = [];
        const headingIdx = [];

        for (let index = 0; index < lines.length; index++) {
            const line = lines[index];
            const match = line.match(headingRe);
            if (match) {
                headingIdx.push({ i: index, level: match[1].length, title: match[2].trim() });
            }
        }

        if (headingIdx.length === 0) return [];

        for (let k = 0; k < headingIdx.length; k++) {
            const cur = headingIdx[k];
            const next = headingIdx[k + 1];
            const startLine = cur.i;
            const endLine = next ? next.i : lines.length;

            while (stack.length && stack[stack.length - 1].level >= cur.level) {
                stack.pop();
            }
            stack.push({ level: cur.level, title: cur.title });

            const path = stack.map(function (s) { return s.title; }).join(' / ');
            const contentLines = lines.slice(startLine + 1, endLine);
            const content = contentLines.join('\n').trimEnd() + '\n';

            sections.push({
                level: cur.level,
                title: cur.title,
                content: content,
                path: path
            });
        }

        return sections;
    }

    /**
     * 按行优先策略拆分文本，尝试将文本拆分成长度不超过理想值的段落。
     *
     * @param {string} text 原始文本
     * @param {number} idealLen 理想段落长度
     * @returns {string[]} 拆分后的字符串数组
     */
    function splitByNewlinePrefer(text, idealLen) {
        const result = [];
        let buffer = '';

        const flushBuffer = function () {
            const trimmed = buffer.trimEnd();
            if (trimmed.trim()) result.push(trimmed + '\n');
            buffer = '';
        };

        const lines = text.replace(/\r\n/g, '\n').split('\n');
        for (let index = 0; index < lines.length; index++) {
            const currentLine = lines[index];
            const appendStr = (buffer === '' ? '' : '\n') + currentLine;

            if ((buffer + appendStr).length <= idealLen) {
                buffer += appendStr;
                continue;
            }

            if (buffer.trim().length > 0) flushBuffer();

            if (currentLine.length > idealLen) {
                // 单行超长，硬切
                let offset = 0;
                while (offset < currentLine.length) {
                    const segment = currentLine.slice(offset, offset + idealLen);
                    result.push(segment.trimEnd() + '\n');
                    offset += idealLen;
                }
            } else {
                buffer = currentLine;
            }
        }

        if (buffer.trim().length > 0) flushBuffer();
        return result;
    }

    /**
     * 根据配置将路径字符串格式化为最终输出前缀。
     *
     * @param {string} path 当前段落的层级路径
     * @param {object} option 已合并默认值的拆分选项
     * @returns {string} 格式化后的路径前缀
     */
    function formatPath(path, option) {
        const wholePath = option.pathPrefix + path + '*\n';
        return option.pathOnNewLine ? wholePath : (option.pathPrefix + path + '* ');
    }

    /**
     * 判断文本是否看起来像 Markdown 格式。
     *
     * @param {string} text 原始文本
     * @returns {boolean} 是否看起来像 Markdown 格式
     */
    function looksLikeMarkdown(text) {
        const hasHeading = /(^|\n)#{1,6}\s+\S/.test(text);
        const hasFence = /(^|\n)```/.test(text);
        const hasList = /(^|\n)\s*([-*+]|\d+\.)\s+\S/.test(text);
        const hasQuote = /(^|\n)>\s+\S/.test(text);
        const hasTable = /(^|\n)\s*\|.*\|/.test(text);
        return hasHeading || hasFence || hasList || hasQuote || hasTable;
    }

    // 暴露到全局命名空间
    global.splitTextToStrings = splitTextToStrings;
    global.splitTextToSections = parseMarkdownSections; // 暴露段落解析以便 UI 切片预览使用
})(typeof window !== 'undefined' ? window : this);
