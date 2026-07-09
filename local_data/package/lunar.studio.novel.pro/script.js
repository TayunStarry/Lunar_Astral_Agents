/**
 * 小说工作室 - 核心逻辑
 * AI 辅助小说创作工具，支持大纲构建、章节续写、内容审核与记忆点管理
 */

// ==== 状态定义 ====
const PHASE = {
    IDLE: 'idle',
    BUILDING: 'building',
    WRITING: 'writing',
    WORDS_REVIEW: 'words_review',
    CONTENT_REVIEW: 'content_review',
    INTERRUPTED: 'interrupted'
};

const CHAPTER_STATUS = {
    DRAFT: 'draft',
    APPROVED: 'approved'
};

const STATE_FILE = 'package/lunar.studio.novel/status.json';
const MODEL_NAME = 'system-multimodal';
const MAX_MEMORIES = 50;
const MAX_RETRIES = 3;
const MAX_CONTEXT_TOKENS = 16384;

// ==== 默认状态 ====
function createDefaultState() {
    return {
        version: 2,
        phase: PHASE.IDLE,
        config: {
            wordMin: 2345,
            wordMax: 4096,
            summaryCount: 3,          // 上下文引用前N章摘要的数量
            dynamicDirection: true,
            dynamicCriteria: true,
            model: MODEL_NAME,
            retries: {
                wordAdjust: 2,        // 字数调整重试次数
                contentReview: 3,     // 内容审核重试次数
                buildStory: 1,        // 构建故事重试次数
                memoryReview: 1,      // 记忆点审核重试次数
                summaryGenerate: 1   // 摘要生成重试次数
            }
        },
        outline: '',
        direction: '',
        criteria: '',
        chapterIndex: 0,
        chapters: [],
        memories: [],
        retryCount: 0,
        lastReviewFeedback: '',
        reviewLog: [],  // 审批操作日志
        totalTokens: 0  // 累积 token 消耗
    };
}

// ==== 小说工作室主类 ====
class NovelStudio {
    constructor() {
        this.state = createDefaultState();
        this.elements = {};
        this.pendingStop = false;
        this.currentChapterView = 0;
        this.prompts = {};
    }

    // ==== 初始化 ====
    async init() {
        this.cacheElements();
        this.bindEvents();
        await this.loadPrompts();
        await this.loadState();
        this.renderAll();
    }

    cacheElements() {
        const ids = [
            'buildBtn', 'continueBtn', 'stopBtn', 'exportBtn', 'resetBtn', 'editChapterBtn',
            'chapterCounter', 'phaseIndicator', 'toolbarStatus',
            'seedInput', 'outlineInput', 'directionInput', 'criteriaInput',
            'wordMin', 'wordMax', 'summaryCount',
            'dynamicDirection', 'dynamicCriteria',
            'retryWordAdjust', 'retryContentReview', 'retryBuildStory', 'retryMemoryReview', 'retrySummaryGenerate',
            'prevChapterBtn', 'nextChapterBtn',
            'chapterTitle', 'chapterContent', 'chapterEditArea', 'chapterStatus', 'chapterWords', 'chapterTokens',
            'toast', 'toastMessage',
            'directionModal', 'modalDirection', 'modalCriteria',
            'modalKeepBtn', 'modalUpdateBtn', 'directionModalClose',
            'interruptModal', 'interruptCancelBtn', 'interruptConfirmBtn', 'interruptModalClose',
            'exportModal', 'exportCancelBtn', 'exportConfirmBtn', 'exportModalClose',
            'reviewApprovalModal', 'reviewChapterBadge', 'reviewRetryInfo', 'reviewIssuesList',
            'reviewUserOpinion', 'reviewRewriteAllBtn', 'reviewRewriteSelectedBtn', 'reviewAcceptBtn',
            'memoriesPanel', 'memoriesCounter', 'memoriesList', 'memoryAddBtn',
            'memoryCharInput', 'memorySceneInput', 'memoryPlotInput', 'memoryTimeInput', 'memoryNoteInput'
        ];
        for (const id of ids) {
            this.elements[id] = document.getElementById(id);
        }
    }

    bindEvents() {
        // 工具栏按钮
        this.elements.buildBtn.addEventListener('click', () => this.buildStory());
        this.elements.continueBtn.addEventListener('click', () => this.continueStory());
        this.elements.stopBtn.addEventListener('click', () => this.showInterruptModal());
        this.elements.exportBtn.addEventListener('click', () => this.showExportModal());
        this.elements.resetBtn.addEventListener('click', () => this.resetState());
        this.elements.editChapterBtn.addEventListener('click', () => this.toggleEditChapter());

        // 章节导航
        this.elements.prevChapterBtn.addEventListener('click', () => this.navigateChapter(-1));
        this.elements.nextChapterBtn.addEventListener('click', () => this.navigateChapter(1));

        // 弹窗事件
        this.elements.directionModalClose.addEventListener('click', () => this.closeDirectionModal());
        this.elements.modalKeepBtn.addEventListener('click', () => this.onDirectionKeep());
        this.elements.modalUpdateBtn.addEventListener('click', () => this.onDirectionUpdate());
        this.elements.directionModal.addEventListener('click', (e) => {
            if (e.target === this.elements.directionModal) this.closeDirectionModal();
        });

        this.elements.interruptCancelBtn.addEventListener('click', () => this.closeInterruptModal());
        this.elements.interruptConfirmBtn.addEventListener('click', () => this.onInterruptConfirm());
        this.elements.interruptModalClose.addEventListener('click', () => this.closeInterruptModal());
        this.elements.interruptModal.addEventListener('click', (e) => {
            if (e.target === this.elements.interruptModal) this.closeInterruptModal();
        });

        this.elements.exportCancelBtn.addEventListener('click', () => this.closeExportModal());
        this.elements.exportConfirmBtn.addEventListener('click', () => this.onExportConfirm());
        this.elements.exportModalClose.addEventListener('click', () => this.closeExportModal());
        this.elements.exportModal.addEventListener('click', (e) => {
            if (e.target === this.elements.exportModal) this.closeExportModal();
        });

        // 内容审核审批弹窗
        this.elements.reviewRewriteAllBtn.addEventListener('click', () => this.onReviewRewriteAll());
        this.elements.reviewRewriteSelectedBtn.addEventListener('click', () => this.onReviewRewriteSelected());
        this.elements.reviewAcceptBtn.addEventListener('click', () => this.onReviewAccept());
        this.elements.reviewIssuesList.addEventListener('change', (e) => {
            if (e.target.classList.contains('review-issue-checkbox')) {
                this.updateReviewSelectedState();
            }
        });

        // 知识点面板
        this._addMemoryHandler = () => this.addMemory();
        this.elements.memoryAddBtn.addEventListener('click', this._addMemoryHandler);
    }

    // ==== 加载 Prompt 文件 ====
    async loadPrompts() {
        const promptFiles = [
            'build_story', 'continue_chapter', 'review_words', 'review_content', 'review_memories', 'rewrite_paragraph', 'generate_summary'
        ];
        for (const name of promptFiles) {
            try {
                const resp = await fetch(`/file/read/package/lunar.studio.novel/prompts/${name}.md`);
                if (resp.ok) {
                    this.prompts[name] = await resp.text();
                }
            } catch (e) {
                console.warn(`加载 prompt 失败: ${name}`, e);
            }
        }
    }

    // ==== 状态持久化 ====
    async loadState() {
        try {
            const resp = await fetch(`/file/read/${STATE_FILE}`);
            if (resp.ok) {
                const data = await resp.json();
                // 合并默认值，确保新增字段有默认值
                this.state = { ...createDefaultState(), ...data };
                this.state.config = { ...createDefaultState().config, ...(data.config || {}) };

                // 页面刷新后，运行中的 AI 调用已丢失，将中间阶段重置为 IDLE
                const midProcessPhases = [PHASE.BUILDING, PHASE.WRITING, PHASE.WORDS_REVIEW, PHASE.CONTENT_REVIEW];
                if (midProcessPhases.includes(this.state.phase) || this.state.phase === PHASE.INTERRUPTED) {
                    this.state.phase = PHASE.IDLE;
                    this.showToast('已恢复上次工作状态，点击「续写故事」继续');
                } else {
                    this.showToast('已恢复上次工作状态');
                }
            }
        } catch (e) {
            // 首次使用，无缓存文件
        }
    }

