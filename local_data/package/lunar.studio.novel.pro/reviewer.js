/**
 * 评审模块
 * 负责段落审核、改写、字数审核、过渡段生成等 AI 调用与解析
 */

(function(global) {
    'use strict';

    // ==== Reviewer 类 ====
    class Reviewer {
        constructor(app) {
            this.app = app;
        }

        // ==== 自动审核单个段落 ====
        // 返回: { passed, issues, suggestions }
        async reviewParagraph(paragraphIndex, paragraphContent, prevContext, nextContext) {
            var state = this.app.state;
            var prompt = await global.PromptLoader.loadAndFill('review_paragraph', {
                criteria: state.criteria || '',
                paragraphIndex: paragraphIndex,
                paragraphContent: paragraphContent,
                prevContext: prevContext || '（无）',
                nextContext: nextContext || '（无）'
            });

            var messages = [
                { role: 'system', content: '你是一位严格的小说审核编辑。请按评判标准审核段落，以 JSON 格式返回审核结果。不要添加任何额外解释。' },
                { role: 'user', content: prompt }
            ];

            var result = await this.app.config.callChat(messages, {
                temperature: 0.3,
                maxTokens: 2048
            });

            // 记录 token
            this.app.config.trackToken({
                step: 'review_paragraph',
                model: this.app.config.getConfig().name,
                inputTokens: result.inputTokens,
                outputTokens: result.outputTokens,
                totalTokens: result.totalTokens
            });

            return this._parseReviewResult(result.content);
        }

        // ==== 改写单个段落 ====
        // 返回: { content, inputTokens, outputTokens, totalTokens }
        async rewriteParagraph(paragraphIndex, originalContent, issues, suggestions, userOpinion) {
            var state = this.app.state;
            var prompt = await global.PromptLoader.loadAndFill('rewrite_paragraph', {
                paragraphIndex: paragraphIndex,
                originalParagraph: originalContent,
                issues: Array.isArray(issues) ? issues.join('\n') : (issues || '无'),
                suggestions: Array.isArray(suggestions) ? suggestions.join('\n') : (suggestions || '无'),
                userOpinion: userOpinion || '无',
                chapterOutline: (state.currentDraft && state.currentDraft.chapterOutline) || ''
            });

            var messages = [
                { role: 'system', content: '你是一位小说作家。请根据审核意见改写段落，直接输出改写后的段落正文，不要添加序号或额外标记。' },
                { role: 'user', content: prompt }
            ];

            var result = await this.app.config.callChat(messages, {
                temperature: 0.7,
                maxTokens: 4096
            });

            // 记录 token
            this.app.config.trackToken({
                step: 'rewrite_paragraph',
                model: this.app.config.getConfig().name,
                inputTokens: result.inputTokens,
                outputTokens: result.outputTokens,
                totalTokens: result.totalTokens
            });

            return {
                content: (result.content || '').trim(),
                inputTokens: result.inputTokens,
                outputTokens: result.outputTokens,
                totalTokens: result.totalTokens
            };
        }

        // ==== 字数审核 ====
        // 返回: { wordStatus, transitionPositions, trimParagraphs, deleteParagraphs }
        async checkWordCount(chapterContent, wordCount, wordMin, wordMax) {
            var prompt = await global.PromptLoader.loadAndFill('check_word_count', {
                wordCount: wordCount,
                wordMin: wordMin,
                wordMax: wordMax,
                chapterContent: chapterContent
            });

            var messages = [
                { role: 'system', content: '你是一位小说编辑。请审核章节字数和段落结构，以 JSON 格式返回审核结果。不要添加任何额外解释。' },
                { role: 'user', content: prompt }
            ];

            var result = await this.app.config.callChat(messages, {
                temperature: 0.3,
                maxTokens: 2048
            });

            // 记录 token
            this.app.config.trackToken({
                step: 'check_word_count',
                model: this.app.config.getConfig().name,
                inputTokens: result.inputTokens,
                outputTokens: result.outputTokens,
                totalTokens: result.totalTokens
            });

            return this._parseWordCountResult(result.content);
        }

        // ==== 生成过渡段 ====
        // 返回: { content, inputTokens, outputTokens, totalTokens }
        async generateTransition(prevIndex, prevParagraph, nextIndex, nextParagraph) {
            var state = this.app.state;
            var prompt = await global.PromptLoader.loadAndFill('generate_transition', {
                chapterOutline: (state.currentDraft && state.currentDraft.chapterOutline) || '',
                prevIndex: prevIndex,
                prevParagraph: prevParagraph,
                nextIndex: nextIndex,
                nextParagraph: nextParagraph
            });

            var messages = [
                { role: 'system', content: '你是一位小说作家。请在两个段落之间生成过渡自然段，直接输出正文，不要添加序号或额外标记。' },
                { role: 'user', content: prompt }
            ];

            var result = await this.app.config.callChat(messages, {
                temperature: 0.7,
                maxTokens: 1024
            });

            // 记录 token
            this.app.config.trackToken({
                step: 'generate_transition',
                model: this.app.config.getConfig().name,
                inputTokens: result.inputTokens,
                outputTokens: result.outputTokens,
                totalTokens: result.totalTokens
            });

            return {
                content: (result.content || '').trim(),
                inputTokens: result.inputTokens,
                outputTokens: result.outputTokens,
                totalTokens: result.totalTokens
            };
        }

        // ==== 解析段落审核结果 ====
        _parseReviewResult(text) {
            if (!text) return { passed: true, issues: [], suggestions: [] };

            // 尝试提取 JSON
            var jsonStr = this._extractJSON(text);
            if (jsonStr) {
                try {
                    var parsed = JSON.parse(jsonStr);
                    return {
                        passed: !!parsed.passed,
                        issues: Array.isArray(parsed.issues) ? parsed.issues : [],
                        suggestions: Array.isArray(parsed.suggestions) ? parsed.suggestions : []
                    };
                } catch (e) {
                    // JSON 解析失败，降级处理
                }
            }

            // 降级：通过关键词判断
            var lower = text.toLowerCase();
            var passed = lower.includes('passed') && !lower.includes('"passed": false') && !lower.includes('"passed":false');
            if (lower.includes('不通过') || lower.includes('未通过') || lower.includes('未达标')) {
                passed = false;
            }
            return { passed: passed, issues: [], suggestions: [] };
        }

        // ==== 解析字数审核结果 ====
        _parseWordCountResult(text) {
            var defaults = {
                wordStatus: 'ok',
                transitionPositions: [],
                trimParagraphs: [],
                deleteParagraphs: []
            };

            if (!text) return defaults;

            var jsonStr = this._extractJSON(text);
            if (jsonStr) {
                try {
                    var parsed = JSON.parse(jsonStr);
                    return {
                        wordStatus: parsed.wordStatus || 'ok',
                        transitionPositions: Array.isArray(parsed.transitionPositions) ? parsed.transitionPositions : [],
                        trimParagraphs: Array.isArray(parsed.trimParagraphs) ? parsed.trimParagraphs : [],
                        deleteParagraphs: Array.isArray(parsed.deleteParagraphs) ? parsed.deleteParagraphs : []
                    };
                } catch (e) {
                    // JSON 解析失败
                }
            }

            return defaults;
        }

        // ==== 从文本中提取 JSON 字符串 ====
        _extractJSON(text) {
            if (!text) return null;

            // 尝试提取 ```json ... ``` 代码块
            var codeBlockMatch = text.match(/```(?:json)?\s*\n?([\s\S]*?)\n?```/);
            if (codeBlockMatch) return codeBlockMatch[1].trim();

            // 尝试匹配花括号对象
            var braceMatch = text.match(/\{[\s\S]*\}/);
            if (braceMatch) return braceMatch[0];

            // 尝试匹配方括号数组
            var bracketMatch = text.match(/\[[\s\S]*\]/);
            if (bracketMatch) return bracketMatch[0];

            return null;
        }
    }

    // ==== 暴露到全局 ====
    global.Reviewer = Reviewer;
})(typeof window !== 'undefined' ? window : this);
