/**
 * 工作流模块
 * 6 步章节生成流水线状态机，含中断恢复机制
 */

(function(global) {
    'use strict';

    var PHASE = global.PHASE;
    var CHAPTER_STATUS = global.CHAPTER_STATUS;

    // ==== 中断错误类 ====
    function InterruptError(step, paragraphIndex, attempt) {
        this.interrupted = true;
        this.step = step;
        this.paragraphIndex = paragraphIndex || 0;
        this.attempt = attempt || 0;
    }
    InterruptError.prototype = Object.create(Error.prototype);
    InterruptError.prototype.constructor = InterruptError;

    // ==== Workflow 类 ====
    class Workflow {
        constructor(app) {
            this.app = app;
            this.pendingStop = false;
        }

        // ==== 启动章节生成（步骤 1-6） ====
        async startChapter() {
            var state = this.app.state;
            this.resetStop();

            // 初始化 currentDraft
            if (!state.currentDraft) {
                state.currentDraft = {
                    chapterIndex: state.chapterIndex,
                    chapterOutline: '',
                    direction: '',
                    paragraphOutlines: [],
                    paragraphs: [],
                    reviewResults: [],
                    wordCountAttempts: 0,
                    formatChanges: 0
                };
            }

            try {
                // 步骤 1: 章节大纲确认
                await this._step1_confirmOutline();
                await this.app.saveState();

                // 步骤 2: 段落大纲规划
                await this._step2_buildParagraphOutlines();
                await this.app.saveState();

                // 步骤 3: 逐段生成
                await this._step3_generateParagraphs();
                await this.app.saveState();

                // 步骤 4: 格式校验
                await this._step4_formatCheck();
                await this.app.saveState();

                // 步骤 5: 字数审核
                await this._step5_wordCountReview();
                await this.app.saveState();

                // 步骤 6: 段落级自动审核
                await this._step6_contentReview();
                await this.app.saveState();

                // 全部完成，进入人工审核阶段
                this.app.setPhase(PHASE.HUMAN_REVIEW);
                this._renderProgress(6, '章节生成完成，等待人工审核');
                this._showHumanApprovalCard();
                await this.app.saveState();

            } catch (e) {
                if (e && e.interrupted) {
                    // 保存中断上下文
                    state.interruptContext = {
                        step: e.step,
                        paragraphIndex: e.paragraphIndex,
                        attempt: e.attempt
                    };
                    this.app.setPhase(PHASE.INTERRUPTED);
                    this._renderProgress(e.step, '已中断 — 可点击「续写」恢复');
                    await this.app.saveState();
                    this.app.showToast('已在步骤 ' + e.step + ' 中断，可恢复');
                } else {
                    // 其他错误
                    this.app.setPhase(PHASE.IDLE);
                    this._renderProgress(0, '生成失败: ' + (e.message || '未知错误'));
                    await this.app.saveState();
                    this.app.showToast('章节生成失败: ' + (e.message || '未知错误'));
                }
            }
        }

        // ==== 从中断点恢复 ====
        async resumeFromInterrupt() {
            var state = this.app.state;
            var ctx = state.interruptContext;
            if (!ctx) {
                // 无中断上下文，从头开始
                await this.startChapter();
                return;
            }

            this.resetStop();
            this.app.showToast('正在从步骤 ' + ctx.step + ' 恢复...');

            try {
                // 根据中断步骤恢复
                if (ctx.step <= 1) {
                    await this._step1_confirmOutline();
                    await this.app.saveState();
                    await this._step2_buildParagraphOutlines();
                    await this.app.saveState();
                    await this._step3_generateParagraphs();
                    await this.app.saveState();
                } else if (ctx.step === 2) {
                    await this._step2_buildParagraphOutlines();
                    await this.app.saveState();
                    await this._step3_generateParagraphs();
                    await this.app.saveState();
                } else if (ctx.step === 3) {
                    // 从段落中断点恢复
                    await this._step3_generateParagraphs(ctx.paragraphIndex);
                    await this.app.saveState();
                } else if (ctx.step === 4) {
                    await this._step4_formatCheck();
                    await this.app.saveState();
                } else if (ctx.step === 5) {
                    await this._step5_wordCountReview();
                    await this.app.saveState();
                } else if (ctx.step === 6) {
                    // 从审核段落中断点恢复
                    await this._step6_contentReview(ctx.paragraphIndex, ctx.attempt);
                    await this.app.saveState();
                }

                // 完成中断步骤后，继续后续步骤
                var currentStep = ctx.step;

                if (currentStep < 4) {
                    await this._step4_formatCheck();
                    await this.app.saveState();
                }
                if (currentStep < 5) {
                    await this._step5_wordCountReview();
                    await this.app.saveState();
                }
                if (currentStep < 6) {
                    await this._step6_contentReview();
                    await this.app.saveState();
                }

                // 清除中断上下文
                state.interruptContext = null;

                // 进入人工审核阶段
                this.app.setPhase(PHASE.HUMAN_REVIEW);
                this._renderProgress(6, '章节生成完成，等待人工审核');
                this._showHumanApprovalCard();
                await this.app.saveState();

            } catch (e) {
                if (e && e.interrupted) {
                    state.interruptContext = {
                        step: e.step,
                        paragraphIndex: e.paragraphIndex,
                        attempt: e.attempt
                    };
                    this.app.setPhase(PHASE.INTERRUPTED);
                    this._renderProgress(e.step, '已中断 — 可点击「续写」恢复');
                    await this.app.saveState();
                    this.app.showToast('再次中断，可恢复');
                } else {
                    this.app.setPhase(PHASE.IDLE);
                    this._renderProgress(0, '恢复失败: ' + (e.message || '未知错误'));
                    await this.app.saveState();
                    this.app.showToast('恢复失败: ' + (e.message || '未知错误'));
                }
            }
        }

        // ==== 步骤10：编辑章节后重新生成摘要 ====
        async step10_editChapter(chapterIndex, newContent) {
            var state = this.app.state;
            var chapter = state.chapters.find(function(c) { return c.index === chapterIndex; });
            if (!chapter) {
                this.app.showToast('未找到章节 ' + chapterIndex);
                return;
            }

            this.app.setPhase(PHASE.GENERATING_SUMMARY);
            this._renderProgress(10, '正在重新生成摘要...');

            try {
                var prevSummary = this._getPrevSummary(chapterIndex);
                var chapterOutline = '';
                if (state.chapterOutlines && state.chapterOutlines[chapterIndex - 1]) {
                    chapterOutline = state.chapterOutlines[chapterIndex - 1].outline || '';
                }

                var prompt = await global.PromptLoader.loadAndFill('generate_summary', {
                    outline: state.outline || '',
                    chapterOutline: chapterOutline,
                    direction: chapter.direction || chapterOutline,
                    chapterContent: newContent,
                    prevSummary: prevSummary || '（无）',
                    chapterIndex: chapterIndex
                });

                var messages = [
                    { role: 'system', content: '你是一位专业的小说编辑助手。请严格按照 JSON 格式输出摘要和下一章建议，不要包含任何其他文字。' },
                    { role: 'user', content: prompt }
                ];

                var result = await this.app.config.callChat(messages, {
                    temperature: 0.5,
                    maxTokens: 4096
                });

                // 记录 token
                this.app.config.trackToken({
                    step: 'generate_summary',
                    model: this.app.config.getConfig().name,
                    inputTokens: result.inputTokens,
                    outputTokens: result.outputTokens,
                    totalTokens: result.totalTokens
                });

                // 解析摘要结果
                var summaryData = this._parseSummaryResult(result.content);
                if (summaryData) {
                    chapter.summary = summaryData.summary || '';
                    // 如果有下一章走向建议，更新状态
                    if (summaryData.nextDirection) {
                        state.nextDirection = summaryData.nextDirection;
                    }
                    if (summaryData.nextCriteria) {
                        state.nextCriteria = summaryData.nextCriteria;
                    }
                    // 更新下一章走向 UI
                    this._renderNextDirection();
                }

                await this.app.saveState();
                this.app.setPhase(PHASE.IDLE);
                this._renderProgress(0, '摘要已重新生成');
                this.app.showToast('摘要已重新生成');

            } catch (e) {
                this.app.setPhase(PHASE.IDLE);
                this.app.showToast('摘要生成失败: ' + (e.message || '未知错误'));
            }
        }

        // ==== 请求中断 ====
        requestStop() {
            this.pendingStop = true;
            this.app.pendingStop = true;
        }

        // ==== 重置中断标志 ====
        resetStop() {
            this.pendingStop = false;
            this.app.pendingStop = false;
        }

        // ==== 步骤 1: 章节大纲确认 ====
        async _step1_confirmOutline() {
            this._checkStop();
            var state = this.app.state;
            var draft = state.currentDraft;

            this.app.setPhase(PHASE.BUILDING);
            this._renderProgress(1, '正在确认章节大纲...');

            // 取 chapterOutlines[chapterIndex] 作为当前章节大纲
            var chapterIndex = state.chapterIndex;
            var chapterOutline = '';

            if (state.chapterOutlines && state.chapterOutlines[chapterIndex]) {
                chapterOutline = state.chapterOutlines[chapterIndex].outline || '';
            }

            // 若无且 chapters 为空，取 state.nextDirection
            if (!chapterOutline && state.chapters.length === 0) {
                chapterOutline = state.nextDirection || '';
            }

            // 若仍无，取 nextDirection
            if (!chapterOutline) {
                chapterOutline = state.nextDirection || '';
            }

            draft.chapterIndex = chapterIndex + 1;
            draft.chapterOutline = chapterOutline;
            draft.direction = chapterOutline;

            // 更新方向 UI
            this.app.elements.directionInput.value = chapterOutline;

            this._renderProgress(1, '章节大纲已确认: 第 ' + draft.chapterIndex + ' 章');
        }

        // ==== 步骤 2: 段落大纲规划 ====
        async _step2_buildParagraphOutlines() {
            this._checkStop();
            var state = this.app.state;
            var draft = state.currentDraft;

            this.app.setPhase(PHASE.BUILDING);
            this._renderProgress(2, '正在规划段落大纲...');

            var prevSummary = this._getPrevSummary();
            var wordTarget = state.config.paragraphTarget || 400;

            var prompt = await global.PromptLoader.loadAndFill('build_paragraph_outlines', {
                outline: state.outline || '',
                chapterOutline: draft.chapterOutline,
                criteria: state.criteria || '',
                prevSummary: prevSummary || '（无）',
                wordTarget: wordTarget
            });

            var messages = [
                { role: 'system', content: '你是一位专业的小说创作顾问。请严格按照 JSON 数组格式返回段落大纲，不要添加任何额外文字。' },
                { role: 'user', content: prompt }
            ];

            var result = await this.app.config.callChat(messages, {
                temperature: 0.7,
                maxTokens: 4096
            });

            // 记录 token
            this.app.config.trackToken({
                step: 'build_paragraph_outlines',
                model: this.app.config.getConfig().name,
                inputTokens: result.inputTokens,
                outputTokens: result.outputTokens,
                totalTokens: result.totalTokens
            });

            // 解析段落大纲
            draft.paragraphOutlines = this._parseParagraphOutlines(result.content);

            this._renderProgress(2, '段落大纲规划完成，共 ' + draft.paragraphOutlines.length + ' 段');
        }

        // ==== 步骤 3: 逐段生成 ====
        async _step3_generateParagraphs(startFromIndex) {
            var state = this.app.state;
            var draft = state.currentDraft;
            var outlines = draft.paragraphOutlines;

            if (!outlines || outlines.length === 0) return;

            this.app.setPhase(PHASE.GENERATING_PARAGRAPHS);

            // 确定起始段落索引
            var startIdx = (startFromIndex !== undefined && startFromIndex > 0) ? startFromIndex : 0;

            // 如果从0开始，清空段落
            if (startIdx === 0) {
                draft.paragraphs = [];
            }

            for (var i = startIdx; i < outlines.length; i++) {
                // 中断检查
                this._checkStop(3, i, 0);

                var outline = outlines[i];
                this._renderProgress(3, '正在生成第 ' + (i + 1) + '/' + outlines.length + ' 段...');

                // 查询记忆库知识点
                var knowledgeContext = '';
                try {
                    var kp = outline.knowledgePoints || [];
                    var queryText = outline.content || '';
                    if (kp.length > 0) {
                        queryText = kp.join('，') + '。' + queryText;
                    }
                    knowledgeContext = await this.app.memory.getKnowledgeContext(queryText, 10);
                } catch (e) {
                    console.warn('知识点查询失败:', e);
                }

                // 获取上一段内容
                var prevParagraph = (i > 0 && draft.paragraphs[i - 1]) ? draft.paragraphs[i - 1] : '';

                var wordTarget = state.config.paragraphTarget || 400;

                var prompt = await global.PromptLoader.loadAndFill('generate_paragraph', {
                    paragraphIndex: i + 1,
                    wordTarget: wordTarget,
                    outline: state.outline || '',
                    chapterOutline: draft.chapterOutline,
                    paragraphOutline: outline.content || '',
                    knowledgeContext: knowledgeContext || '（无）',
                    prevParagraph: prevParagraph || '（无，这是第一段）'
                });

                var messages = [
                    { role: 'system', content: '你是一位才华横溢的小说作家。请直接输出段落正文，不要添加序号、标题或任何额外标记。' },
                    { role: 'user', content: prompt }
                ];

                var result = await this.app.config.callChat(messages, {
                    temperature: 0.8,
                    maxTokens: 4096
                });

                // 记录 token
                this.app.config.trackToken({
                    step: 'generate_paragraph',
                    model: this.app.config.getConfig().name,
                    inputTokens: result.inputTokens,
                    outputTokens: result.outputTokens,
                    totalTokens: result.totalTokens
                });

                var paragraphText = (result.content || '').trim();
                draft.paragraphs[i] = paragraphText;

                // 实时更新草稿面板
                this._renderDraft();
                await this.app.saveState();
            }

            this._renderProgress(3, '所有段落生成完成，共 ' + draft.paragraphs.length + ' 段');
        }

        // ==== 步骤 4: 格式校验 ====
        async _step4_formatCheck() {
            this._checkStop();
            var state = this.app.state;
            var draft = state.currentDraft;

            this._renderProgress(4, '正在执行格式校验...');

            // 合并段落为完整章节
            var chapterContent = draft.paragraphs.join('\n\n');

            // 正则清理
            var cleaned = this._formatCleanRegex(chapterContent);

            // 计算改动比例
            var formatChanges = 0;
            if (chapterContent !== cleaned) {
                var diffLen = Math.abs(chapterContent.length - cleaned.length);
                formatChanges = chapterContent.length > 0 ? diffLen / chapterContent.length : 0;
                draft.paragraphs = cleaned.split(/\n\n+/);
                draft.formatChanges = formatChanges;
            }

            // 若改动 > 5%，触发 AI 兜底校验
            if (formatChanges > 0.05) {
                this._renderProgress(4, '格式改动较大（' + (formatChanges * 100).toFixed(1) + '%），执行 AI 兜底校验...');

                var prompt = await global.PromptLoader.loadAndFill('format_check', {
                    content: cleaned
                });

                var messages = [
                    { role: 'system', content: '你是一位文本清洁编辑。请直接输出清理后的完整章节内容，不要添加任何解释。' },
                    { role: 'user', content: prompt }
                ];

                var result = await this.app.config.callChat(messages, {
                    temperature: 0.2,
                    maxTokens: 8192
                });

                // 记录 token
                this.app.config.trackToken({
                    step: 'format_check',
                    model: this.app.config.getConfig().name,
                    inputTokens: result.inputTokens,
                    outputTokens: result.outputTokens,
                    totalTokens: result.totalTokens
                });

                var aiCleaned = (result.content || '').trim();
                if (aiCleaned) {
                    draft.paragraphs = aiCleaned.split(/\n\n+/);
                }
            }

            this._renderDraft();
            this._renderProgress(4, '格式校验完成');
        }

        // ==== 步骤 5: 字数审核 ====
        async _step5_wordCountReview() {
            this._checkStop();
            var state = this.app.state;
            var draft = state.currentDraft;
            var wordMin = state.config.wordMin || 3000;
            var wordMax = state.config.wordMax || 4000;
            var maxRetries = 3;

            this.app.setPhase(PHASE.WORD_REVIEW);

            for (var attempt = 0; attempt < maxRetries; attempt++) {
                this._checkStop();

                var chapterContent = draft.paragraphs.join('\n\n');
                var wordCount = this.app.countWords(chapterContent);

                this._renderProgress(5, '字数审核: ' + wordCount + ' 字（目标 ' + wordMin + '-' + wordMax + '），第 ' + (attempt + 1) + '/' + maxRetries + ' 轮');

                // 字数达标
                if (wordCount >= wordMin && wordCount <= wordMax) {
                    this._renderProgress(5, '字数审核通过: ' + wordCount + ' 字');
                    draft.wordCountAttempts = attempt;
                    return;
                }

                // 调用 AI 字数审核
                var reviewResult = await this.app.reviewer.checkWordCount(
                    chapterContent, wordCount, wordMin, wordMax
                );

                if (reviewResult.wordStatus === 'ok') {
                    this._renderProgress(5, '字数审核通过（AI 判定）');
                    draft.wordCountAttempts = attempt;
                    return;
                }

                if (reviewResult.wordStatus === 'under') {
                    // 字数不足：插入过渡段
                    var positions = reviewResult.transitionPositions || [];
                    if (positions.length > 0) {
                        // 按倒序插入，避免索引偏移
                        var sortedPositions = positions.slice().sort(function(a, b) { return b - a; });
                        for (var j = 0; j < sortedPositions.length; j++) {
                            var pos = sortedPositions[j];
                            if (pos >= 0 && pos < draft.paragraphs.length - 1) {
                                var prevPara = draft.paragraphs[pos];
                                var nextPara = draft.paragraphs[pos + 1];

                                var transitionResult = await this.app.reviewer.generateTransition(
                                    pos + 1, prevPara, pos + 2, nextPara
                                );

                                if (transitionResult.content) {
                                    draft.paragraphs.splice(pos + 1, 0, transitionResult.content);
                                    // 更新 paragraphOutlines 索引
                                    draft.paragraphOutlines.splice(pos + 1, 0, {
                                        index: pos + 2,
                                        content: '（过渡段）',
                                        knowledgePoints: []
                                    });
                                }
                            }
                        }
                        this._renderDraft();
                    }
                } else if (reviewResult.wordStatus === 'over') {
                    // 字数超标：精简或删除段落
                    var trimParagraphs = reviewResult.trimParagraphs || [];
                    var deleteParagraphs = reviewResult.deleteParagraphs || [];

                    // 先删除（按倒序避免索引偏移）
                    if (deleteParagraphs.length > 0) {
                        var sortedDeletes = deleteParagraphs.slice().sort(function(a, b) { return b - a; });
                        for (var k = 0; k < sortedDeletes.length; k++) {
                            var delIdx = sortedDeletes[k] - 1; // 转为 0-based
                            if (delIdx >= 0 && delIdx < draft.paragraphs.length) {
                                draft.paragraphs.splice(delIdx, 1);
                                draft.paragraphOutlines.splice(delIdx, 1);
                            }
                        }
                    }

                    // 精简（改写指定段落）
                    for (var m = 0; m < trimParagraphs.length; m++) {
                        var trimItem = trimParagraphs[m];
                        var trimIdx = (typeof trimItem === 'object' ? trimItem.index : trimItem) - 1;
                        var direction = typeof trimItem === 'object' ? trimItem.direction : '精简内容';

                        if (trimIdx >= 0 && trimIdx < draft.paragraphs.length) {
                            var rewriteResult = await this.app.reviewer.rewriteParagraph(
                                trimIdx + 1,
                                draft.paragraphs[trimIdx],
                                ['字数超标，需要精简'],
                                [direction],
                                ''
                            );
                            if (rewriteResult.content) {
                                draft.paragraphs[trimIdx] = rewriteResult.content;
                            }
                        }
                    }

                    this._renderDraft();
                }

                await this.app.saveState();
            }

            draft.wordCountAttempts = maxRetries;
            this._renderProgress(5, '字数审核达到重试上限，继续后续步骤');
        }

        // ==== 步骤 6: 段落级自动审核 ====
        async _step6_contentReview(startFromIndex, startAttempt) {
            var state = this.app.state;
            var draft = state.currentDraft;
            var paragraphs = draft.paragraphs;
            var maxRounds = 3;

            this.app.setPhase(PHASE.CONTENT_REVIEW);

            // 初始化审核结果
            if (!draft.reviewResults) {
                draft.reviewResults = [];
            }

            // 确定起始段落索引
            var startIdx = (startFromIndex !== undefined && startFromIndex > 0) ? startFromIndex : 0;

            for (var i = startIdx; i < paragraphs.length; i++) {
                // 中断检查
                this._checkStop(6, i, 0);

                var paragraphContent = paragraphs[i];
                if (!paragraphContent || !paragraphContent.trim()) continue;

                // 获取上下文
                var prevContext = i > 0 ? paragraphs[i - 1] : '';
                var nextContext = i < paragraphs.length - 1 ? paragraphs[i + 1] : '';

                // 初始化该段落的审核结果
                var reviewResult = draft.reviewResults[i] || {
                    index: i + 1,
                    passed: false,
                    issues: [],
                    suggestions: [],
                    attempts: 0
                };

                // 确定起始审核轮次
                var startRound = (i === startIdx && startAttempt !== undefined && startAttempt > 0) ? startAttempt : 0;

                for (var round = startRound; round < maxRounds; round++) {
                    this._checkStop(6, i, round);

                    this._renderProgress(6, '审核第 ' + (i + 1) + '/' + paragraphs.length + ' 段，第 ' + (round + 1) + '/' + maxRounds + ' 轮');

                    // 调用审核
                    var result = await this.app.reviewer.reviewParagraph(
                        i + 1, paragraphContent, prevContext, nextContext
                    );

                    reviewResult.attempts = round + 1;
                    reviewResult.issues = result.issues || [];
                    reviewResult.suggestions = result.suggestions || [];

                    if (result.passed) {
                        reviewResult.passed = true;
                        break;
                    }

                    // 未通过，改写
                    this._renderProgress(6, '第 ' + (i + 1) + ' 段未通过审核，正在改写...');

                    var rewriteResult = await this.app.reviewer.rewriteParagraph(
                        i + 1,
                        paragraphContent,
                        result.issues,
                        result.suggestions,
                        ''
                    );

                    if (rewriteResult.content) {
                        paragraphContent = rewriteResult.content;
                        paragraphs[i] = paragraphContent;
                    }

                    await this.app.saveState();
                }

                // 保存审核结果
                draft.reviewResults[i] = reviewResult;

                // 实时更新草稿
                this._renderDraft();
                await this.app.saveState();
            }

            // 统计审核结果
            var passedCount = draft.reviewResults.filter(function(r) { return r && r.passed; }).length;
            var totalCount = draft.reviewResults.filter(function(r) { return r; }).length;
            this._renderProgress(6, '自动审核完成: ' + passedCount + '/' + totalCount + ' 段通过');
        }

        // ==== 检查是否请求中断 ====
        _checkStop(step, paragraphIndex, attempt) {
            if (this.app.pendingStop || this.pendingStop) {
                throw new InterruptError(step || 0, paragraphIndex || 0, attempt || 0);
            }
        }

        // ==== 渲染草稿面板 ====
        _renderDraft() {
            var draft = this.app.state.currentDraft;
            if (!draft) return;

            var el = this.app.elements;
            var paragraphs = draft.paragraphs || [];
            var outlines = draft.paragraphOutlines || [];

            // 显示草稿面板
            el.draftPanel.style.display = 'flex';
            el.draftMeta.textContent = paragraphs.length + ' / ' + (outlines.length || '—');

            // 渲染段落
            var html = '';
            for (var i = 0; i < paragraphs.length; i++) {
                var p = paragraphs[i] || '';
                var outline = outlines[i] || {};
                var review = (draft.reviewResults && draft.reviewResults[i]) || null;

                var statusClass = '';
                if (review) {
                    statusClass = review.passed ? ' passed' : ' failed';
                }

                html += '<div class="draft-paragraph' + statusClass + '">';
                html += '<div class="draft-paragraph-header">';
                html += '<span class="draft-paragraph-index">第 ' + (i + 1) + ' 段</span>';
                if (outline.content) {
                    html += '<span class="draft-paragraph-outline">' + this._escapeHtml(outline.content.substring(0, 50)) + '</span>';
                }
                if (review && !review.passed) {
                    html += '<span class="draft-paragraph-status draft-status-failed"><i class="fas fa-times-circle"></i> 未通过</span>';
                } else if (review && review.passed) {
                    html += '<span class="draft-paragraph-status draft-status-passed"><i class="fas fa-check-circle"></i> 通过</span>';
                }
                html += '</div>';
                html += '<div class="draft-paragraph-content">' + this._escapeHtml(p) + '</div>';
                html += '</div>';
            }

            el.draftParagraphs.innerHTML = html;
        }

        // ==== 渲染进度信息 ====
        _renderProgress(step, message) {
            var el = this.app.elements;
            var stepNames = {
                0: '就绪',
                1: '步骤1: 章节大纲确认',
                2: '步骤2: 段落大纲规划',
                3: '步骤3: 逐段生成',
                4: '步骤4: 格式校验',
                5: '步骤5: 字数审核',
                6: '步骤6: 段落级自动审核',
                10: '步骤10: 重新生成摘要'
            };

            if (el.progressCurrentStep) {
                el.progressCurrentStep.textContent = stepNames[step] || ('步骤 ' + step);
            }
            if (el.progressMessage) {
                el.progressMessage.textContent = message || '';
            }
        }

        // ==== 获取前几章摘要拼接 ====
        _getPrevSummary(upToChapterIndex) {
            var state = this.app.state;
            var chapters = state.chapters || [];
            var summaries = [];
            var maxIndex = upToChapterIndex !== undefined ? upToChapterIndex : state.chapterIndex;

            for (var i = 0; i < chapters.length && i < maxIndex; i++) {
                var ch = chapters[i];
                if (ch && ch.summary) {
                    summaries.push('第 ' + ch.index + ' 章摘要: ' + ch.summary);
                }
            }

            return summaries.length > 0 ? summaries.join('\n\n') : '';
        }

        // ==== 正则清理格式 ====
        _formatCleanRegex(content) {
            if (!content) return content;

            var cleaned = content;

            // 1. 清理 Markdown 代码块围栏 ```...```
            cleaned = cleaned.replace(/```[\s\S]*?```/g, function(match) {
                // 提取代码块中的实际内容（去掉围栏行）
                var inner = match.replace(/^```\w*\n?/, '').replace(/\n?```$/, '');
                return inner.trim();
            });

            // 2. 清理孤立的代码块开始/结束标记
            cleaned = cleaned.replace(/^```[\w]*\s*$/gm, '');

            // 3. 清理 JSON 残留（单行简单 JSON 对象）
            cleaned = cleaned.replace(/\{[^}]*"[^"]*"\s*:\s*[^}]*\}/g, '');

            // 4. 清理 HTML 标签残留
            cleaned = cleaned.replace(/<\/?[a-zA-Z][^>]*>/g, '');

            // 5. 清理多余的空行（超过 2 个连续空行压缩为 2 个）
            cleaned = cleaned.replace(/\n{3,}/g, '\n\n');

            // 6. 清理不自然的 AI 输出痕迹
            cleaned = cleaned.replace(/^(?:以下是|好的，|好的，以下是|当然，|当然，以下是|下面是)[\s，：:]*/gm, '');

            // 7. 清理行首序号标记（如 "1. " "1、" 等，仅在段落开头）
            cleaned = cleaned.replace(/^[\s]*(?:\d+[\.、\)）]\s*)/gm, '');

            return cleaned.trim();
        }

        // ==== 解析段落大纲 JSON ====
        _parseParagraphOutlines(text) {
            if (!text) return [];

            // 尝试提取 JSON
            var jsonStr = null;
            var codeBlockMatch = text.match(/```(?:json)?\s*\n?([\s\S]*?)\n?```/);
            if (codeBlockMatch) {
                jsonStr = codeBlockMatch[1].trim();
            } else {
                // 尝试匹配方括号数组
                var bracketMatch = text.match(/\[[\s\S]*\]/);
                if (bracketMatch) {
                    jsonStr = bracketMatch[0];
                }
            }

            if (jsonStr) {
                try {
                    var parsed = JSON.parse(jsonStr);
                    if (Array.isArray(parsed)) {
                        return parsed.map(function(item, idx) {
                            if (typeof item === 'string') {
                                return { index: idx + 1, content: item, knowledgePoints: [] };
                            }
                            return {
                                index: item.index || idx + 1,
                                content: item.content || item.outline || item.text || '',
                                knowledgePoints: Array.isArray(item.knowledgePoints) ? item.knowledgePoints : []
                            };
                        }).filter(function(po) { return po.content.trim(); });
                    }
                } catch (e) {
                    // JSON 解析失败，尝试按行分割
                }
            }

            // 按行分割降级处理
            var lines = text.split('\n').filter(function(l) { return l.trim().length > 5; });
            return lines.map(function(line, idx) {
                // 移除序号前缀
                var cleaned = line.replace(/^\s*\d+[\.、\)）]\s*/, '').trim();
                return { index: idx + 1, content: cleaned, knowledgePoints: [] };
            }).filter(function(po) { return po.content.length > 2; });
        }

        // ==== 解析摘要生成结果 ====
        _parseSummaryResult(text) {
            if (!text) return null;

            var jsonStr = null;
            var codeBlockMatch = text.match(/```(?:json)?\s*\n?([\s\S]*?)\n?```/);
            if (codeBlockMatch) {
                jsonStr = codeBlockMatch[1].trim();
            } else {
                var braceMatch = text.match(/\{[\s\S]*\}/);
                if (braceMatch) {
                    jsonStr = braceMatch[0];
                }
            }

            if (jsonStr) {
                try {
                    return JSON.parse(jsonStr);
                } catch (e) {
                    // JSON 解析失败
                }
            }

            return null;
        }

        // ==== 渲染下一章走向卡片 ====
        _renderNextDirection() {
            var state = this.app.state;
            if (!state.nextDirection) return;

            var el = this.app.elements;
            el.nextDirectionPreview.value = state.nextDirection;
            el.nextCriteriaPreview.value = state.nextCriteria || '';
            el.nextDirectionCard.style.display = '';
        }

        // ==== 显示人工审核入口 ====
        _showHumanApprovalCard() {
            var el = this.app.elements;
            el.humanApprovalCard.style.display = '';
        }

        // ==== HTML 转义 ====
        _escapeHtml(text) {
            if (!text) return '';
            var div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
    }

    // ==== 暴露到全局 ====
    global.Workflow = Workflow;
})(typeof window !== 'undefined' ? window : this);