    async saveState() {
        try {
            const jsonStr = JSON.stringify(this.state, null, '\t');
            const blob = new Blob([jsonStr], { type: 'application/json' });
            const resp = await fetch('/file/write', {
                method: 'POST',
                headers: {
                    'X-File-Name': btoa(unescape(encodeURIComponent(STATE_FILE))),
                    'X-Overwrite': 'true'
                },
                body: blob
            });
            if (!resp.ok) {
                console.error('保存状态失败');
            }
        } catch (e) {
            console.error('保存状态异常', e);
            this.showToast('状态保存失败，请检查服务');
        }
    }

    clearState() {
        this.state = createDefaultState();
        this.pendingStop = false;
        this.currentChapterView = 0;
        this.renderAll();
    }

    async resetState() {
        if (!confirm('确定要放弃所有缓存，恢复初始状态吗？\n\n此操作将清除所有章节内容、大纲、记忆点等数据，且不可撤销。')) {
            return;
        }

        // 删除服务端缓存文件
        try {
            await fetch(`/file/delete/${encodeURIComponent(STATE_FILE)}`, { method: 'DELETE' });
        } catch (e) {
            console.warn('删除缓存文件失败', e);
        }

        this.clearState();
        this.showToast('已恢复初始状态');
    }

    // ==== 渲染 ====
    renderAll() {
        this.renderToolbar();
        this.renderPanel();
        this.renderChapterView();
        this.updateChapterNav();
    }

    renderToolbar() {
        const { phase, chapterIndex } = this.state;
        const isBusy = phase !== PHASE.IDLE && phase !== PHASE.INTERRUPTED;

        this.elements.chapterCounter.innerHTML = `<i class="fas fa-book"></i> 章节: <strong>${chapterIndex}</strong>`;
        this.elements.buildBtn.disabled = isBusy;
        this.elements.continueBtn.disabled = isBusy;
        this.elements.stopBtn.disabled = !isBusy || this.pendingStop;

        const phaseInfo = this.getPhaseInfo(phase);
        this.elements.phaseIndicator.innerHTML = `
            <span class="phase-dot ${phase !== PHASE.IDLE ? 'active' : ''} ${phase === PHASE.INTERRUPTED ? 'error' : ''}"></span>
            ${phaseInfo}
        `;
    }

    getPhaseInfo(phase) {
        const map = {
            [PHASE.IDLE]: '就绪',
            [PHASE.BUILDING]: '构建故事中...',
            [PHASE.WRITING]: '续写章节中...',
            [PHASE.WORDS_REVIEW]: '字数审核中...',
            [PHASE.CONTENT_REVIEW]: '内容审核中...',
            [PHASE.INTERRUPTED]: '已中断'
        };
        return map[phase] || phase;
    }

    renderPanel() {
        this.elements.seedInput.value = '';
        this.elements.outlineInput.value = this.state.outline;
        this.elements.directionInput.value = this.state.direction;
        this.elements.criteriaInput.value = this.state.criteria;
        this.elements.wordMin.value = this.state.config.wordMin;
        this.elements.wordMax.value = this.state.config.wordMax;
        this.elements.summaryCount.value = this.state.config.summaryCount;
        this.elements.dynamicDirection.checked = this.state.config.dynamicDirection;
        this.elements.dynamicCriteria.checked = this.state.config.dynamicCriteria;
        this.elements.retryWordAdjust.value = this.state.config.retries.wordAdjust;
        this.elements.retryContentReview.value = this.state.config.retries.contentReview;
        this.elements.retryBuildStory.value = this.state.config.retries.buildStory;
        this.elements.retryMemoryReview.value = this.state.config.retries.memoryReview;
        this.elements.retrySummaryGenerate.value = this.state.config.retries.summaryGenerate;
        this.renderMemoriesPanel();
    }

    renderChapterView() {
        const { chapters, chapterIndex } = this.state;
        const chapter = chapters[this.currentChapterView];

        if (!chapter) {
            this.elements.chapterTitle.textContent = `第 ${chapterIndex} 章`;
            this.elements.chapterContent.innerHTML = `
                <div class="empty-state">
                    <i class="fas fa-feather-alt"></i>
                    <h3>小说工作室</h3>
                    <p>在左侧面板输入故事引子，然后点击「构建故事」开始创作之旅</p>
                </div>`;
            this.elements.chapterStatus.textContent = '';
            this.elements.chapterWords.textContent = '';
            this.elements.chapterTokens.innerHTML = `<span class="token-label">Tokens:</span> <span class="token-current">0</span> / <span class="token-total">${this.state.totalTokens || 0}</span>`;
            return;
        }

        this.elements.chapterTitle.textContent = `第 ${chapter.index} 章`;
        // 防御性处理：如果内容看起来是被错误存储的 JSON，提取真实文本
        const displayContent = this.extractContent(chapter.content);
        this.renderMarkdownContent(displayContent);

        const statusText = chapter.status === CHAPTER_STATUS.APPROVED ? '已审核通过' : '草稿';
        const statusIcon = chapter.status === CHAPTER_STATUS.APPROVED ? 'fa-check-circle' : 'fa-pencil-alt';
        this.elements.chapterStatus.innerHTML = `<i class="fas ${statusIcon}"></i> ${statusText}`;
        this.elements.chapterWords.textContent = `约 ${this.countWords(displayContent)} 字`;
        // Token 统计：当前章节实际消耗 + 累计总消耗
        const currentTokens = chapter.tokens || 0;
        this.elements.chapterTokens.innerHTML = `<span class="token-label">Tokens:</span> <span class="token-current">${currentTokens}</span> / <span class="token-total">${this.state.totalTokens || 0}</span>`;
    }

    async renderMarkdownContent(content) {
        if (typeof marked !== 'undefined') {
            marked.setOptions({ breaks: true, gfm: true });
            const html = await marked.parse(content || '');
            this.elements.chapterContent.innerHTML = html;
            if (typeof hljs !== 'undefined') {
                this.elements.chapterContent.querySelectorAll('pre code').forEach(block => {
                    try { hljs.highlightElement(block); } catch (e) { /* ignore */ }
                });
            }
        } else {
            this.elements.chapterContent.innerHTML = this.escapeHtml(content || '')
                .replace(/\n/g, '<br>');
        }
    }

    updateChapterNav() {
        const { chapters } = this.state;
        this.elements.prevChapterBtn.disabled = this.currentChapterView <= 0;
        this.elements.nextChapterBtn.disabled = this.currentChapterView >= chapters.length - 1;

        // 编辑按钮：仅对已审核通过的章节可用
        const currentChapter = chapters[this.currentChapterView];
        if (this._editingChapter) {
            this.elements.editChapterBtn.disabled = false;
        } else {
            this.elements.editChapterBtn.disabled =
                !currentChapter || currentChapter.status !== CHAPTER_STATUS.APPROVED;
        }
    }

    // ==== 编辑章节 ====
    async toggleEditChapter() {
        const chapters = this.state.chapters;
        const currentChapter = chapters[this.currentChapterView];
        if (!currentChapter || currentChapter.status !== CHAPTER_STATUS.APPROVED) {
            this.showToast('仅可编辑已审核通过的章节');
            return;
        }

        if (this._editingChapter) {
            // 保存编辑
            const newContent = this.elements.chapterEditArea.value;
            const changed = newContent !== currentChapter.content;

            this._editingChapter = false;
            this.elements.chapterEditArea.style.display = 'none';
            this.elements.chapterContent.style.display = '';
            this.elements.editChapterBtn.innerHTML = '<i class="fas fa-edit"></i> 编辑故事';
            this.elements.editChapterBtn.classList.remove('btn-glass-accent');

            if (changed) {
                currentChapter.content = newContent;
                this.renderChapterView();
                this.saveState();
                this.showToast('章节内容已保存');

                // 更新章节摘要
                this.elements.toolbarStatus.textContent = '正在更新章节摘要...';
                await this.generateSummary(currentChapter);
                await this.saveState();
                this.elements.toolbarStatus.textContent = '';
            }
        } else {
            // 进入编辑模式
            this._editingChapter = true;
            this.elements.chapterEditArea.value = currentChapter.content;
            this.elements.chapterContent.style.display = 'none';
            this.elements.chapterEditArea.style.display = '';
            this.elements.editChapterBtn.innerHTML = '<i class="fas fa-save"></i> 保存修改';
            this.elements.editChapterBtn.classList.add('btn-glass-accent');
            this.elements.chapterEditArea.focus();
        }
    }

