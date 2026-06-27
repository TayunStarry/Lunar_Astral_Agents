/**
 * 评审模块
 * 负责步骤 4（AI 兜底格式校验）、5（章节大纲评审）、6（总体大纲评审）、7（段落评审）的 AI 调用与解析。
 *
 * 设计契约：
 *   - 评审 AI 返回严格 JSON，失败时降级回退（grilling 问题 6）
 *   - 步骤 5/6 失败 → 整章重写；步骤 7 段落 < 60% → 段落重写，最多 3 次
 *   - 聊天模型固定 system-multimodal（grilling 问题 18）
 *   - token 跟踪通过 onTokenUsed 回调上报（grilling 问题 16）
 */

(function (global) {
    'use strict';

    /** 聊天模型名（固定） */
    const CHAT_MODEL = 'system-multimodal';

    /** 段落评审通过阈值（60%） */
    const PARAGRAPH_PASS_THRESHOLD = 60;

    /** 段落重写最大次数 */
    const PARAGRAPH_MAX_REWRITE = 3;

    /**
     * Reviewer 类
     */
    class Reviewer {
        /**
         * @param {object} callbacks 回调集合
         * @param {function} callbacks.onTokenUsed token 跟踪回调
         * @param {function} callbacks.onProgress 进度回调 (message)
         * @param {function} callbacks.checkInterrupt 检查中断标志 () => boolean
         */
        constructor(callbacks) {
            this.callbacks = callbacks || {};
        }

        /** ==================== 通用 AI 调用 ==================== */

        /**
         * 调用 /v1/chat/completions，返回完整响应文本与 token 消耗。
         *
         * @param {string} prompt 完整 prompt 内容（作为 user message）
         * @param {object} meta token 跟踪元信息 {step, paragraphIndex, attempt}
         * @param {object} options 选项 {systemMessage, temperature, maxTokens}
         * @returns {Promise<object>} {content, inputTokens, outputTokens, totalTokens}
         */
        async callChat(prompt, meta, options) {
            const opt = options || {};
            const messages = [];
            if (opt.systemMessage) {
                messages.push({ role: 'system', content: opt.systemMessage });
            }
            messages.push({ role: 'user', content: prompt });

            const response = await fetch('/v1/chat/completions', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    model: CHAT_MODEL,
                    messages: messages,
                    temperature: opt.temperature !== undefined ? opt.temperature : 0.7,
                    max_tokens: opt.maxTokens || 4096,
                    stream: false
                })
            });

            if (!response.ok) {
                const errText = await response.text();
                throw new Error('AI 调用失败: ' + response.status + ' ' + errText);
            }

            const data = await response.json();
            const content = (data.choices && data.choices[0] && data.choices[0].message && data.choices[0].message.content) || '';
            const usage = data.usage || {};
            const inputTokens = usage.prompt_tokens || 0;
            const outputTokens = usage.completion_tokens || 0;
            const totalTokens = usage.total_tokens || (inputTokens + outputTokens);

            // token 跟踪
            if (this.callbacks.onTokenUsed) {
                this.callbacks.onTokenUsed({
                    type: 'chat',
                    model: CHAT_MODEL,
                    inputTokens: inputTokens,
                    outputTokens: outputTokens,
                    totalTokens: totalTokens,
                    paragraphIndex: (meta && meta.paragraphIndex !== undefined) ? meta.paragraphIndex : -1,
                    step: (meta && meta.step) || 0,
                    attempt: (meta && meta.attempt) || 0
                });
            }

            return {
                content: content,
                inputTokens: inputTokens,
                outputTokens: outputTokens,
                totalTokens: totalTokens
            };
        }

        /** ==================== JSON 解析与降级 ==================== */

        /**
         * 解析 AI 返回的 JSON，失败时降级为人工审核标记。
         *
         * @param {string} content AI 响应文本
         * @param {object} fallbackShape 降级时的回退结构
         * @returns {object} {parsed: bool, data: object}
         */
        parseJsonWithFallback(content, fallbackShape) {
            if (!content) {
                return { parsed: false, data: fallbackShape };
            }

            let text = content.trim();

            // 1. 直接尝试解析
            try {
                return { parsed: true, data: JSON.parse(text) };
            } catch (e) { /* 继续尝试 */ }

            // 2. 提取 ```json ... ``` 或 ``` ... ``` 代码块
            const fenceMatch = text.match(/```(?:json)?\s*\n([\s\S]*?)\n```/);
            if (fenceMatch) {
                try {
                    return { parsed: true, data: JSON.parse(fenceMatch[1]) };
                } catch (e) { /* 继续尝试 */ }
            }

            // 3. 提取第一个 { 到最后一个 } 之间的内容
            const start = text.indexOf('{');
            const end = text.lastIndexOf('}');
            if (start !== -1 && end !== -1 && end > start) {
                try {
                    return { parsed: true, data: JSON.parse(text.slice(start, end + 1)) };
                } catch (e) { /* 继续尝试 */ }
            }

            // 4. 提取第一个 [ 到最后一个 ] 之间的内容（数组形式）
            const arrStart = text.indexOf('[');
            const arrEnd = text.lastIndexOf(']');
            if (arrStart !== -1 && arrEnd !== -1 && arrEnd > arrStart) {
                try {
                    return { parsed: true, data: JSON.parse(text.slice(arrStart, arrEnd + 1)) };
                } catch (e) { /* 继续尝试 */ }
            }

            // 5. 全部失败，降级
            const fallback = Object.assign({}, fallbackShape);
            if (!fallback.issues) fallback.issues = [];
            fallback.issues.push({
                severity: 'high',
                location: '整章',
                issue: 'AI 响应格式异常，无法解析为 JSON',
                suggestion: '原始反馈（截断）: ' + text.slice(0, 200)
            });
            fallback.summary = 'AI 响应格式异常，需人工审核';
            fallback._parseFailed = true;
            return { parsed: false, data: fallback };
        }

        /** ==================== 步骤 4: AI 兜底格式校验 ==================== */

        /**
         * 调用 AI 执行格式校验（仅当正则扫描改动比例 > 5% 时触发）。
         *
         * @param {string} content 待校验内容
         * @param {object} meta token 跟踪元信息
         * @returns {Promise<object>} {cleanedText, removedCount, reasons}
         */
        async formatCheckWithAI(content, meta) {
            const prompt = await global.PromptLoader.loadAndFill('format_check', { content: content });
            const result = await this.callChat(prompt, Object.assign({ step: 4 }, meta), {
                systemMessage: '你是一位严格的小说校对编辑，只输出 JSON。',
                temperature: 0.3
            });

            const parsed = this.parseJsonWithFallback(result.content, {
                cleanedText: content,
                removedCount: 0,
                reasons: ['AI 格式校验调用失败，已保留原内容']
            });
            return parsed.data;
        }

        /** ==================== 步骤 5: 章节大纲评审 ==================== */

        /**
         * 步骤 5 - 评估剧情发展是否符合本章大纲要求。
         *
         * @param {object} direction 章节走向 {keyEvents, characters, scenes, emotionalTone, direction}
         * @param {object[]} paragraphs 段落数组 [{index, content}]
         * @param {object} meta token 跟踪元信息
         * @returns {Promise<object>} {stage, pass, score, issues, summary}
         */
        async reviewChapterOutline(direction, paragraphs, meta) {
            const numberedContent = paragraphs.map(function (p) {
                return '[段落' + p.index + '] ' + p.content;
            }).join('\n\n');

            const prompt = await global.PromptLoader.loadAndFill('review_chapter_outline', {
                direction: JSON.stringify(direction, null, 2),
                content: numberedContent
            });

            if (this.callbacks.onProgress) this.callbacks.onProgress('步骤 5：评估章节大纲符合性...');
            const result = await this.callChat(prompt, Object.assign({ step: 5 }, meta), {
                systemMessage: '你是一位严格的小说剧情评审编辑，只输出 JSON。',
                temperature: 0.4
            });

            const parsed = this.parseJsonWithFallback(result.content, {
                stage: 'outline_review',
                pass: false,
                score: 50,
                issues: [],
                summary: 'AI 响应格式异常，需人工审核'
            });

            // 校正 pass 与 score 一致性
            if (parsed.data.score !== undefined) {
                parsed.data.pass = parsed.data.score >= 60;
            }
            if (!parsed.data.stage) parsed.data.stage = 'outline_review';
            return parsed.data;
        }

        /** ==================== 步骤 6: 总体大纲评审 ==================== */

        /**
         * 步骤 6 - 评估当前章节内容发展与走向是否符合总体大纲规范。
         *
         * @param {string} outline 总体大纲
         * @param {object} direction 章节走向
         * @param {object[]} paragraphs 段落数组
         * @param {object} meta token 跟踪元信息
         * @returns {Promise<object>} {stage, pass, score, issues, summary}
         */
        async reviewTotalOutline(outline, direction, paragraphs, meta) {
            const fullContent = paragraphs.map(function (p) { return p.content; }).join('\n\n');

            const prompt = await global.PromptLoader.loadAndFill('review_total_outline', {
                outline: outline,
                direction: JSON.stringify(direction, null, 2),
                content: fullContent
            });

            if (this.callbacks.onProgress) this.callbacks.onProgress('步骤 6：评估总体大纲符合性...');
            const result = await this.callChat(prompt, Object.assign({ step: 6 }, meta), {
                systemMessage: '你是一位资深的小说总编，只输出 JSON。',
                temperature: 0.4
            });

            const parsed = this.parseJsonWithFallback(result.content, {
                stage: 'total_outline_review',
                pass: false,
                score: 50,
                issues: [],
                summary: 'AI 响应格式异常，需人工审核'
            });

            if (parsed.data.score !== undefined) {
                parsed.data.pass = parsed.data.score >= 60;
            }
            if (!parsed.data.stage) parsed.data.stage = 'total_outline_review';
            return parsed.data;
        }

        /** ==================== 步骤 7: 段落评审 ==================== */

        /**
         * 步骤 7 - 对单个段落执行评审，返回评分与问题。
         *
         * @param {string} criteria 评判标准
         * @param {object} paragraph 段落 {index, content}
         * @param {number} totalParagraphs 总段落数
         * @param {string} prevContext 前一段最后 100 字
         * @param {string} nextContext 后一段前 100 字
         * @param {object} meta token 跟踪元信息 {step, attempt}
         * @returns {Promise<object>} {paragraphIndex, score, issues, summary}
         */
        async reviewParagraph(criteria, paragraph, totalParagraphs, prevContext, nextContext, meta) {
            const prompt = await global.PromptLoader.loadAndFill('review_paragraph', {
                criteria: criteria,
                paragraphContent: paragraph.content,
                paragraphIndex: paragraph.index,
                totalParagraphs: totalParagraphs,
                prevContext: prevContext || '（无，本段为开头）',
                nextContext: nextContext || '（无，本段为结尾）'
            });

            if (this.callbacks.onProgress) this.callbacks.onProgress('步骤 7：评审段落 ' + (paragraph.index + 1) + '/' + totalParagraphs + '...');
            const result = await this.callChat(prompt, Object.assign({ step: 7, paragraphIndex: paragraph.index }, meta), {
                systemMessage: '你是一位严格的小说段落评审编辑，只输出 JSON。',
                temperature: 0.4
            });

            const parsed = this.parseJsonWithFallback(result.content, {
                paragraphIndex: paragraph.index,
                score: 50,
                issues: [],
                summary: 'AI 响应格式异常，需人工审核'
            });

            if (!parsed.data.paragraphIndex) parsed.data.paragraphIndex = paragraph.index;
            return parsed.data;
        }

        /**
         * 步骤 7 - 批量评审所有段落，返回每段评分与是否需要重写。
         *
         * @param {string} criteria 评判标准
         * @param {object[]} paragraphs 段落数组
         * @param {object} meta token 跟踪元信息
         * @returns {Promise<object>} {reviews: [{paragraphIndex, score, issues, summary, needsRewrite}], allPassed: bool}
         */
        async reviewAllParagraphs(criteria, paragraphs, meta) {
            const reviews = [];
            for (let i = 0; i < paragraphs.length; i++) {
                if (this.callbacks.checkInterrupt && this.callbacks.checkInterrupt()) {
                    throw new Error('用户中断');
                }
                const para = paragraphs[i];
                const prevCtx = i > 0 ? paragraphs[i - 1].content.slice(-100) : '';
                const nextCtx = i < paragraphs.length - 1 ? paragraphs[i + 1].content.slice(0, 100) : '';
                const review = await this.reviewParagraph(criteria, para, paragraphs.length, prevCtx, nextCtx, meta);
                review.needsRewrite = review.score < PARAGRAPH_PASS_THRESHOLD;
                reviews.push(review);
            }
            const allPassed = reviews.every(function (r) { return !r.needsRewrite; });
            return { reviews: reviews, allPassed: allPassed };
        }

        /** ==================== 步骤 7 重写: 段落重写 ==================== */

        /**
         * 步骤 7 重写 - 基于评审问题重写指定段落。
         *
         * @param {object} paragraph 段落 {index, content}
         * @param {object} review 评审结果 {issues, summary}
         * @param {object} context 上下文 {direction, prevParagraph, criteria}
         * @param {number} attempt 第几次重写（1-3）
         * @param {object} meta token 跟踪元信息
         * @returns {Promise<string>} 重写后的段落内容
         */
        async rewriteParagraph(paragraph, review, context, attempt, meta) {
            const issuesText = review.issues.map(function (iss) {
                return '- [' + iss.severity + '] ' + iss.location + ': ' + iss.issue + ' → 建议: ' + iss.suggestion;
            }).join('\n');

            const prompt = await global.PromptLoader.loadAndFill('rewrite_paragraph', {
                originalParagraph: paragraph.content,
                issues: issuesText,
                reviewSummary: review.summary,
                direction: context.direction || '',
                prevParagraph: (context.prevParagraph || '').slice(-200),
                criteria: context.criteria || '',
                attempt: attempt
            });

            if (this.callbacks.onProgress) this.callbacks.onProgress('步骤 7：重写段落 ' + (paragraph.index + 1) + '（第 ' + attempt + '/' + PARAGRAPH_MAX_REWRITE + ' 次）...');
            const result = await this.callChat(prompt, Object.assign({ step: 7, paragraphIndex: paragraph.index, attempt: attempt }, meta), {
                systemMessage: '你是一位专业小说编辑，根据评审建议改写段落。只输出改写后的段落正文，不要任何 JSON、围栏或解释。',
                temperature: 0.8
            });

            // 重写不要求 JSON，直接取 content
            let rewritten = result.content.trim();
            // 移除可能残留的围栏
            rewritten = rewritten.replace(/^```[\w]*\n?/, '').replace(/\n?```$/, '').trim();
            return rewritten;
        }

        /** ==================== 常量暴露 ==================== */

        static get PARAGRAPH_PASS_THRESHOLD() { return PARAGRAPH_PASS_THRESHOLD; }
        static get PARAGRAPH_MAX_REWRITE() { return PARAGRAPH_MAX_REWRITE; }
        static get CHAT_MODEL() { return CHAT_MODEL; }
    }

    // 暴露到全局
    global.Reviewer = Reviewer;
})(typeof window !== 'undefined' ? window : this);
