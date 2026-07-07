/**
 * 工作流模块
 * 实现 10 步章节生成流水线状态机。
 *
 * 设计契约：
 *   - 步骤 1: 大纲处理（覆写/格式化，grilling 问题 8）
 *   - 步骤 2: 段落大纲预构建
 *   - 步骤 3: 段落内容生成（每段查询知识库，grilling 问题 2）
 *   - 步骤 4: 格式校验（正则 + AI 兜底，grilling 问题 9）
 *   - 步骤 5: 章节大纲评审（失败 → 整章重写）
 *   - 步骤 6: 总体大纲评审（失败 → 整章重写）
 *   - 步骤 7: 段落评审 + 重写（60% 阈值，最多 3 次重写，grilling 问题 6/7）
 *   - 步骤 8: 人工审核（阻塞等待用户操作）
 *   - 步骤 9: 摘要+nextDirection+nextCriteria 生成（grilling 问题 10）
 *   - 步骤 10: 后期编辑（仅 approved 章节，触发摘要重生成，grilling 问题 15）
 *
 * 中断恢复（grilling 问题 12）：
 *   - 调用级中断（步骤 3/7 段落之间）
 *   - 步骤级降级（步骤 5/6 失败整章重写时不可中断）
 */

(function (global) {
    'use strict';

    /** 步骤枚举 */
    const STEPS = {
        STEP1_OUTLINE: 1,
        STEP2_PARAGRAPH_OUTLINES: 2,
        STEP3_GENERATE_PARAGRAPHS: 3,
        STEP4_FORMAT_CHECK: 4,
        STEP5_CHAPTER_OUTLINE_REVIEW: 5,
        STEP6_TOTAL_OUTLINE_REVIEW: 6,
        STEP7_PARAGRAPH_REVIEW: 7,
        STEP8_HUMAN_APPROVAL: 8,
        STEP9_GENERATE_SUMMARY: 9,
        STEP10_POST_EDIT: 10
    };

    /** 整章重写最大次数 */
    const MAX_CHAPTER_REWRITE = 3;

    /** 段落目标字数（用于估算段落数量） */
    const PARAGRAPH_TARGET_WORDS = 400;

    /**
     * Workflow 类
     */
    class Workflow {
        /**
         * @param {object} status status.json 引用
         * @param {object} vectorStore VectorStore 实例
         * @param {object} reviewer Reviewer 实例
         * @param {object} callbacks 回调集合
         * @param {function} callbacks.onProgress 进度回调 (step, message)
         * @param {function} callbacks.onTokenUsed token 跟踪回调
         * @param {function} callbacks.onStateChange 状态变更回调
         * @param {function} callbacks.onSaveStatus 保存 status 回调
         * @param {function} callbacks.onHumanApproval 人工审核回调 (chapterData) => Promise<{action, opinion, paragraphOpinions}>
         * @param {function} callbacks.onConfirm 确认弹窗回调 (title, msg) => Promise<bool>
         * @param {function} callbacks.onToast Toast 回调 (message, level)
         * @param {function} callbacks.checkInterrupt 检查中断 () => bool
         * @param {function} callbacks.setInterruptContext 设置中断上下文 (ctx) => void
         */
        constructor(status, vectorStore, reviewer, callbacks) {
            this.status = status;
            this.vectorStore = vectorStore;
            this.reviewer = reviewer;
            this.callbacks = callbacks || {};
            this.pendingStop = false;
        }

        /** 请求中断（用户点击 stopBtn 时调用） */
        requestStop() {
            this.pendingStop = true;
        }

        /** 重置中断标志 */
        resetStop() {
            this.pendingStop = false;
        }

        /** 检查中断：调用级中断点（步骤 3/7 段落之间）调用 */
        checkInterruptCallLevel(context) {
            if (this.pendingStop) {
                this._saveInterruptContext(Object.assign({ canResume: true }, context));
                throw new Error('USER_INTERRUPTED');
            }
        }

        /** 检查中断：步骤级降级点（步骤 5/6 重写时不可中断） */
        checkInterruptStepLevel(context) {
            // 步骤级降级：当前在不可中断的步骤内，忽略 pendingStop
            // 但记录用户已请求中断，等步骤边界再处理
        }

        /** 保存中断上下文 */
        _saveInterruptContext(ctx) {
            this.status.phase = 'interrupted';
            this.status.interruptContext = Object.assign({
                step: 0,
                paragraphIndex: 0,
                attempt: 0,
                retryCount: 0,
                canResume: true
            }, ctx);
            if (this.callbacks.setInterruptContext) {
                this.callbacks.setInterruptContext(this.status.interruptContext);
            }
            if (this.callbacks.onSaveStatus) {
                this.callbacks.onSaveStatus();
            }
        }

        /** 更新进度 */
        _progress(step, message) {
            if (this.callbacks.onProgress) this.callbacks.onProgress(step, message);
        }

        /** token 跟踪 */
        _trackToken(tokenInfo) {
            if (this.callbacks.onTokenUsed) this.callbacks.onTokenUsed(tokenInfo);
        }

        /** 保存状态 */
        _saveState() {
            if (this.callbacks.onSaveStatus) this.callbacks.onSaveStatus();
        }

        /** ==================== 主流程入口 ==================== */

        /**
         * 启动章节生成（从步骤 1 开始）。
         */
        async startChapter() {
            this.resetStop();
            this.status.phase = 'building';
            this.status.currentDraft = this._initDraft();
            this._saveState();

            try {
                await this.runFromStep(STEPS.STEP1_OUTLINE);
            } catch (e) {
                if (e.message === 'USER_INTERRUPTED') {
                    this._progress(0, '已中断，可点击续写故事恢复');
                } else {
                    this.status.phase = 'idle';
                    this._saveState();
                    throw e;
                }
            }
        }

        /**
         * 从中断点恢复。
         */
        async resumeFromInterrupt() {
            const ctx = this.status.interruptContext;
            if (!ctx || !ctx.canResume) {
                // 不可恢复，丢弃 currentDraft
                this.status.currentDraft = null;
                this.status.phase = 'idle';
                this.status.interruptContext = null;
                this._saveState();
                if (this.callbacks.onToast) this.callbacks.onToast('上次中断不可恢复，已回退到本章起点', 'warning');
                return;
            }
            this.resetStop();
            this.status.phase = 'building';
            this._saveState();
            this._progress(ctx.step, '正在恢复到步骤 ' + ctx.step + '...');
            await this.runFromStep(ctx.step, ctx);
        }

        /**
         * 从指定步骤开始执行（中断恢复或正常流程）。
         */
        async runFromStep(startStep, context) {
            const draft = this.status.currentDraft;
            if (!draft) throw new Error('currentDraft 不存在');

            let step = startStep;
            while (step <= STEPS.STEP7_PARAGRAPH_REVIEW) {
                if (this.pendingStop && step !== STEPS.STEP1_OUTLINE) {
                    // 步骤边界检查中断
                    this._saveInterruptContext({ step: step, canResume: true });
                    this._progress(0, '已中断，可点击续写故事恢复');
                    return;
                }

                draft.currentStep = step;
                this._saveState();

                switch (step) {
                    case STEPS.STEP1_OUTLINE:
                        await this.step1_processOutline();
                        step = STEPS.STEP2_PARAGRAPH_OUTLINES;
                        break;
                    case STEPS.STEP2_PARAGRAPH_OUTLINES:
                        await this.step2_buildParagraphOutlines();
                        step = STEPS.STEP3_GENERATE_PARAGRAPHS;
                        break;
                    case STEPS.STEP3_GENERATE_PARAGRAPHS:
                        await this.step3_generateParagraphs(context);
                        step = STEPS.STEP4_FORMAT_CHECK;
                        break;
                    case STEPS.STEP4_FORMAT_CHECK:
                        await this.step4_formatCheck();
                        step = STEPS.STEP5_CHAPTER_OUTLINE_REVIEW;
                        break;
                    case STEPS.STEP5_CHAPTER_OUTLINE_REVIEW:
                        const pass5 = await this.step5_chapterOutlineReview();
                        if (!pass5) {
                            // 整章重写：回到步骤 2 重新生成段落大纲与内容
                            draft.retryCount = (draft.retryCount || 0) + 1;
                            if (draft.retryCount > MAX_CHAPTER_REWRITE) {
                                if (this.callbacks.onToast) this.callbacks.onToast('整章重写已达上限 ' + MAX_CHAPTER_REWRITE + ' 次，进入人工审核', 'warning');
                                await this.step8_humanApproval();
                                return;
                            }
                            this._progress(5, '步骤 5 评审未通过，进入整章重写（第 ' + draft.retryCount + '/' + MAX_CHAPTER_REWRITE + ' 次）');
                            step = STEPS.STEP2_PARAGRAPH_OUTLINES;
                        } else {
                            step = STEPS.STEP6_TOTAL_OUTLINE_REVIEW;
                        }
                        break;
                    case STEPS.STEP6_TOTAL_OUTLINE_REVIEW:
                        const pass6 = await this.step6_totalOutlineReview();
                        if (!pass6) {
                            draft.retryCount = (draft.retryCount || 0) + 1;
                            if (draft.retryCount > MAX_CHAPTER_REWRITE) {
                                if (this.callbacks.onToast) this.callbacks.onToast('整章重写已达上限 ' + MAX_CHAPTER_REWRITE + ' 次，进入人工审核', 'warning');
                                await this.step8_humanApproval();
                                return;
                            }
                            this._progress(6, '步骤 6 评审未通过，进入整章重写（第 ' + draft.retryCount + '/' + MAX_CHAPTER_REWRITE + ' 次）');
                            step = STEPS.STEP2_PARAGRAPH_OUTLINES;
                        } else {
                            step = STEPS.STEP7_PARAGRAPH_REVIEW;
                        }
                        break;
                    case STEPS.STEP7_PARAGRAPH_REVIEW:
                        await this.step7_paragraphReview(context);
                        await this.step8_humanApproval();
                        return;
                }
            }
        }

        /** 初始化草稿对象 */
        _initDraft() {
            return {
                chapterIndex: this.status.chapterIndex,
                startedAt: new Date().toISOString(),
                currentStep: 1,
                direction: this.status.direction,
                paragraphs: [],
                paragraphOutlines: [],
                formatLog: [],
                reviewResults: {
                    chapterOutline: null,
                    totalOutline: null,
                    paragraphs: []
                },
                retryCount: 0,
                tokens: 0,
                tokenBreakdown: {
                    step1: 0, step2: 0, step3_paragraphs: 0, step4_format: 0,
                    step5_review: 0, step6_review: 0, step7_review: 0, step7_rewrite: 0,
                    step9_summary: 0, step10_edit: 0
                },
                callLog: [],
                revisions: []
            };
        }

        /** ==================== 步骤 1: 大纲处理 ==================== */

        async step1_processOutline() {
            this._progress(1, '步骤 1：处理章节大纲...');
            const draft = this.status.currentDraft;
            const config = this.status.config;

            // 检索上一章摘要（步骤 9 产物）
            let prevSummary = '';
            if (this.status.chapters.length > 0) {
                const lastChapter = this.status.chapters[this.status.chapters.length - 1];
                prevSummary = lastChapter.summary || '';
            }

            // 从知识库检索相关知识点（基于当前章节走向）
            let knowledgeContext = '';
            if (!this.status.config.vectorDimensionMismatch) {
                const results = await this.vectorStore.query(
                    'story_points',
                    this.status.direction || '故事背景',
                    5,
                    { step: 1, paragraphIndex: -1 }
                );
                knowledgeContext = results.map(function (r) {
                    return '[相关知识点 ' + (r.item.path || '') + '] ' + r.item.content;
                }).join('\n\n');
            }

            const prompt = await global.PromptLoader.loadAndFill('process_outline', {
                outline: this.status.outline || '',
                direction: this.status.direction || '',
                prevSummary: prevSummary,
                knowledgeContext: knowledgeContext,
                overwriteDirection: config.overwriteDirection ? 'true' : 'false',
                hasPrevSummary: prevSummary ? 'true' : 'false'
            });

            const result = await this.reviewer.callChat(prompt, { step: 1 }, {
                systemMessage: '你是一位资深小说编辑与故事架构师，只输出 JSON。',
                temperature: 0.6
            });

            const parsed = this.reviewer.parseJsonWithFallback(result.content, {
                direction: this.status.direction,
                keyEvents: [],
                characters: [],
                scenes: [],
                emotionalTone: '',
                modified: false
            });

            // 应用处理后的走向
            draft.direction = parsed.data.direction || this.status.direction;
            draft.directionMeta = {
                keyEvents: parsed.data.keyEvents || [],
                characters: parsed.data.characters || [],
                scenes: parsed.data.scenes || [],
                emotionalTone: parsed.data.emotionalTone || '',
                modified: parsed.data.modified || false
            };
            this._saveState();
        }

        /** ==================== 步骤 2: 段落大纲预构建 ==================== */

        async step2_buildParagraphOutlines() {
            this._progress(2, '步骤 2：构建段落大纲...');
            const draft = this.status.currentDraft;
            const config = this.status.config;

            // 估算段落数：字数目标 / 段落目标字数
            const targetParagraphCount = Math.max(3, Math.ceil(config.wordMax / PARAGRAPH_TARGET_WORDS));

            let prevSummary = '';
            if (this.status.chapters.length > 0) {
                prevSummary = this.status.chapters[this.status.chapters.length - 1].summary || '';
            }

            let knowledgeContext = '';
            if (!this.status.config.vectorDimensionMismatch) {
                const results = await this.vectorStore.query(
                    'story_points',
                    draft.direction,
                    5,
                    { step: 2, paragraphIndex: -1 }
                );
                knowledgeContext = results.map(function (r) { return r.item.content; }).join('\n\n');
            }

            const prompt = await global.PromptLoader.loadAndFill('build_paragraph_outlines', {
                outline: this.status.outline || '',
                direction: draft.direction,
                prevSummary: prevSummary,
                knowledgeContext: knowledgeContext,
                targetParagraphCount: targetParagraphCount
            });

            const result = await this.reviewer.callChat(prompt, { step: 2 }, {
                systemMessage: '你是一位资深小说编辑，只输出 JSON。',
                temperature: 0.5
            });

            const parsed = this.reviewer.parseJsonWithFallback(result.content, {
                paragraphOutlines: []
            });

            draft.paragraphOutlines = parsed.data.paragraphOutlines || [];
            this._saveState();
        }

        /** ==================== 步骤 3: 段落内容生成 ==================== */

        async step3_generateParagraphs(context) {
            this._progress(3, '步骤 3：生成段落内容...');
            const draft = this.status.currentDraft;
            const outlines = draft.paragraphOutlines;
            if (!outlines || outlines.length === 0) throw new Error('段落大纲为空');

            // 中断恢复：从指定段落索引继续
            const startIdx = (context && context.step === 3 && context.paragraphIndex) ? context.paragraphIndex : 0;

            // 若从中断恢复，保留已生成的段落
            if (startIdx > 0 && draft.paragraphs && draft.paragraphs.length >= startIdx) {
                draft.paragraphs = draft.paragraphs.slice(0, startIdx);
            } else {
                draft.paragraphs = [];
            }

            for (let i = startIdx; i < outlines.length; i++) {
                this.checkInterruptCallLevel({ step: 3, paragraphIndex: i });

                const outline = outlines[i];
                const prevParagraph = i > 0 ? draft.paragraphs[i - 1].content : '';
                const prevSummary = this.status.chapters.length > 0 ? (this.status.chapters[this.status.chapters.length - 1].summary || '') : '';

                // 查询知识库
                let knowledgeContext = '';
                if (!this.status.config.vectorDimensionMismatch) {
                    const results = await this.vectorStore.query(
                        'story_points',
                        outline.summary,
                        3,
                        { step: 3, paragraphIndex: i }
                    );
                    knowledgeContext = results.map(function (r) { return r.item.content; }).join('\n\n');
                }

                const prompt = await global.PromptLoader.loadAndFill('generate_paragraph', {
                    outline: this.status.outline || '',
                    direction: draft.direction,
                    prevSummary: prevSummary,
                    paragraphOutline: outline.summary,
                    paragraphIndex: i,
                    totalParagraphs: outlines.length,
                    prevParagraph: prevParagraph.slice(-200),
                    knowledgeContext: knowledgeContext,
                    wordTarget: this.status.config.wordMax,
                    wordGenerated: draft.paragraphs.reduce(function (s, p) { return s + p.content.length; }, 0)
                });

                this._progress(3, '步骤 3：生成段落 ' + (i + 1) + '/' + outlines.length + '...');
                const result = await this.reviewer.callChat(prompt, { step: 3, paragraphIndex: i }, {
                    systemMessage: '你是一位专业小说作家，直接输出段落正文，不要 JSON、围栏或解释。',
                    temperature: 0.8,
                    maxTokens: 1500
                });

                let content = result.content.trim();
                // 移除残留围栏
                content = content.replace(/^```[\w]*\n?/, '').replace(/\n?```$/, '').trim();

                draft.paragraphs.push({
                    index: i,
                    content: content,
                    wordCount: content.length,
                    revisions: []
                });
                this._saveState();
            }
        }

        /** ==================== 步骤 4: 格式校验 ==================== */

        async step4_formatCheck() {
            this._progress(4, '步骤 4：格式校验...');
            const draft = this.status.currentDraft;
            const formatters = global.NovelStudioProFormatters;

            // 合并段落为完整章节
            const fullContent = '# 第' + this._chapterTitle(this.status.chapterIndex) + '章\n\n' +
                draft.paragraphs.map(function (p) { return p.content; }).join('\n\n');

            // 正则扫描
            const regexResult = formatters.formatChapterContent(fullContent);
            draft.formatLog = regexResult.formatLog;

            let finalContent = regexResult.cleaned;
            if (regexResult.needsAiReview) {
                this._progress(4, '步骤 4：检测到大量异常，触发 AI 兜底校验...');
                const aiResult = await this.reviewer.formatCheckWithAI(finalContent, { step: 4 });
                if (aiResult.cleanedText) {
                    finalContent = aiResult.cleanedText;
                    draft.formatLog.push('ai_fallback_cleaned: ' + aiResult.removedCount + ' 处');
                }
            }

            // 重新切分段落
            const newParagraphs = formatters.segmentParagraphs(finalContent);
            if (newParagraphs.length > 0) {
                draft.paragraphs = newParagraphs.map(function (p) {
                    return Object.assign(p, { revisions: [] });
                });
            }
            draft.formattedContent = finalContent;
            this._saveState();
        }

        /** ==================== 步骤 5: 章节大纲评审 ==================== */

        async step5_chapterOutlineReview() {
            const draft = this.status.currentDraft;
            const review = await this.reviewer.reviewChapterOutline(
                Object.assign({ direction: draft.direction }, draft.directionMeta),
                draft.paragraphs,
                { step: 5 }
            );
            draft.reviewResults.chapterOutline = review;
            this._saveState();
            return review.pass;
        }

        /** ==================== 步骤 6: 总体大纲评审 ==================== */

        async step6_totalOutlineReview() {
            const draft = this.status.currentDraft;
            const review = await this.reviewer.reviewTotalOutline(
                this.status.outline,
                Object.assign({ direction: draft.direction }, draft.directionMeta),
                draft.paragraphs,
                { step: 6 }
            );
            draft.reviewResults.totalOutline = review;
            this._saveState();
            return review.pass;
        }

        /** ==================== 步骤 7: 段落评审 ==================== */

        async step7_paragraphReview(context) {
            this._progress(7, '步骤 7：段落评审...');
            const draft = this.status.currentDraft;
            const criteria = this.status.criteria || '';
            const paragraphs = draft.paragraphs;

            // 中断恢复：从指定段落开始
            const startIdx = (context && context.step === 7 && context.paragraphIndex) ? context.paragraphIndex : 0;
            if (startIdx > 0 && draft.reviewResults.paragraphs.length >= startIdx) {
                draft.reviewResults.paragraphs = draft.reviewResults.paragraphs.slice(0, startIdx);
            } else {
                draft.reviewResults.paragraphs = [];
            }

            // 阶段 1：批量评审
            for (let i = startIdx; i < paragraphs.length; i++) {
                this.checkInterruptCallLevel({ step: 7, paragraphIndex: i });
                const para = paragraphs[i];
                const prevCtx = i > 0 ? paragraphs[i - 1].content.slice(-100) : '';
                const nextCtx = i < paragraphs.length - 1 ? paragraphs[i + 1].content.slice(0, 100) : '';
                const review = await this.reviewer.reviewParagraph(
                    criteria, para, paragraphs.length, prevCtx, nextCtx, { step: 7 }
                );
                review.needsRewrite = review.score < global.Reviewer.PARAGRAPH_PASS_THRESHOLD;
                draft.reviewResults.paragraphs.push(review);
                this._saveState();
            }

            // 阶段 2：对需要重写的段落执行重写 + 重新评审
            for (let i = 0; i < draft.reviewResults.paragraphs.length; i++) {
                this.checkInterruptCallLevel({ step: 7, paragraphIndex: i, attempt: 1 });
                const review = draft.reviewResults.paragraphs[i];
                if (!review.needsRewrite) continue;

                let attempt = 0;
                let currentContent = paragraphs[i].content;
                let currentReview = review;

                while (attempt < global.Reviewer.PARAGRAPH_MAX_REWRITE && currentReview.score < global.Reviewer.PARAGRAPH_PASS_THRESHOLD) {
                    attempt++;
                    this.checkInterruptCallLevel({ step: 7, paragraphIndex: i, attempt: attempt });

                    const prevParagraph = i > 0 ? paragraphs[i - 1].content : '';
                    const rewritten = await this.reviewer.rewriteParagraph(
                        { index: i, content: currentContent },
                        currentReview,
                        { direction: draft.direction, prevParagraph: prevParagraph, criteria: criteria },
                        attempt,
                        { step: 7, paragraphIndex: i, attempt: attempt }
                    );

                    // 记录改动
                    paragraphs[i].revisions.push({
                        attempt: attempt,
                        oldContent: currentContent,
                        newContent: rewritten,
                        scoreBefore: currentReview.score
                    });
                    currentContent = rewritten;
                    paragraphs[i].content = rewritten;
                    paragraphs[i].wordCount = rewritten.length;

                    // 重新评审
                    const prevCtx = i > 0 ? paragraphs[i - 1].content.slice(-100) : '';
                    const nextCtx = i < paragraphs.length - 1 ? paragraphs[i + 1].content.slice(0, 100) : '';
                    currentReview = await this.reviewer.reviewParagraph(
                        criteria, paragraphs[i], paragraphs.length, prevCtx, nextCtx,
                        { step: 7, paragraphIndex: i, attempt: attempt }
                    );
                    currentReview.needsRewrite = currentReview.score < global.Reviewer.PARAGRAPH_PASS_THRESHOLD;
                    currentReview.attempt = attempt;
                    draft.reviewResults.paragraphs[i] = currentReview;
                    this._saveState();
                }

                if (currentReview.score < global.Reviewer.PARAGRAPH_PASS_THRESHOLD) {
                    currentReview.rewriteExhausted = true;
                    if (this.callbacks.onToast) {
                        this.callbacks.onToast('段落 ' + (i + 1) + ' 重写 3 次仍未达标，将进入人工审核', 'warning');
                    }
                }
            }
        }

        /** ==================== 步骤 8: 人工审核 ==================== */

        async step8_humanApproval() {
            this._progress(8, '步骤 8：等待人工审核...');
            this.status.phase = 'reviewing';
            this._saveState();

            const draft = this.status.currentDraft;
            if (!this.callbacks.onHumanApproval) {
                throw new Error('未配置 onHumanApproval 回调');
            }

            const result = await this.callbacks.onHumanApproval({
                chapterIndex: draft.chapterIndex,
                paragraphs: draft.paragraphs,
                reviewResults: draft.reviewResults,
                formatLog: draft.formatLog,
                revisions: draft.revisions
            });

            if (result.action === 'approve') {
                // 进入步骤 9
                await this.step9_generateSummary();
            } else if (result.action === 'rewrite_all') {
                // 整章重写
                this.status.phase = 'building';
                draft.retryCount = (draft.retryCount || 0) + 1;
                if (result.opinion) {
                    draft.directionMeta = draft.directionMeta || {};
                    draft.directionMeta.userOpinion = result.opinion;
                }
                this._saveState();
                await this.runFromStep(STEPS.STEP2_PARAGRAPH_OUTLINES);
            } else if (result.action === 'edit_paragraphs') {
                // 用户对特定段落提出改进意见
                if (result.paragraphOpinions) {
                    for (const op of result.paragraphOpinions) {
                        const para = draft.paragraphs[op.index];
                        if (!para) continue;
                        const rewritten = await this.reviewer.rewriteParagraph(
                            para,
                            { issues: [{ severity: 'high', location: '整体', issue: op.opinion, suggestion: op.opinion }], summary: '用户意见' },
                            { direction: draft.direction, prevParagraph: op.index > 0 ? draft.paragraphs[op.index - 1].content : '', criteria: this.status.criteria },
                            1,
                            { step: 8, paragraphIndex: op.index }
                        );
                        para.revisions.push({ attempt: 'user', oldContent: para.content, newContent: rewritten });
                        para.content = rewritten;
                        para.wordCount = rewritten.length;
                    }
                    this._saveState();
                }
                // 重新进入人工审核
                await this.step8_humanApproval();
            }
        }

        /** ==================== 步骤 9: 摘要生成 ==================== */

        async step9_generateSummary() {
            this._progress(9, '步骤 9：生成摘要与下一章走向...');
            const draft = this.status.currentDraft;
            const config = this.status.config;

            let prevSummary = '';
            if (this.status.chapters.length > 0) {
                prevSummary = this.status.chapters[this.status.chapters.length - 1].summary || '';
            }

            const fullContent = draft.paragraphs.map(function (p) { return p.content; }).join('\n\n');

            const prompt = await global.PromptLoader.loadAndFill('generate_summary', {
                outline: this.status.outline || '',
                direction: draft.direction,
                chapterContent: fullContent,
                prevSummary: prevSummary,
                chapterIndex: draft.chapterIndex
            });

            const result = await this.reviewer.callChat(prompt, { step: 9 }, {
                systemMessage: '你是一位专业的小说编辑助手，只输出 JSON。',
                temperature: 0.5,
                maxTokens: 2000
            });

            const parsed = this.reviewer.parseJsonWithFallback(result.content, {
                summary: '',
                nextDirection: '',
                nextCriteria: ''
            });

            // 写入当前章节到 chapters 数组
            const chapterTokens = draft.callLog.reduce(function (s, c) { return s + (c.totalTokens || 0); }, 0);
            const approvedChapter = {
                index: draft.chapterIndex,
                title: '第' + this._chapterTitle(draft.chapterIndex) + '章',
                content: fullContent,
                summary: parsed.data.summary || '',
                tokens: chapterTokens,
                tokenBreakdown: draft.tokenBreakdown,
                approvedAt: new Date().toISOString(),
                revisions: []
            };
            this.status.chapters.push(approvedChapter);

            // 写入摘要到向量库 chapter_summaries
            if (parsed.data.summary && !this.status.config.vectorDimensionMismatch) {
                try {
                    const embedResult = await this.vectorStore.embed(parsed.data.summary, { step: 9, paragraphIndex: -1 });
                    if (embedResult.embedding) {
                        await this.vectorStore.addItem('chapter_summaries', {
                            chapterIndex: draft.chapterIndex,
                            summary: parsed.data.summary,
                            embedding: embedResult.embedding,
                            source: 'auto'
                        });
                    }
                } catch (e) {
                    if (this.callbacks.onToast) this.callbacks.onToast('章节摘要写入向量库失败: ' + e.message, 'warning');
                }
            }

            // 暂存下一章走向与评判标准（待用户确认后覆盖）
            this.status.nextDirection = parsed.data.nextDirection || '';
            this.status.nextCriteria = parsed.data.nextCriteria || '';
            this.status.nextDirectionSource = 'ai_suggestion';

            // 压缩 callLog（approved 后）
            draft.callLog = [];

            // 清理 currentDraft
            this.status.currentDraft = null;
            this.status.chapterIndex = draft.chapterIndex + 1;
            this.status.phase = 'idle';
            this.status.interruptContext = null;
            this._saveState();

            this._progress(9, '步骤 9 完成，章节已通过审核');

            // 弹出确认弹窗（由 UI 层处理）
            if (this.callbacks.onNextDirectionReady) {
                this.callbacks.onNextDirectionReady(this.status.nextDirection, this.status.nextCriteria);
            }
        }

        /** ==================== 步骤 10: 后期编辑 ==================== */

        /**
         * 编辑已审核通过的章节并重新生成摘要。
         *
         * @param {number} chapterIndex 章节序号
         * @param {string} newContent 新内容
         */
        async step10_editChapter(chapterIndex, newContent) {
            this._progress(10, '步骤 10：编辑章节并重新生成摘要...');
            const chapter = this.status.chapters.find(function (c) { return c.index === chapterIndex; });
            if (!chapter) throw new Error('章节不存在: ' + chapterIndex);

            const oldSummary = chapter.summary;
            const oldTokens = chapter.tokens || 0;

            // 更新内容
            chapter.content = newContent;

            // 删除旧摘要向量
            if (!this.status.config.vectorDimensionMismatch) {
                await this.vectorStore.deleteByChapterIndex('chapter_summaries', chapterIndex);
            }

            // 重新生成摘要
            let prevSummary = '';
            const idx = this.status.chapters.findIndex(function (c) { return c.index === chapterIndex; });
            if (idx > 0) prevSummary = this.status.chapters[idx - 1].summary || '';

            const prompt = await global.PromptLoader.loadAndFill('generate_summary', {
                outline: this.status.outline || '',
                direction: '（编辑后重新生成摘要，仅生成 summary 字段）',
                chapterContent: newContent,
                prevSummary: prevSummary,
                chapterIndex: chapterIndex
            });

            const result = await this.reviewer.callChat(prompt, { step: 10 }, {
                systemMessage: '你是一位专业的小说编辑助手，只输出 JSON。',
                temperature: 0.5,
                maxTokens: 1500
            });

            const parsed = this.reviewer.parseJsonWithFallback(result.content, {
                summary: '',
                nextDirection: '',
                nextCriteria: ''
            });

            chapter.summary = parsed.data.summary || '';
            chapter.tokens = oldTokens + result.totalTokens;
            chapter.editedAt = new Date().toISOString();

            // 记录改动历史
            chapter.revisions.push({
                at: new Date().toISOString(),
                oldSummary: oldSummary,
                newSummary: chapter.summary,
                oldTokens: oldTokens,
                newTokens: chapter.tokens,
                tokensDelta: result.totalTokens
            });

            // 写入新摘要到向量库
            if (chapter.summary && !this.status.config.vectorDimensionMismatch) {
                try {
                    const embedResult = await this.vectorStore.embed(chapter.summary, { step: 10, paragraphIndex: -1 });
                    if (embedResult.embedding) {
                        await this.vectorStore.addItem('chapter_summaries', {
                            chapterIndex: chapterIndex,
                            summary: chapter.summary,
                            embedding: embedResult.embedding,
                            source: 'edit'
                        });
                    }
                } catch (e) {
                    if (this.callbacks.onToast) this.callbacks.onToast('新摘要写入向量库失败: ' + e.message, 'warning');
                }
            }

            this._saveState();
            this._progress(10, '章节已更新，摘要已重新生成');
        }

        /** ==================== 工具方法 ==================== */

        /**
         * 章节序号转中文标题。
         */
        _chapterTitle(num) {
            const chinese = ['零', '一', '二', '三', '四', '五', '六', '七', '八', '九', '十'];
            if (num <= 10) return chinese[num];
            if (num < 20) return '十' + chinese[num - 10];
            if (num < 100) {
                const tens = Math.floor(num / 10);
                const ones = num % 10;
                return chinese[tens] + '十' + (ones > 0 ? chinese[ones] : '');
            }
            return String(num);
        }

        /**
         * 用户确认采用 AI 建议的下一章走向后调用，覆盖 direction/criteria。
         */
        applyNextDirection(editedDirection, editedCriteria) {
            this.status.direction = editedDirection || this.status.nextDirection;
            this.status.criteria = editedCriteria || this.status.nextCriteria;
            this.status.nextDirection = '';
            this.status.nextCriteria = '';
            this.status.nextDirectionSource = '';
            this._saveState();
        }

        /**
         * 用户拒绝采用 AI 建议的下一章走向。
         */
        rejectNextDirection() {
            this.status.nextDirection = '';
            this.status.nextCriteria = '';
            this.status.nextDirectionSource = '';
            this._saveState();
        }
    }

    // 暴露到全局
    global.Workflow = Workflow;
    global.WORKFLOW_STEPS = STEPS;
})(typeof window !== 'undefined' ? window : this);