    navigateChapter(delta) {
        const newIdx = this.currentChapterView + delta;
        if (newIdx < 0 || newIdx >= this.state.chapters.length) return;
        this.currentChapterView = newIdx;
        this.renderChapterView();
        this.updateChapterNav();
    }

    // ==== 从面板收集配置 ====
    collectConfig() {
        this.state.config.wordMin = parseInt(this.elements.wordMin.value) || 3000;
        this.state.config.wordMax = parseInt(this.elements.wordMax.value) || 4000;
        this.state.config.summaryCount = parseInt(this.elements.summaryCount.value) || 3;
        this.state.config.dynamicDirection = this.elements.dynamicDirection.checked;
        this.state.config.dynamicCriteria = this.elements.dynamicCriteria.checked;
        this.state.config.retries.wordAdjust = parseInt(this.elements.retryWordAdjust.value) || 2;
        this.state.config.retries.contentReview = parseInt(this.elements.retryContentReview.value) || 3;
        this.state.config.retries.buildStory = parseInt(this.elements.retryBuildStory.value) || 1;
        this.state.config.retries.memoryReview = parseInt(this.elements.retryMemoryReview.value) || 1;
        this.state.config.retries.summaryGenerate = parseInt(this.elements.retrySummaryGenerate.value) || 1;
        this.state.outline = this.elements.outlineInput.value;
        this.state.direction = this.elements.directionInput.value;
        this.state.criteria = this.elements.criteriaInput.value;
    }

    // ==== 构建故事 ====
    async buildStory() {
        const seed = this.elements.seedInput.value.trim();
        if (!seed) {
            this.showToast('请先输入故事引子');
            return;
        }

        this.collectConfig();

        // 检查是否包含预制章节
        const prebuiltChapters = this.parsePrebuiltChapters(seed);
        if (prebuiltChapters.length > 0) {
            this.state.chapters = prebuiltChapters.map((content, i) => ({
                index: i + 1,
                content,
                status: CHAPTER_STATUS.APPROVED,
                criteriaUsed: '',
                tokens: 0
            }));
            this.state.chapterIndex = prebuiltChapters.length;
            this.currentChapterView = prebuiltChapters.length - 1;
            this.showToast(`已识别并导入 ${prebuiltChapters.length} 个预制章节`);

            // 仍需要 AI 生成大纲、走向和评判标准
            // 取引子中非章节部分作为引子
        }

        this.setPhase(PHASE.BUILDING);
        this.elements.toolbarStatus.textContent = '正在构建故事框架...';

        try {
            const systemPrompt = this.prompts.build_story || '请根据故事引子构建小说大纲、章节走向和评判标准。';
            const userMsg = `故事引子：\n${seed}\n\n请生成小说大纲、当前章节延续走向和评判标准。`;
            const buildStoryRetries = this.state.config.retries.buildStory;
            let result = null;

            for (let attempt = 0; attempt <= buildStoryRetries; attempt++) {
                result = await this.callAI(systemPrompt, userMsg);
                if (result && (result.outline || result.direction || result.criteria)) break;
                if (attempt < buildStoryRetries) {
                    this.elements.toolbarStatus.textContent = `构建故事框架重试 (${attempt + 1}/${buildStoryRetries})...`;
                }
            }

            if (result) {
                // 确保所有字段都是字符串，防止 [object Object]
                this.state.outline = this.ensureString(result.outline);
                this.state.direction = this.ensureString(result.direction);
                this.state.criteria = this.ensureString(result.criteria);
                this.elements.outlineInput.value = this.state.outline;
                this.elements.directionInput.value = this.state.direction;
                this.elements.criteriaInput.value = this.state.criteria;
                this.showToast('故事框架构建完成！');
            }

            this.setPhase(PHASE.IDLE);
            this.elements.toolbarStatus.textContent = '';
            this.renderChapterView();
            await this.saveState();
        } catch (e) {
            console.error('构建故事失败', e);
            this.showToast('构建故事失败: ' + (e.message || '未知错误'));
            this.setPhase(PHASE.IDLE);
            this.elements.toolbarStatus.textContent = '';
        }
    }

    // ==== 续写故事（每次点击编写一章） ====
    async continueStory() {
        this.collectConfig();

        if (this.state.phase === PHASE.INTERRUPTED) {
            // 从中断恢复
            this.state.phase = PHASE.IDLE;
            this.pendingStop = false;
        }

        this.setPhase(PHASE.WRITING);
        this.pendingStop = false;
        this.elements.toolbarStatus.textContent = '';

        try {
            await this.writeOneChapter();
        } catch (e) {
            console.error('续写流程异常', e);
            this.showToast('续写过程中发生错误: ' + (e.message || '未知错误'));
            await this.saveState();
        }

        if (this.pendingStop) {
            this.setPhase(PHASE.INTERRUPTED);
            this.showToast('已中断，状态已保存');
        } else if (this.state.phase !== PHASE.INTERRUPTED) {
            this.setPhase(PHASE.IDLE);
        }

        this.pendingStop = false;
        this.elements.toolbarStatus.textContent = '';
        this.renderAll();
    }

    // ==== 编写单章 ====
    async writeOneChapter() {
        const nextChapterIndex = this.state.chapterIndex + 1;

        // 1. 续写章节
        this.setPhase(PHASE.WRITING);
        this.elements.toolbarStatus.textContent = `正在编写第 ${nextChapterIndex} 章...`;

        // 回填缺失的摘要（如预制章节无摘要）
        await this.backfillSummaries();

        const context = this.buildContext();
        const systemPrompt = this.prompts.continue_chapter || '请根据上下文续写小说下一章。';
        const userMsg = `请续写第 ${nextChapterIndex} 章。\n\n${context}`;
        const result = await this.callAI(systemPrompt, userMsg, nextChapterIndex);

        if (!result || !result.content) {
            throw new Error('AI 未返回章节内容');
        }

        let chapterContent = result.content;

        // 添加或更新草稿章节
        const existingDraft = this.state.chapters.find(
            c => c.index === nextChapterIndex && c.status === CHAPTER_STATUS.DRAFT
        );
        if (existingDraft) {
            existingDraft.content = chapterContent;
        } else {
            this.state.chapters.push({
                index: nextChapterIndex,
                content: chapterContent,
                status: CHAPTER_STATUS.DRAFT,
                criteriaUsed: this.state.criteria,
                tokens: 0
            });
        }

        this.currentChapterView = this.state.chapters.length - 1;
        this.renderChapterView();
        this.updateChapterNav();
        await this.saveState();

        if (this.pendingStop) return;

        // 2. 字数审核（使用配置的重试次数）
        this.setPhase(PHASE.WORDS_REVIEW);
        const wordAdjustRetries = this.state.config.retries.wordAdjust;

        for (let attempt = 0; attempt <= wordAdjustRetries; attempt++) {
            const wordResult = await this.reviewWords(chapterContent);
            if (!wordResult || wordResult.passed !== false || !wordResult.action) break;

            this.elements.toolbarStatus.textContent =
                `字数${wordResult.action === 'expand' ? '不足' : '过多'} (${attempt + 1}/${wordAdjustRetries + 1})，正在调整...`;
            const adjustedContent = await this.adjustWords(chapterContent, wordResult);

            const draft = this.state.chapters.find(
                c => c.index === nextChapterIndex && c.status === CHAPTER_STATUS.DRAFT
            );
            if (draft) draft.content = adjustedContent;
            chapterContent = adjustedContent;
            this.renderChapterView();
            await this.saveState();
        }

        if (this.pendingStop) return;

        // 3. 内容审核
        this.setPhase(PHASE.CONTENT_REVIEW);
        this.elements.toolbarStatus.textContent = `正在审核第 ${nextChapterIndex} 章内容...`;

        const currentDraft = this.state.chapters.find(
            c => c.index === nextChapterIndex && c.status === CHAPTER_STATUS.DRAFT
        );
        const contentToReview = currentDraft ? currentDraft.content : chapterContent;

        await this.doContentReview(nextChapterIndex, contentToReview);

        if (this.pendingStop) return;

        // 4. 审核通过，标记已完成
        const approved = this.state.chapters.find(
            c => c.index === nextChapterIndex && c.status === CHAPTER_STATUS.DRAFT
        );
        if (approved) {
            approved.status = CHAPTER_STATUS.APPROVED;
            this.state.chapterIndex = nextChapterIndex;
            this.state.retryCount = 0;
            this.state.lastReviewFeedback = '';

            // 生成章节摘要
            this.elements.toolbarStatus.textContent = '正在生成摘要/走向/标准中...';
            await this.generateSummary(approved);
            await this.saveState();

            // 更新记忆点
            await this.updateMemories(approved.content);

            // 弹窗询问走向与评判标准
            if (this.state.config.dynamicDirection || this.state.config.dynamicCriteria) {
                if (typeof this.showDirectionModal === 'function') {
                    await this.showDirectionModal();
                }
            }

            this.renderChapterView();
            await this.saveState();
            this.showToast(`第 ${nextChapterIndex} 章已完成！`);
        }
    }

