/**
 * 小说工作室 Pro — 主入口
 * NovelStudioPro 类，聚合所有模块，管理状态与 UI 渲染
 * 新增：随机背景图、分页导航、可折叠进度条、确认按钮状态控制
 */

(function(global) {
    'use strict';

    // ==== 状态文件路径 ====
    var STATE_FILE = 'package/lunar.studio.novel.pro/status.json';

    // PHASE / CHAPTER_STATUS 由 prompts.js 全局定义

    // ==== 创建默认状态 ====
    function createDefaultState() {
        return {
            version: 3,
            initialized: false,
            phase: PHASE.IDLE,
            config: {
                ai: {
                    url: 'http://localhost:36789/v1',
                    name: 'system-multimodal',
                    key: '2000-0218'
                },
                embeddingModel: 'system-embedding',
                wordMin: 3000,
                wordMax: 4000,
                paragraphTarget: 400,
                editChangeThreshold: 50,
                memoryTableName: 'novel_knowledge'
            },
            outline: '',
            criteria: '',
            chapterIndex: 0,
            chapters: [],
            chapterOutlines: [],
            currentDraft: null,
            nextDirection: '',
            nextCriteria: '',
            interruptContext: null,
            totalTokens: 0,
            callLog: []
        };
    }

    // ==== NovelStudioPro 主类 ====
    class NovelStudioPro {
        constructor() {
            this.state = createDefaultState();
            this.config = null;
            this.memory = null;
            this.knowledge = null;
            this.creative = null;
            this.workflow = null;
            this.reviewer = null;
            this.ui = null;
            this.exporter = null;
            this.pendingStop = false;
            this.currentChapterView = 0;
            this._editingChapter = false;
            this._toastTimer = null;
            this._currentSidebarPage = 'creative';
        }

        // ==== 初始化 ====
        async init() {
            // 1. 创建模块实例
            this.config = new global.ConfigManager(this);
            this.memory = new global.MemoryClient(this);
            this.knowledge = new global.KnowledgePanel(this);
            this.creative = new global.CreativeBuilder(this);
            this.workflow = new global.Workflow(this);
            this.reviewer = new global.Reviewer(this);
            this.ui = new global.UI(this);
            this.exporter = new global.Exporter(this);

            // 2. 缓存 DOM + 绑定事件
            this.cacheElements();
            this.bindEvents();

            // 3. 加载随机背景图
            this._applyRandomBackground();

            // 4. 加载状态
            await this.loadState();

            // 5. 确保 AI 配置
            await this.config.ensureConfigured();

            // 6. 初始化记忆库
            try {
                await this.memory.ensureInitialized();
            } catch (e) {
                console.warn('记忆库初始化失败:', e);
            }

            // 7. 渲染
            this.renderAll();

            // 8. 加载知识库列表
            try {
                await this.knowledge.loadAndRender();
            } catch (e) {
                console.warn('知识库列表加载失败:', e);
            }
        }

        // ==== 随机背景图 ====
        _applyRandomBackground() {
            var bgEl = document.getElementById('backgroundLayer');
            if (!bgEl) return;

            // 通过 /background 端点加载随机背景图（服务端随机选取）
            var cacheBuster = Date.now();
            var bgUrl = '/background?t=' + cacheBuster;
            bgEl.style.setProperty('--bg-image', 'url(' + bgUrl + ')');

            // 预加载图片，加载成功后渐显
            var img = new Image();
            img.onload = function() {
                bgEl.style.opacity = '0.6';
            };
            img.onerror = function() {
                // 加载失败时保持 body 渐变作为回退
                bgEl.style.removeProperty('--bg-image');
                bgEl.style.opacity = '0';
            };
            img.src = bgUrl;
        }

        // ==== DOM 缓存 ====
        cacheElements() {
            var ids = [
                'buildBtn', 'continueBtn', 'stopBtn', 'exportBtn', 'editChapterBtn',
                'configBtn', 'resetBtn',
                'statusDot', 'statusText',
                'importZone', 'fileInput', 'selectFileBtn', 'rawTextInput',
                'extractOutlineBtn', 'importHint',
                'outlineInput', 'criteriaInput', 'chapterOutlinesList', 'editOutlinesBtn',
                'outlineStatus', 'criteriaStatus', 'chapterOutlinesStatus',
                'confirmInitBtn', 'confirmInitHint', 'confirmSection',
                'outlinesModal', 'outlinesModalClose', 'outlinesModalBody', 'outlinesModalSaveBtn',
                'spContentInput', 'spAddBtn', 'bulkTextInput', 'aiSummarizeBtn',
                'memoriesPanel', 'memoriesCounter', 'memoriesList',
                'knowledgeTabs', 'quickPane', 'aiSummaryPane',
                'wordMin', 'wordMax', 'paragraphTarget',
                'prevChapterBtn', 'nextChapterBtn',
                'chapterTitle', 'chapterContent', 'chapterEditArea', 'chapterMeta', 'chapterOutlineView', 'chapterSummary',
                'progressCurrentStep', 'progressMessage', 'progressPanel',
                'draftPanel', 'draftMeta', 'draftParagraphs',
                'tokenCurrent', 'tokenTotal', 'chapterCount', 'wordCount',
                'directionInput', 'criteriaViewInput',
                'nextDirectionCard', 'nextDirectionPreview', 'nextCriteriaPreview',
                'nextDirectionEditBtn', 'nextDirectionAcceptBtn', 'nextDirectionRejectBtn',
                'humanApprovalCard', 'openApprovalBtn', 'approvalHint',
                'reviewTab', 'reviewBadge', 'reviewInline', 'reviewEmpty', 'pageReview',
                'sidebarTabs', 'pageCreative', 'pageOutlines', 'pageKnowledge',
                'configModal', 'configModalClose', 'aiUrlInput', 'aiNameInput', 'aiKeyInput', 'configSaveBtn',
                'exportModal', 'exportCancelBtn', 'exportConfirmBtn', 'exportModalClose',
                'interruptModal', 'interruptCancelBtn', 'interruptConfirmBtn', 'interruptModalClose',
                'toast', 'toastMessage'
            ];
            this.elements = {};
            for (var i = 0; i < ids.length; i++) {
                this.elements[ids[i]] = document.getElementById(ids[i]);
            }
        }

        // ==== 事件绑定 ====
        bindEvents() {
            var self = this;

            // 工具栏
            this.elements.buildBtn.addEventListener('click', function() { self.onBuild(); });
            this.elements.continueBtn.addEventListener('click', function() { self.onContinue(); });
            this.elements.stopBtn.addEventListener('click', function() { self.showInterruptModal(); });
            this.elements.exportBtn.addEventListener('click', function() { self.showExportModal(); });
            this.elements.editChapterBtn.addEventListener('click', function() { self.toggleEditChapter(); });
            this.elements.configBtn.addEventListener('click', function() { self.config.showConfigModal(); });
            this.elements.resetBtn.addEventListener('click', function() { self.resetState(); });

            // 知识库 Tab 切换
            this.elements.knowledgeTabs.querySelectorAll('.tab-btn').forEach(function(btn) {
                btn.addEventListener('click', function() { self.switchKnowledgeTab(btn.dataset.tab); });
            });

            // 章节导航
            this.elements.prevChapterBtn.addEventListener('click', function() { self.navigateChapter(-1); });
            this.elements.nextChapterBtn.addEventListener('click', function() { self.navigateChapter(1); });

            // 分页导航
            this.elements.sidebarTabs.querySelectorAll('.sidebar-tab').forEach(function(tab) {
                tab.addEventListener('click', function() {
                    var page = tab.dataset.page;
                    self.switchSidebarPage(page);
                });
            });

            // AI 配置模态框
            this.elements.configModalClose.addEventListener('click', function() { self.closeConfigModal(); });
            this.elements.configSaveBtn.addEventListener('click', function() { self.config.saveConfig(); });
            this.elements.configModal.addEventListener('click', function(e) {
                if (e.target === self.elements.configModal) self.closeConfigModal();
            });

            // 导出模态框
            this.elements.exportCancelBtn.addEventListener('click', function() { self.closeExportModal(); });
            this.elements.exportConfirmBtn.addEventListener('click', function() { self.onExportConfirm(); });
            this.elements.exportModalClose.addEventListener('click', function() { self.closeExportModal(); });
            this.elements.exportModal.addEventListener('click', function(e) {
                if (e.target === self.elements.exportModal) self.closeExportModal();
            });

            // 中断模态框
            this.elements.interruptCancelBtn.addEventListener('click', function() { self.closeInterruptModal(); });
            this.elements.interruptConfirmBtn.addEventListener('click', function() { self.onInterruptConfirm(); });
            this.elements.interruptModalClose.addEventListener('click', function() { self.closeInterruptModal(); });
            this.elements.interruptModal.addEventListener('click', function(e) {
                if (e.target === self.elements.interruptModal) self.closeInterruptModal();
            });

            // 创意构建
            this.elements.extractOutlineBtn.addEventListener('click', function() { self.creative.extractOutline(); });
            this.elements.selectFileBtn.addEventListener('click', function() { self.elements.fileInput.click(); });
            this.elements.fileInput.addEventListener('change', function(e) { self.creative.onFileSelected(e); });
            this.elements.confirmInitBtn.addEventListener('click', function() { self.creative.confirmInit(); });
            this.elements.importZone.addEventListener('dragover', function(e) { e.preventDefault(); self.elements.importZone.classList.add('drag-over'); });
            this.elements.importZone.addEventListener('dragleave', function() { self.elements.importZone.classList.remove('drag-over'); });
            this.elements.importZone.addEventListener('drop', function(e) { e.preventDefault(); self.elements.importZone.classList.remove('drag-over'); self.creative.onFileDropped(e); });
            // 创意构建输入框实时同步与确认按钮状态
            this.elements.outlineInput.addEventListener('input', function() {
                self.state.outline = self.elements.outlineInput.value;
                self._updateCreativeStatus();
            });
            this.elements.criteriaInput.addEventListener('input', function() {
                self.state.criteria = self.elements.criteriaInput.value;
                self._updateCreativeStatus();
            });

            // 章节大纲编辑按钮
            this.elements.editOutlinesBtn.addEventListener('click', function() { self.openOutlinesModal(); });

            // 章节大纲模态框
            this.elements.outlinesModalClose.addEventListener('click', function() { self.closeOutlinesModal(); });
            this.elements.outlinesModalSaveBtn.addEventListener('click', function() { self.saveOutlinesModal(); });
            this.elements.outlinesModal.addEventListener('click', function(e) {
                if (e.target === self.elements.outlinesModal) self.closeOutlinesModal();
            });

            // 知识库
            this.elements.spAddBtn.addEventListener('click', function() { self.knowledge.addSingle(); });
            this.elements.aiSummarizeBtn.addEventListener('click', function() { self.knowledge.aiSummarize(); });

            // 下一章走向
            this.elements.nextDirectionEditBtn.addEventListener('click', function() { self.onNextDirectionEdit(); });
            this.elements.nextDirectionAcceptBtn.addEventListener('click', function() { self.onNextDirectionAccept(); });
            this.elements.nextDirectionRejectBtn.addEventListener('click', function() { self.onNextDirectionReject(); });

            // 人工审核
            this.elements.openApprovalBtn.addEventListener('click', function() { self.onOpenApproval(); });

            // 参数同步
            ['wordMin', 'wordMax', 'paragraphTarget'].forEach(function(id) {
                self.elements[id].addEventListener('change', function() { self.collectConfig(); });
            });

            // 可折叠面板切换
            document.querySelectorAll('[data-toggle]').forEach(function(el) {
                el.addEventListener('click', function() {
                    var targetId = el.dataset.toggle;
                    var target = document.getElementById(targetId);
                    if (!target) return;
                    el.closest('.panel-section').classList.toggle('collapsed');
                    target.classList.toggle('collapsed');
                });
            });
        }

        // ==== 分页切换 ====
        switchSidebarPage(page) {
            this._currentSidebarPage = page;

            // 更新 Tab 激活状态
            this.elements.sidebarTabs.querySelectorAll('.sidebar-tab').forEach(function(tab) {
                tab.classList.toggle('active', tab.dataset.page === page);
            });

            // 更新页面显示
            var pages = document.querySelectorAll('.sidebar-page');
            pages.forEach(function(p) {
                p.classList.toggle('active', p.dataset.page === page);
            });
        }

        // ==== 状态持久化 ====
        async loadState() {
            try {
                var resp = await fetch('/file/read/' + STATE_FILE);
                if (resp.ok) {
                    var data = await resp.json();
                    this.state = Object.assign(createDefaultState(), data);
                    this.state.config = Object.assign(createDefaultState().config, data.config || {});
                    this.state.config.ai = Object.assign(createDefaultState().config.ai, (data.config && data.config.ai) || {});
                    // 中间阶段重置为 IDLE
                    var midPhases = [PHASE.BUILDING, PHASE.GENERATING_PARAGRAPHS, PHASE.WORD_REVIEW, PHASE.CONTENT_REVIEW, PHASE.HUMAN_REVIEW, PHASE.GENERATING_SUMMARY];
                    if (midPhases.includes(this.state.phase) || this.state.phase === PHASE.INTERRUPTED) {
                        this.state.phase = PHASE.IDLE;
                        this.showToast('已恢复上次工作状态');
                    }
                } else if (resp.status === 404) {
                    this.state = createDefaultState();
                }
            } catch (e) {
                this.state = createDefaultState();
            }
        }

        async saveState() {
            try {
                var jsonStr = JSON.stringify(this.state, null, '\t');
                var blob = new Blob([jsonStr], { type: 'application/json' });
                var resp = await fetch('/file/write', {
                    method: 'POST',
                    headers: {
                        'X-File-Name': btoa(unescape(encodeURIComponent(STATE_FILE))),
                        'X-Overwrite': 'true'
                    },
                    body: blob
                });
                if (!resp.ok) console.error('保存状态失败');
            } catch (e) {
                console.error('保存状态异常', e);
            }
        }

        // ==== 渲染 ====
        renderAll() {
            this.renderToolbar();
            this.renderSidebarState();
            this.renderChapterView();
            this.updateChapterNav();
            this.renderCreative();
            this.renderTokenBoard();
            this.renderReviewTab();
            // 同步参数到 UI
            this.elements.wordMin.value = this.state.config.wordMin;
            this.elements.wordMax.value = this.state.config.wordMax;
            this.elements.paragraphTarget.value = this.state.config.paragraphTarget;
            this.elements.directionInput.value = this.state.nextDirection || '';
            this.elements.criteriaViewInput.value = this.state.criteria || '';
        }

        renderToolbar() {
            var phase = this.state.phase;
            var isBusy = phase !== PHASE.IDLE && phase !== PHASE.INTERRUPTED;
            this.elements.buildBtn.disabled = isBusy || !this.state.initialized;
            this.elements.continueBtn.disabled = isBusy || !this.state.initialized;
            this.elements.stopBtn.disabled = !isBusy || this.pendingStop;
            var phaseNames = {};
            phaseNames[PHASE.IDLE] = '就绪';
            phaseNames[PHASE.BUILDING] = '构建中...';
            phaseNames[PHASE.GENERATING_PARAGRAPHS] = '生成段落中...';
            phaseNames[PHASE.WORD_REVIEW] = '字数审核中...';
            phaseNames[PHASE.CONTENT_REVIEW] = '内容审核中...';
            phaseNames[PHASE.HUMAN_REVIEW] = '人工审核中...';
            phaseNames[PHASE.GENERATING_SUMMARY] = '生成摘要中...';
            phaseNames[PHASE.INTERRUPTED] = '已中断';
            this.elements.statusText.textContent = phaseNames[phase] || phase;
            this.elements.statusDot.className = 'status-dot' + (phase !== PHASE.IDLE && phase !== PHASE.INTERRUPTED ? ' busy' : '') + (phase === PHASE.INTERRUPTED ? ' error' : '');
        }

        renderSidebarState() {
            // 初始化后折叠创意构建区，未初始化则展开
            var creativeSection = document.getElementById('creativeSection');
            if (creativeSection) {
                if (this.state.initialized) {
                    creativeSection.classList.add('collapsed');
                    var body = document.getElementById('creativeBody');
                    if (body) body.classList.add('collapsed');
                } else {
                    creativeSection.classList.remove('collapsed');
                    var body2 = document.getElementById('creativeBody');
                    if (body2) body2.classList.remove('collapsed');
                }
            }

            // 初始化后默认切换到章节纲要页
            if (this.state.initialized && this._currentSidebarPage === 'creative') {
                this.switchSidebarPage('outlines');
            }
        }

        renderChapterView() {
            var chapters = this.state.chapters;
            var chapterIndex = this.state.chapterIndex;
            var chapter = chapters[this.currentChapterView];

            // 草稿面板：仅在有 currentDraft 且有段落时显示
            if (this.state.currentDraft && this.state.currentDraft.paragraphs && this.state.currentDraft.paragraphs.length > 0) {
                this.elements.draftPanel.style.display = 'flex';
            } else {
                this.elements.draftPanel.style.display = 'none';
            }

            if (!chapter) {
                this.elements.chapterTitle.textContent = '第 ' + (chapterIndex + 1) + ' 章';
                this.elements.chapterContent.innerHTML = '<div class="empty-state"><i class="fas fa-feather-alt"></i><h3>小说工作室 Pro</h3><p>完成创意构建后，在此开始小说编写</p></div>';
                this.elements.chapterSummary.textContent = '';
                this.elements.chapterOutlineView.textContent = '';
                return;
            }
            this.elements.chapterTitle.textContent = '第 ' + chapter.index + ' 章';
            if (typeof marked !== 'undefined') {
                this.elements.chapterContent.innerHTML = marked.parse(chapter.content || '');
            } else {
                this.elements.chapterContent.textContent = chapter.content || '';
            }
            this.elements.chapterSummary.textContent = chapter.summary ? '摘要：' + chapter.summary : '';
            this.elements.chapterOutlineView.textContent = chapter.outline ? '大纲：' + chapter.outline : '';
            this.elements.chapterMeta.style.display = (chapter.summary || chapter.outline) ? '' : 'none';
        }

        renderCreative() {
            var outline = this.state.outline;
            var criteria = this.state.criteria;
            var chapterOutlines = this.state.chapterOutlines;
            var initialized = this.state.initialized;
            this.elements.outlineInput.value = outline;
            this.elements.criteriaInput.value = criteria;
            this.elements.outlineStatus.textContent = outline ? '已填写' : '未填写';
            this.elements.outlineStatus.className = 'title-badge' + (outline ? ' filled' : '');
            this.elements.criteriaStatus.textContent = criteria ? '已填写' : '未填写';
            this.elements.criteriaStatus.className = 'title-badge' + (criteria ? ' filled' : '');
            var hasOutlines = chapterOutlines && chapterOutlines.length > 0;
            this.elements.chapterOutlinesStatus.textContent = hasOutlines ? chapterOutlines.length + ' 章' : '未填写';
            this.elements.chapterOutlinesStatus.className = 'title-badge' + (hasOutlines ? ' filled' : '');

            // 增强的确认按钮状态控制
            this._updateConfirmButton();

            // 渲染章节大纲摘要列表 + 编辑按钮
            if (hasOutlines) {
                this.elements.chapterOutlinesList.innerHTML = chapterOutlines.map(function(co, i) {
                    var preview = (co.outline || '').substring(0, 60);
                    if (co.outline && co.outline.length > 60) preview += '...';
                    return '<div class="outline-summary-item"><span class="outline-summary-num">' + (i + 1) + '</span><span class="outline-summary-text">' + this.escapeHtml(preview) + '</span></div>';
                }.bind(this)).join('');
                this.elements.editOutlinesBtn.innerHTML = '<i class="fas fa-edit"></i> 查看/编辑章节大纲';
            } else {
                this.elements.chapterOutlinesList.innerHTML = '<div class="empty-state-small">AI 未提取到章节大纲，可手动添加</div>';
                this.elements.editOutlinesBtn.innerHTML = '<i class="fas fa-plus"></i> 添加章节大纲';
            }
            this.elements.editOutlinesBtn.style.display = '';
        }

        // ==== 增强的确认按钮状态控制 ====
        _updateConfirmButton() {
            var outline = (this.elements.outlineInput.value || '').trim();
            var criteria = (this.elements.criteriaInput.value || '').trim();
            var chapterOutlines = this.state.chapterOutlines || [];

            // 更严格的验证条件
            var outlineValid = outline.length >= 10;
            var criteriaValid = criteria.length >= 10;
            var chaptersValid = chapterOutlines.length >= 1;

            var allValid = outlineValid && criteriaValid && chaptersValid;
            this.elements.confirmInitBtn.disabled = !allValid;

            // 显示提示信息
            var hint = this.elements.confirmInitHint;
            if (allValid) {
                hint.style.display = 'none';
            } else {
                var reasons = [];
                if (!outlineValid) reasons.push('总体大纲内容不足');
                if (!criteriaValid) reasons.push('评判标准内容不足');
                if (!chaptersValid) reasons.push('至少需要一个章节大纲');
                hint.textContent = '待完成: ' + reasons.join('、');
                hint.style.display = 'block';
            }
        }

        renderTokenBoard() {
            var chapters = this.state.chapters;
            var totalTokens = this.state.totalTokens || 0;
            var currentChapter = chapters[this.currentChapterView];

            var currentTokens = (currentChapter && currentChapter.tokens) || 0;
            if (this.state.currentDraft && this.state.callLog) {
                var draftIdx = this.state.currentDraft.chapterIndex;
                var draftTokens = this.state.callLog
                    .filter(function(c) { return c.chapterIndex === draftIdx; })
                    .reduce(function(sum, c) { return sum + (c.totalTokens || 0); }, 0);
                currentTokens = currentTokens + draftTokens;
            }

            var approvedTokens = chapters.reduce(function(sum, ch) { return sum + (ch.tokens || 0); }, 0);
            var unsavedTokens = (this.state.callLog || []).reduce(function(sum, c) { return sum + (c.totalTokens || 0); }, 0);
            totalTokens = approvedTokens + unsavedTokens;
            this.state.totalTokens = totalTokens;

            this.elements.tokenCurrent.textContent = currentTokens;
            this.elements.tokenTotal.textContent = totalTokens;
            this.elements.chapterCount.textContent = chapters.filter(function(c) { return c.status === CHAPTER_STATUS.APPROVED; }).length;
            this.elements.wordCount.textContent = currentChapter ? this.countWords(currentChapter.content) : 0;
        }

        // ==== 审核 Tab 显示控制 ====
        renderReviewTab() {
            var hasReviewContent = this.state.phase === PHASE.HUMAN_REVIEW && this.state.currentDraft;
            var reviewTab = this.elements.reviewTab;
            var reviewBadge = this.elements.reviewBadge;

            if (hasReviewContent) {
                reviewTab.style.display = '';
                reviewBadge.style.display = '';
                // 如果当前在审核页但有审核内容，保持显示
                if (this.state.currentDraft && this.state.currentDraft.paragraphs && this.state.currentDraft.paragraphs.length > 0) {
                    this.elements.humanApprovalCard.style.display = '';
                    this.elements.reviewEmpty.style.display = 'none';
                    this.elements.approvalHint.textContent = '第 ' + this.state.currentDraft.chapterIndex + ' 章已生成，共 ' + this.state.currentDraft.paragraphs.length + ' 段，请审核。';
                }
            } else {
                reviewTab.style.display = 'none';
                reviewBadge.style.display = 'none';
                this.elements.humanApprovalCard.style.display = 'none';
                this.elements.reviewInline.style.display = 'none';
                this.elements.reviewEmpty.style.display = '';
                // 如果当前在审核页，切回章节纲要
                if (this._currentSidebarPage === 'review') {
                    this.switchSidebarPage('outlines');
                }
            }
        }

        // ==== 知识库 Tab 切换 ====
        switchKnowledgeTab(tabName) {
            this.elements.knowledgeTabs.querySelectorAll('.tab-btn').forEach(function(btn) {
                btn.classList.toggle('active', btn.dataset.tab === tabName);
            });
            this.elements.quickPane.classList.toggle('active', tabName === 'quick');
            this.elements.aiSummaryPane.classList.toggle('active', tabName === 'aiSummary');
        }

        // ==== 配置模态框 ====
        closeConfigModal() {
            this.elements.configModal.classList.remove('active');
        }

        // ==== 导出模态框 ====
        showExportModal() { this.elements.exportModal.classList.add('active'); }
        closeExportModal() { this.elements.exportModal.classList.remove('active'); }
        async onExportConfirm() {
            var format = 'txt';
            var radios = document.querySelectorAll('input[name="exportFormat"]');
            for (var i = 0; i < radios.length; i++) {
                if (radios[i].checked) { format = radios[i].value; break; }
            }
            if (this.exporter) await this.exporter.export(format);
            this.closeExportModal();
        }

        // ==== 中断模态框 ====
        showInterruptModal() { this.elements.interruptModal.classList.add('active'); }
        closeInterruptModal() { this.elements.interruptModal.classList.remove('active'); }
        onInterruptConfirm() {
            this.closeInterruptModal();
            this.pendingStop = true;
            if (this.workflow) this.workflow.requestStop();
            this.elements.stopBtn.disabled = true;
            this.showToast('将在当前任务完成后中断');
        }

        // ==== 章节导航 ====
        navigateChapter(delta) {
            var newIdx = this.currentChapterView + delta;
            if (newIdx < 0 || newIdx >= this.state.chapters.length) return;
            this.currentChapterView = newIdx;
            this.renderChapterView();
            this.updateChapterNav();
            this.renderTokenBoard();
        }

        updateChapterNav() {
            var chapters = this.state.chapters;
            this.elements.prevChapterBtn.disabled = this.currentChapterView <= 0;
            this.elements.nextChapterBtn.disabled = this.currentChapterView >= chapters.length - 1;
            var currentChapter = chapters[this.currentChapterView];
            if (this._editingChapter) {
                this.elements.editChapterBtn.disabled = false;
            } else {
                this.elements.editChapterBtn.disabled = !currentChapter || currentChapter.status !== CHAPTER_STATUS.APPROVED;
            }
        }

        // ==== 编辑章节 ====
        async toggleEditChapter() {
            var chapter = this.state.chapters[this.currentChapterView];
            if (!chapter || chapter.status !== CHAPTER_STATUS.APPROVED) {
                this.showToast('仅可编辑已审核通过的章节');
                return;
            }
            if (this._editingChapter) {
                var newContent = this.elements.chapterEditArea.value;
                var oldContent = chapter.content;
                var changed = newContent !== oldContent;
                this._editingChapter = false;
                this.elements.chapterEditArea.style.display = 'none';
                this.elements.chapterContent.style.display = '';
                this.elements.editChapterBtn.innerHTML = '<i class="fas fa-edit"></i> 编辑';
                if (changed) {
                    chapter.content = newContent;
                    this.renderChapterView();
                    await this.saveState();
                    var diff = Math.abs(newContent.length - oldContent.length);
                    var threshold = this.state.config.editChangeThreshold || 50;
                    if (diff > threshold) {
                        if (confirm('内容改动较大，是否重写章节摘要？')) {
                            this.showToast('正在重写摘要...');
                            if (this.workflow) {
                                await this.workflow.step10_editChapter(chapter.index, newContent);
                            }
                        }
                    }
                    this.showToast('章节内容已保存');
                }
            } else {
                this._editingChapter = true;
                this.elements.chapterEditArea.value = chapter.content;
                this.elements.chapterContent.style.display = 'none';
                this.elements.chapterEditArea.style.display = '';
                this.elements.editChapterBtn.innerHTML = '<i class="fas fa-save"></i> 保存修改';
                this.elements.chapterEditArea.focus();
            }
        }

        // ==== 收集配置 ====
        collectConfig() {
            this.state.config.wordMin = parseInt(this.elements.wordMin.value) || 3000;
            this.state.config.wordMax = parseInt(this.elements.wordMax.value) || 4000;
            this.state.config.paragraphTarget = parseInt(this.elements.paragraphTarget.value) || 400;
        }

        // ==== 构建/续写 ====
        async onBuild() {
            this.collectConfig();
            this.state.chapterIndex = 0;
            this.state.chapters = [];
            this.state.currentDraft = null;
            this.state.nextDirection = '';
            this.state.nextCriteria = '';
            this.state.interruptContext = null;
            this.state.callLog = [];
            this.pendingStop = false;
            if (this.workflow) await this.workflow.startChapter();
        }

        async onContinue() {
            this.collectConfig();
            if (this.state.phase === PHASE.INTERRUPTED) {
                this.state.phase = PHASE.IDLE;
                this.pendingStop = false;
                if (this.workflow && this.state.interruptContext) {
                    await this.workflow.resumeFromInterrupt();
                    return;
                }
            }
            this.state.chapterIndex = this.state.chapters.length;
            this.state.currentDraft = null;
            this.state.interruptContext = null;
            this.state.callLog = [];
            this.pendingStop = false;
            if (this.workflow) await this.workflow.startChapter();
        }

        // ==== 下一章走向 ====
        onNextDirectionEdit() {
            this.elements.nextDirectionPreview.readOnly = false;
            this.elements.nextCriteriaPreview.readOnly = false;
        }

        async onNextDirectionAccept() {
            var dir = this.elements.nextDirectionPreview.value.trim();
            var cri = this.elements.nextCriteriaPreview.value.trim();
            this.state.nextDirection = '';
            this.state.nextCriteria = '';
            this.elements.directionInput.value = dir;
            this.elements.criteriaViewInput.value = cri || this.state.criteria;
            this.elements.nextDirectionCard.style.display = 'none';
            await this.saveState();
            this.showToast('下一章走向已采纳');
        }

        async onNextDirectionReject() {
            this.state.nextDirection = '';
            this.state.nextCriteria = '';
            this.elements.nextDirectionCard.style.display = 'none';
            await this.saveState();
        }

        // ==== 人工审核 ====
        onOpenApproval() {
            // 切换到审核页并显示内联审核
            this.switchSidebarPage('review');
            this.elements.humanApprovalCard.style.display = 'none';
            if (this.ui) this.ui.showHumanApproval(this.state.currentDraft);
        }

        // ==== 重置 ====
        async resetState() {
            if (!confirm('确定要放弃所有缓存，恢复初始状态吗？')) return;
            try {
                await fetch('/file/delete/' + encodeURIComponent(STATE_FILE), { method: 'DELETE' });
            } catch (e) { /* ignore */ }
            this.state = createDefaultState();
            this.pendingStop = false;
            this.currentChapterView = 0;
            this._editingChapter = false;
            this._currentSidebarPage = 'creative';
            this.switchSidebarPage('creative');
            this._applyRandomBackground();
            this.renderAll();
            this.showToast('已恢复初始状态');
        }

        // ==== 创意构建状态更新 ====
        _updateCreativeStatus() {
            var outline = (this.elements.outlineInput.value || '').trim();
            var criteria = (this.elements.criteriaInput.value || '').trim();
            this.elements.outlineStatus.textContent = outline ? '已填写' : '未填写';
            this.elements.outlineStatus.className = 'title-badge' + (outline ? ' filled' : '');
            this.elements.criteriaStatus.textContent = criteria ? '已填写' : '未填写';
            this.elements.criteriaStatus.className = 'title-badge' + (criteria ? ' filled' : '');
            this._updateConfirmButton();
        }

        // ==== 章节大纲模态框 ====
        openOutlinesModal() {
            this._renderOutlinesModalBody();
            this.elements.outlinesModal.classList.add('active');
        }

        _renderOutlinesModalBody() {
            var chapterOutlines = this.state.chapterOutlines || [];
            var body = this.elements.outlinesModalBody;
            var self = this;
            var html = '';
            if (chapterOutlines.length === 0) {
                html += '<div class="empty-state-small">暂无章节大纲，点击下方添加</div>';
            } else {
                chapterOutlines.forEach(function(co, i) {
                    html += '<div class="outline-edit-item">' +
                        '<div class="outline-edit-header">' +
                            '<span class="outline-edit-label">第 ' + (i + 1) + ' 章</span>' +
                            '<button class="btn-glass btn-glass-small btn-glass-danger outline-delete-btn" data-index="' + i + '"><i class="fas fa-trash"></i></button>' +
                        '</div>' +
                        '<textarea class="panel-textarea outline-edit-text" data-index="' + i + '" rows="4">' + self.escapeHtml(co.outline) + '</textarea>' +
                        '</div>';
                });
            }
            html += '<button id="addOutlineBtn" class="btn-glass btn-block" style="margin-top:0.5rem;"><i class="fas fa-plus"></i> 添加章节</button>';
            body.innerHTML = html;

            var addBtn = body.querySelector('#addOutlineBtn');
            addBtn.addEventListener('click', function() { self._addOutlineItem(); });

            body.querySelectorAll('.outline-delete-btn').forEach(function(btn) {
                btn.addEventListener('click', function() { self._removeOutlineItem(parseInt(btn.dataset.index)); });
            });
        }

        _addOutlineItem() {
            if (!this.state.chapterOutlines) this.state.chapterOutlines = [];
            this.state.chapterOutlines.push({ outline: '' });
            this._renderOutlinesModalBody();
            var body = this.elements.outlinesModalBody;
            body.scrollTop = body.scrollHeight;
            var textareas = body.querySelectorAll('.outline-edit-text');
            if (textareas.length > 0) textareas[textareas.length - 1].focus();
        }

        _removeOutlineItem(index) {
            if (!this.state.chapterOutlines) return;
            this.state.chapterOutlines.splice(index, 1);
            this._renderOutlinesModalBody();
        }

        closeOutlinesModal() {
            this.elements.outlinesModal.classList.remove('active');
        }

        async saveOutlinesModal() {
            var textareas = this.elements.outlinesModalBody.querySelectorAll('.outline-edit-text');
            var self = this;
            textareas.forEach(function(ta) {
                var idx = parseInt(ta.dataset.index);
                var newVal = ta.value.trim();
                if (self.state.chapterOutlines[idx]) {
                    self.state.chapterOutlines[idx].outline = newVal;
                }
            });
            this.state.chapterOutlines = this.state.chapterOutlines.filter(function(co) { return co.outline.trim().length > 0; });
            await this.saveState();
            this.renderCreative();
            this.showToast('章节大纲已保存');
            this.closeOutlinesModal();
        }

        // ==== 工具方法 ====
        countWords(text) {
            if (!text) return 0;
            var chineseChars = (text.match(/[\u4e00-\u9fff]/g) || []).length;
            var englishWords = (text.match(/[a-zA-Z]+/g) || []).length;
            return chineseChars + englishWords;
        }

        escapeHtml(text) {
            var div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        showToast(message) {
            this.elements.toastMessage.textContent = message;
            this.elements.toast.classList.add('visible');
            clearTimeout(this._toastTimer);
            var self = this;
            this._toastTimer = setTimeout(function() { self.elements.toast.classList.remove('visible'); }, 3000);
        }

        setPhase(phase) {
            this.state.phase = phase;
            this.renderToolbar();
            this.renderReviewTab();
        }
    }

    // ==== 暴露到全局 ====
    global.NovelStudioPro = NovelStudioPro;
    global.PHASE = PHASE;
    global.CHAPTER_STATUS = CHAPTER_STATUS;

    // ==== 启动 ====
    var app = new NovelStudioPro();
    document.addEventListener('DOMContentLoaded', function() { app.init(); });
})(typeof window !== 'undefined' ? window : this);