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

        // ==== 草稿生成（高温度，关注故事流畅性） ====
        // 返回: { content, inputTokens, outputTokens, totalTokens }
        async draftParagraph(paragraphIndex, paragraphOutline, knowledgeContext, prevParagraph, wordTarget, chapterOutline, outline, dramaticBeat, emotionalTarget, pacing) {
            var prompt = await global.PromptLoader.loadAndFill('draft_paragraph', {
                paragraphIndex: paragraphIndex,
                paragraphOutline: paragraphOutline,
                knowledgeContext: knowledgeContext || '（无）',
                prevParagraph: prevParagraph || '（无，这是第一段）',
                wordTarget: wordTarget,
                chapterOutline: chapterOutline || '',
                outline: outline || '',
                dramaticBeat: dramaticBeat || '',
                emotionalTarget: emotionalTarget || '',
                pacing: pacing || ''
            });

            var messages = [
                { role: 'system', content: '你是一位才华横溢的小说作家。请直接输出段落草稿正文，不要添加序号、标题或任何额外标记。此刻不需要雕琢文字，专注于把故事讲好。' },
                { role: 'user', content: prompt }
            ];

            var result = await this.app.config.callChat(messages, {
                temperature: 0.85,
                maxTokens: 4096
            });

            this.app.config.trackToken({
                step: 'draft_paragraph',
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

        // ==== 段落精炼（中温度，关注文学技巧） ====
        // 返回: { content, inputTokens, outputTokens, totalTokens }
        async polishParagraph(paragraphIndex, draftContent, paragraphOutline, prevParagraph, nextOutline, chapterOutline, criteria) {
            var prompt = await global.PromptLoader.loadAndFill('polish_paragraph', {
                paragraphIndex: paragraphIndex,
                draftContent: draftContent,
                paragraphOutline: paragraphOutline || '',
                prevParagraph: prevParagraph || '（无，这是第一段）',
                nextOutline: nextOutline || '（无，这是最后一段）',
                chapterOutline: chapterOutline || '',
                criteria: criteria || ''
            });

            var messages = [
                { role: 'system', content: '你是一位精益求精的文学编辑。请直接输出精炼后的段落正文，不要添加序号、标题或任何额外标记。' },
                { role: 'user', content: prompt }
            ];

            var result = await this.app.config.callChat(messages, {
                temperature: 0.5,
                maxTokens: 4096
            });

            this.app.config.trackToken({
                step: 'polish_paragraph',
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

        // ==== 整章全局审读 ====
        // 返回: { passed, issues, suggestions, emotionalCurve, pacingAnalysis, reviseParagraphs }
        async reviewChapter(chapterContent, chapterOutline, criteria, outline) {
            var prompt = await global.PromptLoader.loadAndFill('review_chapter', {
                chapterContent: chapterContent,
                chapterOutline: chapterOutline || '',
                criteria: criteria || '',
                outline: outline || ''
            });

            var messages = [
                { role: 'system', content: '你是一位资深的小说主编。请以 JSON 格式返回审读结果，不要添加任何额外文字。' },
                { role: 'user', content: prompt }
            ];

            var result = await this.app.config.callChat(messages, {
                temperature: 0.4,
                maxTokens: 4096
            });

            this.app.config.trackToken({
                step: 'review_chapter',
                model: this.app.config.getConfig().name,
                inputTokens: result.inputTokens,
                outputTokens: result.outputTokens,
                totalTokens: result.totalTokens
            });

            return this._parseChapterReviewResult(result.content);
        }

        // ==== 最终润色（低温度，句子级雕琢） ====
        // 返回: { content, inputTokens, outputTokens, totalTokens }
        async finalPolish(chapterContent, chapterOutline, criteria) {
            var prompt = await global.PromptLoader.loadAndFill('final_polish', {
                chapterContent: chapterContent,
                chapterOutline: chapterOutline || '',
                criteria: criteria || ''
            });

            var messages = [
                { role: 'system', content: '你是一位文字雕琢大师。请直接输出润色后的完整章节内容，不要添加任何解释或标记。' },
                { role: 'user', content: prompt }
            ];

            var result = await this.app.config.callChat(messages, {
                temperature: 0.3,
                maxTokens: 8192
            });

            this.app.config.trackToken({
                step: 'final_polish',
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

        // ==== 解析整章审读结果 ====
        _parseChapterReviewResult(text) {
            var defaults = {
                passed: true,
                issues: [],
                suggestions: [],
                emotionalCurve: '',
                pacingAnalysis: '',
                reviseParagraphs: []
            };

            if (!text) return defaults;

            var jsonStr = this._extractJSON(text);
            if (jsonStr) {
                try {
                    var parsed = JSON.parse(jsonStr);
                    return {
                        passed: !!parsed.passed,
                        issues: Array.isArray(parsed.issues) ? parsed.issues : [],
                        suggestions: Array.isArray(parsed.suggestions) ? parsed.suggestions : [],
                        emotionalCurve: parsed.emotionalCurve || '',
                        pacingAnalysis: parsed.pacingAnalysis || '',
                        reviseParagraphs: Array.isArray(parsed.reviseParagraphs) ? parsed.reviseParagraphs : []
                    };
                } catch (e) {
                    // JSON 解析失败
                }
            }

            return defaults;
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
        // 新格式：expandParagraphs / compressParagraphs（自然扩写/压缩）
        _parseWordCountResult(text) {
            var defaults = {
                wordStatus: 'ok',
                expandParagraphs: [],
                compressParagraphs: []
            };

            if (!text) return defaults;

            var jsonStr = this._extractJSON(text);
            if (jsonStr) {
                try {
                    var parsed = JSON.parse(jsonStr);
                    return {
                        wordStatus: parsed.wordStatus || 'ok',
                        expandParagraphs: Array.isArray(parsed.expandParagraphs) ? parsed.expandParagraphs : [],
                        compressParagraphs: Array.isArray(parsed.compressParagraphs) ? parsed.compressParagraphs : []
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