    // ==== 内容审核循环 ====
    async doContentReview(chapterIndex, content) {
        this.state.retryCount = 0;
        this.state.lastReviewFeedback = '';
        const contentReviewRetries = this.state.config.retries.contentReview;

        while (this.state.retryCount < contentReviewRetries) {
            if (this.pendingStop) return;

            const reviewResult = await this.reviewContent(content, chapterIndex);
            // 解析失败或无 passed 字段，视为本次审核失败，重试而非直接跳过
            if (!reviewResult || reviewResult.passed === undefined) {
                this.state.retryCount++;
                console.warn(`内容审核 AI 返回格式异常 (${this.state.retryCount}/${contentReviewRetries})，重试中...`);
                this.elements.toolbarStatus.textContent =
                    `内容审核格式异常 (${this.state.retryCount}/${contentReviewRetries})，重试中...`;
                continue;
            }
            if (reviewResult.passed === true) {
                return; // 审核通过
            }

            this.state.retryCount++;
            const issues = reviewResult.issues || [];
            if (issues.length === 0) {
                // 标记为不通过但没有指出具体问题，跳过
                console.warn('内容审核未通过但无具体问题，跳过');
                return;
            }
            this.state.lastReviewFeedback = JSON.stringify(issues);
            this.elements.toolbarStatus.textContent =
                `内容审核不通过 (${this.state.retryCount}/${contentReviewRetries})，正在改写 ${issues.length} 个段落...`;

            // 按段落逐一改写，而非全文重写
            content = await this.applyParagraphEdits(content, issues, '', chapterIndex);
            const draft = this.state.chapters.find(
                c => c.index === chapterIndex && c.status === CHAPTER_STATUS.DRAFT
            );
            if (draft) draft.content = content;
            this.renderChapterView();
            await this.saveState();
        }

        // 超过最大重试次数 — 弹出用户审批模态框，而非直接抛异常
        const userDecision = await this.showReviewApprovalModal(chapterIndex, content);
        const userOpinion = this._lastUserOpinion || '';  // 用户输入的意见
        if (userDecision === 'accept') {
            // 用户认可当前内容
            return;
        }
        if (userDecision === 'rewrite_all') {
            // 用户要求重写全章
            const rewriteResult = await this.callAI(
                this.prompts.continue_chapter || '请根据上下文续写小说下一章。',
                `请重写第 ${chapterIndex} 章，确保内容符合以下评判标准：\n${this.state.criteria}\n\n用户意见：\n${userOpinion}\n\n当前内容：\n${content}\n\n请输出重写后的完整章节。`,
                chapterIndex
            );
            if (rewriteResult && rewriteResult.content) {
                const draft = this.state.chapters.find(
                    c => c.index === chapterIndex && c.status === CHAPTER_STATUS.DRAFT
                );
                if (draft) {
                    draft.content = rewriteResult.content;
                    this.renderChapterView();
                    await this.saveState();
                }
            }
            // 重写后再次提交内容审核
            const newDraft = this.state.chapters.find(
                c => c.index === chapterIndex && c.status === CHAPTER_STATUS.DRAFT
            );
            if (newDraft) {
                await this.doContentReview(chapterIndex, newDraft.content);
            }
            return;
        }
        if (userDecision === 'rewrite_selected') {
            // 用户选择了部分 issue 重审，附带用户意见
            const selectedIssues = this._reviewSelectedIssues || [];
            if (selectedIssues.length > 0) {
                content = await this.applyParagraphEdits(content, selectedIssues, userOpinion, chapterIndex);
                const draft = this.state.chapters.find(
                    c => c.index === chapterIndex && c.status === CHAPTER_STATUS.DRAFT
                );
                if (draft) {
                    draft.content = content;
                    this.renderChapterView();
                    await this.saveState();
                }
                // 再次提交审核
                await this.doContentReview(chapterIndex, content);
            }
            return;
        }
        // 用户关闭了弹窗（未做选择），中断
        this.setPhase(PHASE.INTERRUPTED);
        this.showToast(`第 ${chapterIndex} 章审核 ${contentReviewRetries} 次未通过，等待用户决策。`);
        throw new Error('用户未做出审核决策');
    }

    // ==== 段落级编辑：按审核意见逐一改写段落 ====
    async applyParagraphEdits(content, issues, userOpinion = '', chapterIndex = null) {
        const paragraphs = this.splitParagraphs(content);
        let modified = false;

        for (const issue of issues) {
            if (this.pendingStop) break;

            const idx = issue.paragraph_index;
            if (idx < 0 || idx >= paragraphs.length) {
                console.warn(`段落索引 ${idx} 超出范围 (共 ${paragraphs.length} 段)，跳过`);
                continue;
            }

            const originalPara = paragraphs[idx];
            const suggestion = issue.suggestion || issue.reason || '';

            try {
                const systemPrompt = this.prompts.rewrite_paragraph ||
                    '请根据建议改写指定的段落，只输出改写后的段落内容。';
                let userMsg = `原段落：\n${originalPara}\n\n改写建议：\n${suggestion}`;
                if (userOpinion) {
                    userMsg += `\n\n用户补充意见：\n${userOpinion}`;
                }
                userMsg += `\n\n请输出改写后的段落。`;
                const result = await this.callAI(systemPrompt, userMsg, chapterIndex);

                if (result && result.paragraph) {
                    paragraphs[idx] = result.paragraph;
                    modified = true;
                } else if (result && result.content) {
                    // 兜底：AI 可能返回 content 格式
                    paragraphs[idx] = result.content;
                    modified = true;
                }
            } catch (e) {
                console.warn(`改写段落 ${idx} 失败`, e);
            }
        }

        return modified ? this.rebuildChapter(paragraphs) : content;
    }

    /**
     * 将章节内容按段落分割（以双换行分隔）
     * 返回段落数组，保留空段落以维持索引准确
     */
    splitParagraphs(text) {
        if (!text) return [];
        return text.split(/\n\n+/);
    }

    /**
     * 将段落数组重新组装为章节内容
     */
    rebuildChapter(paragraphs) {
        return paragraphs.join('\n\n');
    }

    /**
     * 将章节内容转为带编号的格式，供审核 AI 使用
     */
    formatParagraphsForReview(text) {
        const paragraphs = this.splitParagraphs(text);
        return paragraphs.map((p, i) => `[段落${i}] ${p}`).join('\n\n');
    }

    // ==== 构建上下文 ====
    buildContext() {
        const { chapters, direction, memories, config } = this.state;
        const summaryCount = config.summaryCount || 3;
        let context = '';

        // 当前章节延续走向
        context += `【当前章节延续走向】\n${direction || '请根据前文自然发展'}\n\n`;

        // 使用前 N 章摘要（而非原文）
        const approvedChapters = chapters.filter(c => c.status === CHAPTER_STATUS.APPROVED);
        if (approvedChapters.length > 0) {
            const recentSummaries = approvedChapters.slice(-summaryCount);
            context += `【前文章节摘要】\n`;
            recentSummaries.forEach(ch => {
                const summary = ch.summary || '（摘要未生成）';
                context += `第 ${ch.index} 章摘要：${summary}\n\n`;
            });
        }

        // 记忆点
        if (memories.length > 0) {
            context += `【重要记忆点】\n`;
            memories.forEach((m, i) => {
                context += `${i + 1}. 人物:${m.character || ''} 场景:${m.scene || ''} 情节:${m.plot || ''} 时间:${m.time || ''} 备注:${m.note || ''}\n`;
            });
            context += '\n';
        }

        // 上下文裁剪
        return this.truncateContext(context);
    }

    truncateContext(context) {
        const estimatedTokens = context.length / 1.5;
        if (estimatedTokens <= MAX_CONTEXT_TOKENS) return context;

        // 摘要模式下上下文已经很紧凑，如果仍超限，裁剪记忆点
        const lines = context.split('\n');
        const memStart = lines.findIndex(l => l.startsWith('【重要记忆点】'));
        if (memStart >= 0) {
            // 保留前 30 条记忆点
            const before = lines.slice(0, memStart + 1);
            const memLines = lines.slice(memStart + 1);
            const keptMems = memLines.slice(0, Math.min(30, memLines.length));
            return before.join('\n') + '\n' + keptMems.join('\n');
        }

        return context;
    }

    // ==== 摘要生成 ====
    async generateSummary(chapter) {
        const systemPrompt = this.prompts.generate_summary ||
            '请为章节生成不超过1000字的结构化摘要。';
        const userMsg = `请为以下章节内容生成摘要：\n\n${chapter.content}`;
        const maxRetries = this.state.config.retries.summaryGenerate;

        for (let attempt = 0; attempt <= maxRetries; attempt++) {
            try {
                const result = await this.callAI(systemPrompt, userMsg, chapter.index);
                if (result && result.summary) {
                    chapter.summary = result.summary;
                    return;
                } else if (result && result.content) {
                    chapter.summary = result.content;
                    return;
                }
                if (attempt < maxRetries) {
                    console.warn(`摘要生成失败 (${attempt + 1}/${maxRetries + 1})，重试中...`);
                }
            } catch (e) {
                if (attempt < maxRetries) {
                    console.warn(`摘要生成异常 (${attempt + 1}/${maxRetries + 1})，重试中...`, e);
                }
            }
        }
        chapter.summary = '（摘要生成失败）';
    }

    /**
     * 为尚未生成摘要的前置章节回填摘要
     * 在续写章节前调用，确保上下文中有足够的摘要
     */
    async backfillSummaries() {
        const { chapters, config } = this.state;
        const summaryCount = config.summaryCount || 3;
        const approvedChapters = chapters.filter(c => c.status === CHAPTER_STATUS.APPROVED);

        // 查找需要回填的章节（最近 summaryCount 个已通过章节中缺少摘要的）
        const recentChapters = approvedChapters.slice(-summaryCount);
        const missing = recentChapters.filter(c => !c.summary);

        if (missing.length > 0) {
            this.elements.toolbarStatus.textContent = `正在补全 ${missing.length} 个章节摘要...`;
            for (const ch of missing) {
                await this.generateSummary(ch);
                await this.saveState();
            }
        }
    }

    // ==== 知识点面板渲染 ====
    renderMemoriesPanel() {
        const { memories } = this.state;
        const count = memories.length;

        // 计数器
        this.elements.memoriesCounter.textContent = `知识点: ${count}/${MAX_MEMORIES}`;
        this.elements.memoryAddBtn.disabled = count >= MAX_MEMORIES;

        // 列表
        if (count === 0) {
            this.elements.memoriesList.innerHTML = '<div class="memories-empty">暂无知识点</div>';
            return;
        }

        this.elements.memoriesList.innerHTML = memories.map((m, i) => {
            const char = this.escapeHtml(m.character || '');
            const scene = this.escapeHtml(m.scene || '');
            const plot = this.escapeHtml(m.plot || '');
            const time = this.escapeHtml(m.time || '');
            const note = this.escapeHtml(m.note || '');
            return `
            <div class="memory-item" data-index="${i}">
                <div class="memory-item-body">
                    <div class="memory-field"><span class="memory-label">人物</span>${char || '—'}</div>
                    <div class="memory-field"><span class="memory-label">场景</span>${scene || '—'}</div>
                    <div class="memory-field"><span class="memory-label">情节</span>${plot || '—'}</div>
                    <div class="memory-field"><span class="memory-label">时间</span>${time || '—'}</div>
                    <div class="memory-field"><span class="memory-label">备注</span>${note || '—'}</div>
                </div>
                <div class="memory-item-actions">
                    <button class="memory-edit-btn" data-index="${i}" title="编辑"><i class="fas fa-edit"></i></button>
                    <button class="memory-delete-btn" data-index="${i}" title="删除"><i class="fas fa-trash"></i></button>
                </div>
            </div>`;
        }).join('');

        // 绑定编辑/删除事件
        this.elements.memoriesList.querySelectorAll('.memory-edit-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const idx = parseInt(btn.dataset.index);
                this.editMemory(idx);
            });
        });
        this.elements.memoriesList.querySelectorAll('.memory-delete-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const idx = parseInt(btn.dataset.index);
                this.deleteMemory(idx);
            });
        });
    }

    addMemory() {
        if (this.state.memories.length >= MAX_MEMORIES) {
            this.showToast(`知识点已达上限 ${MAX_MEMORIES} 个`);
            return;
        }
        const char = this.elements.memoryCharInput.value.trim();
        const scene = this.elements.memorySceneInput.value.trim();
        const plot = this.elements.memoryPlotInput.value.trim();
        const time = this.elements.memoryTimeInput.value.trim();
        const note = this.elements.memoryNoteInput.value.trim();

        if (!char && !scene && !plot && !time && !note) {
            this.showToast('请至少填写一个字段');
            return;
        }

        this.state.memories.push({ character: char, scene, plot, time, note });
        this.elements.memoryCharInput.value = '';
        this.elements.memorySceneInput.value = '';
        this.elements.memoryPlotInput.value = '';
        this.elements.memoryTimeInput.value = '';
        this.elements.memoryNoteInput.value = '';
        this.renderMemoriesPanel();
        this.saveState();
        this.showToast('知识点已添加');
    }

    editMemory(idx) {
        const m = this.state.memories[idx];
        if (!m) return;

        this.elements.memoryCharInput.value = m.character || '';
        this.elements.memorySceneInput.value = m.scene || '';
        this.elements.memoryPlotInput.value = m.plot || '';
        this.elements.memoryTimeInput.value = m.time || '';
        this.elements.memoryNoteInput.value = m.note || '';

        // 标记为编辑模式，再次点击时执行更新
        const addBtn = this.elements.memoryAddBtn;
        addBtn.innerHTML = '<i class="fas fa-save"></i> 更新';
        addBtn.classList.add('editing');
        addBtn.dataset.editIndex = idx;
        addBtn.removeEventListener('click', this._addMemoryHandler);
        this._editMemoryHandler = () => this.updateMemory(idx);
        addBtn.addEventListener('click', this._editMemoryHandler);
    }

    updateMemory(idx) {
        const m = this.state.memories[idx];
        if (!m) return;

        m.character = this.elements.memoryCharInput.value.trim();
        m.scene = this.elements.memorySceneInput.value.trim();
        m.plot = this.elements.memoryPlotInput.value.trim();
        m.time = this.elements.memoryTimeInput.value.trim();
        m.note = this.elements.memoryNoteInput.value.trim();

        this.elements.memoryCharInput.value = '';
        this.elements.memorySceneInput.value = '';
        this.elements.memoryPlotInput.value = '';
        this.elements.memoryTimeInput.value = '';
        this.elements.memoryNoteInput.value = '';

        // 恢复按钮状态
        const addBtn = this.elements.memoryAddBtn;
        addBtn.innerHTML = '<i class="fas fa-plus"></i> 新增';
        addBtn.classList.remove('editing');
        delete addBtn.dataset.editIndex;
        addBtn.removeEventListener('click', this._editMemoryHandler);
        this._addMemoryHandler = () => this.addMemory();
        addBtn.addEventListener('click', this._addMemoryHandler);

        this.renderMemoriesPanel();
        this.saveState();
        this.showToast('知识点已更新');
    }

    deleteMemory(idx) {
        if (!confirm('确定要删除该知识点吗？')) return;
        this.state.memories.splice(idx, 1);
        this.renderMemoriesPanel();
        this.saveState();
        this.showToast('知识点已删除');
    }

    // ==== AI 调用 ====
    async callAI(systemPrompt, userMessage, chapterIndex = null) {
        const messages = [
            { role: 'system', content: systemPrompt },
            { role: 'user', content: userMessage }
        ];

        let rawContent = '';

        try {
            const resp = await fetch('/v1/chat/completions', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    model: MODEL_NAME,
                    messages
                })
            });

            if (!resp.ok) {
                const errText = await resp.text();
                throw new Error(`AI 请求失败 (${resp.status}): ${errText}`);
            }

            const data = await resp.json();
            rawContent = data.choices?.[0]?.message?.content || '';

            // 累积 token 消耗（全局 + 按章节）
            if (data.usage?.total_tokens) {
                this.state.totalTokens = (this.state.totalTokens || 0) + data.usage.total_tokens;
                if (chapterIndex !== null && chapterIndex !== undefined) {
                    const chapter = this.state.chapters.find(c => c.index === chapterIndex);
                    if (chapter) {
                        chapter.tokens = (chapter.tokens || 0) + data.usage.total_tokens;
                    }
                }
            }

            if (!rawContent) {
                throw new Error('AI 返回空内容');
            }

            // 尝试解析 JSON
            const parsed = this.parseJSONFromResponse(rawContent);
            if (parsed) {
                // 如果解析成功但有 content 字段，确保 content 是纯文本
                if (parsed.content !== undefined) {
                    parsed.content = this.extractContent(parsed.content);
                }
                return parsed;
            }

            // JSON 解析失败，尝试提取纯文本内容作为兜底
            console.warn('AI 返回非 JSON 格式，使用原始文本作为内容');
            return { content: this.extractContent(rawContent) };
        } catch (e) {
            // 如果已经拿到原始文本，用兜底策略返回
            if (rawContent && e.message !== 'AI 返回空内容') {
                console.warn('AI 调用异常但已获取文本，使用原始文本作为内容', e.message);
                return { content: this.extractContent(rawContent) };
            }
            console.error('AI 调用异常', e);
            throw e;
        }
    }

    /**
     * 从 AI 原始响应中提取纯文本内容
     * 处理 AI 可能返回 JSON 包裹的文本、markdown 代码块等情况
     */
    extractContent(text) {
        if (!text) return '';
        if (typeof text !== 'string') return String(text);

        const trimmed = text.trim();

        // 策略 1: 严格 JSON 解析
        if (trimmed.startsWith('{') && trimmed.endsWith('}')) {
            try {
                const obj = JSON.parse(trimmed);
                if (obj.content && typeof obj.content === 'string') {
                    return this.cleanNovelContent(obj.content);
                }
            } catch (e) { /* 不是有效 JSON，继续 */ }
        }

        // 策略 2: 提取 markdown 代码块中的 JSON
        const jsonMatch = text.match(/```json\s*([\s\S]*?)```/);
        if (jsonMatch) {
            try {
                const obj = JSON.parse(jsonMatch[1].trim());
                if (obj.content && typeof obj.content === 'string') {
                    return this.cleanNovelContent(obj.content);
                }
            } catch (e) { /* 继续 */ }
        }

        // 策略 3: 正则兜底 — 从形如 {"content": "..."} 的文本中提取 content 字段
        // 处理 AI 返回的 JSON 中换行未正确转义、引号未转义等格式问题
        if (trimmed.includes('"content"')) {
            const extracted = this.extractContentFieldRegex(trimmed);
            if (extracted) return this.cleanNovelContent(extracted);
        }

        // 兜底：返回原始文本
        return this.cleanNovelContent(text);
    }

    /**
     * 清理小说正文中的 JSON 对象和代码块残留
     * 确保生成的小说正文为纯文本，不含任何结构化标记
     */
    cleanNovelContent(text) {
        if (!text || typeof text !== 'string') return text || '';

        let result = text;

        // 1. 提取 {paragraph: "..."} 中的文本内容，替换整个对象
        result = result.replace(/\{[^}]*"paragraph"\s*:\s*"((?:[^"\\]|\\.)*)"\s*[^}]*\}/g, (_, p1) => {
            return p1.replace(/\\n/g, '\n').replace(/\\t/g, '\t').replace(/\\"/g, '"').replace(/\\\\/g, '\\');
        });

        // 2. 移除 markdown 代码块（``` ... ```），提取内部纯文本
        result = result.replace(/```[\w]*\s*([\s\S]*?)```/g, (_, inner) => {
            // 如果内部是 JSON，尝试提取 content 字段
            const trimmed = inner.trim();
            if (trimmed.startsWith('{') && trimmed.endsWith('}')) {
                try {
                    const obj = JSON.parse(trimmed);
                    if (obj.content && typeof obj.content === 'string') return obj.content;
                    if (obj.paragraph && typeof obj.paragraph === 'string') return obj.paragraph;
                } catch (e) { /* 不是有效 JSON，返回内部文本 */ }
            }
            // 检查是否包含 "content" 字段（非严格 JSON）
            const contentMatch = trimmed.match(/"content"\s*:\s*"((?:[^"\\]|\\.)*)"/);
            if (contentMatch) {
                return contentMatch[1].replace(/\\n/g, '\n').replace(/\\t/g, '\t').replace(/\\"/g, '"').replace(/\\\\/g, '\\');
            }
            return inner;
        });

        // 3. 移除残留的独立 JSON 对象（如 {key: "value"} 或 {"key": "value"}）
        // 仅在行首或标点后出现时清理，避免误伤小说中正常的对话大括号
        result = result.replace(/(?:^|(?<=[。！？\n\r]))\s*\{[^}]*"[^"]*"\s*:\s*"[^"]*"\s*\}/g, '');

        // 4. 清理多余空行（连续 3 个以上空行合并为 2 个）
        result = result.replace(/\n{4,}/g, '\n\n\n');

        return result;
    }

    /**
     * 用正则从 JSON 格式文本中提取 content 字段值
     * 处理 AI 返回的 JSON 格式不严格的情况
     */
    extractContentFieldRegex(text) {
        // 匹配 "content": "..." 或 "content":"..."
        // 捕获引号内的内容，支持转义字符
        const re = /"content"\s*:\s*"((?:[^"\\]|\\.)*)"/s;
        const match = text.match(re);
        if (!match) return null;

        // 还原转义字符
        return match[1]
            .replace(/\\n/g, '\n')
            .replace(/\\t/g, '\t')
            .replace(/\\r/g, '\r')
            .replace(/\\"/g, '"')
            .replace(/\\\\/g, '\\');
    }

    /**
     * 确保值是字符串，防止 [object Object]
     */
    ensureString(value) {
        if (value === null || value === undefined) return '';
        if (typeof value === 'string') return value;
        if (typeof value === 'object') return JSON.stringify(value, null, 2);
        return String(value);
    }

    parseJSONFromResponse(text) {
        if (!text) return null;

        // 尝试直接解析
        try {
            return JSON.parse(text);
        } catch (e) {
            // 尝试提取 JSON 代码块
            const jsonMatch = text.match(/```json\s*([\s\S]*?)```/);
            if (jsonMatch) {
                try {
                    return JSON.parse(jsonMatch[1].trim());
                } catch (e2) {
                    // 继续尝试
                }
            }
            // 尝试提取 { } 包裹的内容
            const braceMatch = text.match(/\{[\s\S]*\}/);
            if (braceMatch) {
                try {
                    return JSON.parse(braceMatch[0]);
                } catch (e3) {
                    // 彻底失败
                }
            }
        }
        return null;
    }

    // ==== 字数审核 ====
    async reviewWords(content, chapterIndex = null) {
        const wordCount = this.countWords(content);
        const { wordMin, wordMax } = this.state.config;

        // 本地先判断字数，符合范围则直接通过，无需调用 AI
        if (wordCount >= wordMin && wordCount <= wordMax) {
            return { passed: true, wordCount };
        }

        // 本地已判定不达标，直接构造调整指令，不依赖 AI 判断
        const action = wordCount < wordMin ? 'expand' : 'shrink';
        const target = wordCount < wordMin ? wordMin : wordMax;
        const instruction = `当前字数 ${wordCount} 字，${action === 'expand' ? '不足' : '超出'}目标范围 ${wordMin}-${wordMax} 字。请将章节内容${action === 'expand' ? '扩充' : '精简'}至约 ${target} 字，保持原有情节和文风。`;

        // 尝试调用 AI 获取更细致的调整建议（最多重试 1 次）
        for (let attempt = 0; attempt < 2; attempt++) {
            try {
                const systemPrompt = this.prompts.review_words || '请审核章节字数。';
                const userMsg = `当前章节字数: ${wordCount}\n要求字数范围: ${wordMin}-${wordMax}\n\n章节内容:\n${content}\n\n请审核字数是否达标，并给出调整建议。`;
                const result = await this.callAI(systemPrompt, userMsg, chapterIndex);

                if (result && result.passed === true) {
                    return { passed: true, wordCount };
                }
                if (result && result.passed === false && result.action) {
                    return result;
                }
                // 格式异常，重试一次
                if (attempt === 0) {
                    console.warn('字数审核 AI 返回格式异常，重试...');
                }
            } catch (e) {
                if (attempt === 0) {
                    console.warn('字数审核 AI 调用异常，重试...', e);
                }
            }
        }

        // 重试后仍失败，使用本地构造的指令兜底
        console.warn('字数审核 AI 重试失败，使用本地判定结果');
        return { passed: false, action, instruction, wordCount };
    }

    async adjustWords(content, reviewResult, chapterIndex = null) {
        const systemPrompt = this.prompts.continue_chapter || '请根据上下文续写小说下一章。';
        const action = reviewResult?.action === 'expand' ? '扩充' : '精简';
        const instruction = reviewResult?.instruction || `请将章节内容${action}到合理字数`;
        const userMsg = `请${action}以下章节内容：\n\n${instruction}\n\n当前章节内容：\n${content}\n\n请输出调整后的完整章节。`;
        const result = await this.callAI(systemPrompt, userMsg, chapterIndex);
        return result?.content || content;
    }

    // ==== 内容审核 ====
    async reviewContent(content, chapterIndex = null) {
        const systemPrompt = this.prompts.review_content || '请根据评判标准审核章节内容。';
        const indexedContent = this.formatParagraphsForReview(content);
        const userMsg = `评判标准：\n${this.state.criteria}\n\n章节内容（按段落编号）：\n${indexedContent}\n\n请逐段审核，指出需要改写的段落编号。`;
        return await this.callAI(systemPrompt, userMsg, chapterIndex);
    }

    // ==== 记忆点管理 ====
    async updateMemories(chapterContent) {
        // 先入先出：维持最大 50 个记忆点
        while (this.state.memories.length > MAX_MEMORIES) {
            this.state.memories.shift();
        }

        // 调用 AI 审核记忆点（使用配置的重试次数）
        const memoryReviewRetries = this.state.config.retries.memoryReview;
        for (let attempt = 0; attempt <= memoryReviewRetries; attempt++) {
            try {
                const systemPrompt = this.prompts.review_memories || '请管理记忆点列表。';
                const userMsg = `当前记忆点列表：\n${JSON.stringify(this.state.memories, null, 2)}\n\n最新章节内容：\n${chapterContent}\n\n请审核并管理记忆点。`;
                const result = await this.callAI(systemPrompt, userMsg);

                if (result && result.actions) {
                    for (const action of result.actions) {
                        if (action.type === 'delete' && action.index !== undefined) {
                            this.state.memories.splice(action.index, 1);
                        } else if (action.type === 'modify' && action.index !== undefined && action.data) {
                            this.state.memories[action.index] = action.data;
                        } else if (action.type === 'add' && action.data) {
                            this.state.memories.push(action.data);
                        }
                    }
                    while (this.state.memories.length > MAX_MEMORIES) {
                        this.state.memories.shift();
                    }
                    return; // 成功就退出
                }
                if (attempt < memoryReviewRetries) {
                    console.warn(`记忆点审核重试 (${attempt + 1}/${memoryReviewRetries})`);
                }
            } catch (e) {
                if (attempt < memoryReviewRetries) {
                    console.warn(`记忆点审核异常 (${attempt + 1}/${memoryReviewRetries})`, e);
                }
            }
        }
        console.warn('记忆点审核所有重试均失败，保持现有记忆点');
    }

    // ==== 预制章节解析 ====
    parsePrebuiltChapters(text) {
        // 匹配 # 第X章 或 ## 第X章 格式
        const chapterRegex = /^#{1,2}\s*第[一二三四五六七八九十百千\d]+章[^\n]*$/gm;
        const matches = [...text.matchAll(chapterRegex)];

        if (matches.length === 0) return [];

        const chapters = [];
        for (let i = 0; i < matches.length; i++) {
            const start = matches[i].index + matches[i][0].length;
            const end = i < matches.length - 1 ? matches[i + 1].index : text.length;
            const content = text.substring(start, end).trim();

            if (content) {
                // 清理标题行
                const cleaned = content.replace(/^#{1,2}\s*第[^\n]+章[^\n]*\n?/gm, '').trim();
                chapters.push(cleaned || content);
            }
        }

        return chapters;
    }

    // ==== 中断处理 ====
    showInterruptModal() {
        this.elements.interruptModal.classList.add('active');
    }

    closeInterruptModal() {
        this.elements.interruptModal.classList.remove('active');
    }

    async onInterruptConfirm() {
        this.closeInterruptModal();
        this.pendingStop = true;
        this.elements.stopBtn.disabled = true;
        this.elements.toolbarStatus.textContent = '正在中断... 等待当前任务完成';
        this.showToast('将在当前任务完成后中断');
    }

    // ==== 内容审核用户审批弹窗 ====
    showReviewApprovalModal(chapterIndex, content) {
        return new Promise((resolve) => {
            this._reviewResolve = resolve;
            this._reviewSelectedIssues = [];

            // 解析上次审核反馈中的 issues
            let issues = [];
            try {
                issues = JSON.parse(this.state.lastReviewFeedback || '[]');
            } catch (e) {
                issues = [];
            }

            // 填充弹窗内容
            this.elements.reviewChapterBadge.textContent = `第 ${chapterIndex} 章`;
            const cr = this.state.config.retries.contentReview;
            this.elements.reviewRetryInfo.textContent =
                `已自动审核 ${this.state.retryCount}/${cr} 次，均未通过。共发现 ${issues.length} 个问题。`;
            this.elements.reviewUserOpinion.value = '';

            // 构建问题列表
            const paragraphs = this.splitParagraphs(content);
            this.elements.reviewIssuesList.innerHTML = issues.map((issue, i) => {
                const paraPreview = paragraphs[issue.paragraph_index] || '';
                const shortPara = paraPreview.length > 80 ? paraPreview.substring(0, 80) + '...' : paraPreview;
                return `
                <div class="review-issue-item" data-index="${i}">
                    <input type="checkbox" class="review-issue-checkbox" data-index="${i}">
                    <div class="review-issue-content">
                        <div class="review-issue-reason">
                            <i class="fas fa-exclamation-triangle"></i> ${this.escapeHtml(issue.reason || '未指定原因')}
                        </div>
                        <div class="review-issue-suggestion">
                            <i class="fas fa-lightbulb"></i> ${this.escapeHtml(issue.suggestion || '')}
                        </div>
                        ${shortPara ? `<div class="review-issue-paragraph"><i class="fas fa-quote-left"></i> ${this.escapeHtml(shortPara)}</div>` : ''}
                    </div>
                </div>`;
            }).join('');

            this.elements.reviewRewriteSelectedBtn.disabled = true;
            this.elements.reviewApprovalModal.classList.add('active');
            new Audio('/file/read/audios/prompt-tone.mp3').play().catch(() => {});
        });
    }

    closeReviewApprovalModal() {
        this.elements.reviewApprovalModal.classList.remove('active');
    }

    dismissReviewApprovalModal() {
        this.elements.reviewApprovalModal.classList.remove('active');
        if (this._reviewResolve) {
            this._reviewResolve(null);
            this._reviewResolve = null;
        }
    }

    updateReviewSelectedState() {
        const checkboxes = this.elements.reviewIssuesList.querySelectorAll('.review-issue-checkbox');
        const selected = [...checkboxes].filter(cb => cb.checked);
        this.elements.reviewRewriteSelectedBtn.disabled = selected.length === 0;

        // 更新选中样式
        this.elements.reviewIssuesList.querySelectorAll('.review-issue-item').forEach(item => {
            const cb = item.querySelector('.review-issue-checkbox');
            item.classList.toggle('selected', cb && cb.checked);
        });
    }

    async onReviewRewriteAll() {
        const opinion = this.elements.reviewUserOpinion.value.trim();
        this._lastUserOpinion = opinion;
        this.addReviewLog('rewrite_all', opinion);
        this.closeReviewApprovalModal();
        this._reviewResolve('rewrite_all');
        this._reviewResolve = null;
    }

    async onReviewRewriteSelected() {
        const checkboxes = this.elements.reviewIssuesList.querySelectorAll('.review-issue-checkbox:checked');
        let issues = [];
        try {
            issues = JSON.parse(this.state.lastReviewFeedback || '[]');
        } catch (e) { }

        this._reviewSelectedIssues = [...checkboxes].map(cb => {
            const idx = parseInt(cb.dataset.index);
            return issues[idx] || null;
        }).filter(Boolean);

        const opinion = this.elements.reviewUserOpinion.value.trim();
        this._lastUserOpinion = opinion;
        this.addReviewLog('rewrite_selected', opinion, this._reviewSelectedIssues.length);
        this.closeReviewApprovalModal();
        this._reviewResolve('rewrite_selected');
        this._reviewResolve = null;
    }

    async onReviewAccept() {
        const opinion = this.elements.reviewUserOpinion.value.trim();
        this._lastUserOpinion = opinion;
        this.addReviewLog('accept', opinion);
        this.closeReviewApprovalModal();
        this._reviewResolve('accept');
        this._reviewResolve = null;
    }

    // ==== 审批日志 ====
    addReviewLog(action, opinion, issueCount) {
        const now = new Date();
        const timeStr = now.toLocaleString('zh-CN');
        const actionLabels = {
            'accept': '认可当前内容',
            'rewrite_all': '重写全章',
            'rewrite_selected': `改写 ${issueCount || 0} 个选中项`
        };

        const entry = {
            time: timeStr,
            action,
            actionLabel: actionLabels[action] || action,
            opinion: opinion || '',
            chapterIndex: this.state.chapterIndex + 1,
            issueCount: issueCount || 0
        };

        this.state.reviewLog.push(entry);
        // 保留最近 100 条日志
        if (this.state.reviewLog.length > 100) {
            this.state.reviewLog.shift();
        }
    }

    /**
     * 获取格式化的审批日志文本
     */
    getReviewLogText() {
        return this.state.reviewLog.map(entry => {
            const actionClass = entry.action === 'accept' ? 'accept' :
                entry.action === 'rewrite_all' ? 'rewrite' : 'rewrite-selected';
            return `<div class="review-log-entry">
                <span class="log-time">${entry.time}</span>
                <span class="log-action ${actionClass}">${entry.actionLabel}</span>
                ${entry.opinion ? `<br><span class="log-opinion">意见: ${this.escapeHtml(entry.opinion)}</span>` : ''}
            </div>`;
        }).join('');
    }

    closeDirectionModal() {
        this.elements.directionModal.classList.remove('active');
        if (this._directionResolve) {
            this._directionResolve();
            this._directionResolve = null;
        }
    }

    showDirectionModal() {
        return new Promise((resolve) => {
            this._directionResolve = resolve;
            this.elements.modalDirection.value = this.state.direction;
            this.elements.modalCriteria.value = this.state.criteria;
            this.elements.directionModal.classList.add('active');
        });
    }

    onDirectionKeep() {
        // 保持现有走向和标准不变
        this.closeDirectionModal();
    }

    async onDirectionUpdate() {
        const newDirection = this.elements.modalDirection.value.trim();
        const newCriteria = this.elements.modalCriteria.value.trim();

        if (this.state.config.dynamicDirection && newDirection) {
            this.state.direction = newDirection;
            this.elements.directionInput.value = newDirection;
        }
        if (this.state.config.dynamicCriteria && newCriteria) {
            this.state.criteria = newCriteria;
            this.elements.criteriaInput.value = newCriteria;
        }

        this.closeDirectionModal();
        await this.saveState();
    }

    // ==== 导出 ====
    showExportModal() {
        this.elements.exportModal.classList.add('active');
    }

    closeExportModal() {
        this.elements.exportModal.classList.remove('active');
    }

    async onExportConfirm() {
        const format = document.querySelector('input[name="exportFormat"]:checked')?.value || 'txt';
        this.closeExportModal();

        let content = '';
        const { chapters } = this.state;

        // 已审核通过的章节
        const approved = chapters.filter(c => c.status === CHAPTER_STATUS.APPROVED);
        // 当前草稿
        const draft = chapters.find(c => c.status === CHAPTER_STATUS.DRAFT);

        if (format === 'md') {
            if (this.state.outline) {
                content += `# 大纲\n\n${this.state.outline}\n\n---\n\n`;
            }
            for (const ch of approved) {
                content += `# 第 ${ch.index} 章\n\n${ch.content}\n\n---\n\n`;
            }
            if (draft) {
                content += `# 第 ${draft.index} 章（草稿）\n\n${draft.content}\n\n`;
            }
        } else {
            // TXT 格式
            if (this.state.outline) {
                content += `【大纲】\n${this.state.outline}\n\n${'='.repeat(50)}\n\n`;
            }
            for (const ch of approved) {
                content += `第 ${ch.index} 章\n\n${ch.content}\n\n${'='.repeat(50)}\n\n`;
            }
            if (draft) {
                content += `第 ${draft.index} 章（草稿）\n\n${draft.content}\n\n`;
            }
        }

        // 下载文件
        const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `novel_${Date.now()}.${format}`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);

        this.showToast('导出成功！');
    }

    // ==== 工具方法 ====
    countWords(text) {
        if (!text) return 0;
        // 统计中文字符 + 英文单词
        const chineseChars = (text.match(/[\u4e00-\u9fff]/g) || []).length;
        const englishWords = (text.match(/[a-zA-Z]+/g) || []).length;
        return chineseChars + englishWords;
    }

    setPhase(phase) {
        this.state.phase = phase;
        this.renderToolbar();
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    showToast(message) {
        this.elements.toastMessage.textContent = message;
        this.elements.toast.classList.add('visible');
        clearTimeout(this._toastTimer);
        this._toastTimer = setTimeout(() => {
            this.elements.toast.classList.remove('visible');
        }, 3000);
    }
}

// ==== 启动 ====
const app = new NovelStudio();
document.addEventListener('DOMContentLoaded', () => app.init());