/**
 * UI 模块
 * 审核内容内联渲染在左侧审核页，不再使用二级模态框
 * 审核通过后：生成摘要 + 推理下章走向
 */

(function(global) {
    'use strict';

    var PHASE = global.PHASE;
    var CHAPTER_STATUS = global.CHAPTER_STATUS;

    // ==== UI 类 ====
    class UI {
        constructor(app) {
            this.app = app;
        }

        // ==== 显示人工审核内容（内联渲染在审核页） ====
        showHumanApproval(draft) {
            if (!draft || !draft.paragraphs || draft.paragraphs.length === 0) {
                this.app.showToast('没有可审核的草稿内容');
                return;
            }

            var self = this;
            var reviewResults = draft.reviewResults || [];
            var paragraphs = draft.paragraphs;
            var reviewInline = this.app.elements.reviewInline;

            // 隐藏入口卡片，显示审核内容
            this.app.elements.humanApprovalCard.style.display = 'none';
            this.app.elements.reviewEmpty.style.display = 'none';
            reviewInline.style.display = 'flex';

            // 构建审核面板
            var html = '';

            // 头部
            html += '<div class="review-inline-header">';
            html += '<h3><i class="fas fa-gavel"></i> 人工审核 — 第 ' + draft.chapterIndex + ' 章</h3>';
            html += '<span class="review-stats">共 ' + paragraphs.length + ' 段 | ' + self.app.countWords(paragraphs.join('\n')) + ' 字</span>';
            html += '</div>';

            // 段落列表
            html += '<div class="review-inline-body" id="reviewInlineBody">';
            for (var i = 0; i < paragraphs.length; i++) {
                var review = reviewResults.find(function(r) { return r.index === i + 1; }) || null;
                html += self._buildParagraphCard(i, paragraphs[i], review, draft);
            }
            html += '</div>';

            // 底部操作栏
            html += '<div class="review-inline-footer">';
            html += '<button class="btn-glass" id="reviewRewriteAllBtn"><i class="fas fa-redo"></i> 重写整章</button>';
            html += '<button class="btn-glass btn-glass-primary" id="reviewApproveBtn"><i class="fas fa-check-circle"></i> 认可并继续</button>';
            html += '</div>';

            reviewInline.innerHTML = html;

            // 绑定事件
            this._bindReviewEvents(draft);
        }

        // ==== 构建单段落卡片 HTML ====
        _buildParagraphCard(index, content, review, draft) {
            var _self = this;
            var passedClass = review ? (review.passed ? ' passed' : ' failed') : '';
            var attemptsText = review && review.attempts > 0 ? '（审核 ' + review.attempts + ' 轮）' : '';
            var statusText = review ? (review.passed ? '自动审核通过' : '自动审核未通过' + attemptsText) : '未审核';

            var html = '<div class="review-paragraph-card" data-index="' + index + '">';

            // 段落头部
            html += '<div class="review-para-header">';
            html += '<span class="review-para-num">第 ' + (index + 1) + ' 段</span>';
            html += '<span class="review-para-words">' + this.app.countWords(content) + ' 字</span>';
            html += '<span class="review-para-status' + passedClass + '">' + statusText + '</span>';
            html += '</div>';

            // 内容展示
            html += '<div class="review-para-content">' + this._escapeHtml(content) + '</div>';

            // 编辑区（默认隐藏）
            html += '<textarea class="review-para-edit-area" style="display:none;" rows="3">' + this._escapeHtml(content) + '</textarea>';

            // 审核问题
            if (review && review.issues && review.issues.length > 0) {
                html += '<div class="review-para-issues">';
                html += '<div class="issues-title"><i class="fas fa-exclamation-circle"></i> 审核发现的问题</div>';
                review.issues.forEach(function(issue, ii) {
                    html += '<div class="issue-item"><label class="issue-checkbox"><input type="checkbox" data-issue-index="' + ii + '" checked> ' + _self._escapeHtml(issue) + '</label></div>';
                });
                if (review.suggestions && review.suggestions.length > 0) {
                    html += '<div class="issues-title" style="margin-top:6px;color:var(--accent);"><i class="fas fa-lightbulb"></i> 改进建议</div>';
                    review.suggestions.forEach(function(sug, si) {
                        html += '<div class="issue-item suggestion-item"><label class="issue-checkbox"><input type="checkbox" data-suggestion-index="' + si + '" checked> ' + _self._escapeHtml(sug) + '</label></div>';
                    });
                }
                html += '</div>';
            }

            // 修改意见
            html += '<div class="review-para-opinion">';
            html += '<textarea class="opinion-input" placeholder="输入你的修改意见（AI 将基于此改写段落）..." rows="2"></textarea>';
            html += '</div>';

            // 操作按钮
            html += '<div class="review-para-actions">';
            html += '<button class="btn-glass btn-glass-small review-edit-btn" data-index="' + index + '"><i class="fas fa-edit"></i> 编辑</button>';
            html += '<button class="btn-glass btn-glass-small review-ai-btn" data-index="' + index + '"><i class="fas fa-robot"></i> AI 改写</button>';
            html += '<button class="btn-glass btn-glass-small btn-glass-danger review-delete-btn" data-index="' + index + '"><i class="fas fa-trash"></i> 删除</button>';
            html += '</div>';

            html += '</div>';
            return html;
        }

        // ==== 绑定审核事件 ====
        _bindReviewEvents(draft) {
            var self = this;
            var reviewInline = this.app.elements.reviewInline;

            // 编辑按钮
            reviewInline.querySelectorAll('.review-edit-btn').forEach(function(btn) {
                btn.addEventListener('click', function() {
                    var idx = parseInt(btn.dataset.index);
                    self._toggleEdit(idx, btn);
                });
            });

            // AI 改写按钮
            reviewInline.querySelectorAll('.review-ai-btn').forEach(function(btn) {
                btn.addEventListener('click', function() {
                    var idx = parseInt(btn.dataset.index);
                    self._onAIRewriteInline(idx, draft, btn);
                });
            });

            // 删除按钮
            reviewInline.querySelectorAll('.review-delete-btn').forEach(function(btn) {
                btn.addEventListener('click', function() {
                    var idx = parseInt(btn.dataset.index);
                    self._onDeleteParagraphInline(idx, draft);
                });
            });

            // 重写整章
            var rewriteBtn = document.getElementById('reviewRewriteAllBtn');
            if (rewriteBtn) {
                rewriteBtn.addEventListener('click', function() { self._onRewriteFullChapter(); });
            }

            // 认可并继续
            var approveBtn = document.getElementById('reviewApproveBtn');
            if (approveBtn) {
                approveBtn.addEventListener('click', function() { self._onApproveChapter(); });
            }
        }

        // ==== 切换编辑模式 ====
        _toggleEdit(index, btn) {
            var card = this.app.elements.reviewInline.querySelector('.review-paragraph-card[data-index="' + index + '"]');
            if (!card) return;

            var contentDiv = card.querySelector('.review-para-content');
            var editArea = card.querySelector('.review-para-edit-area');

            if (editArea.style.display === 'none') {
                // 进入编辑模式
                editArea.style.display = '';
                contentDiv.style.display = 'none';
                editArea.value = this.app.state.currentDraft.paragraphs[index];
                btn.innerHTML = '<i class="fas fa-save"></i> 保存';
                editArea.focus();
            } else {
                // 保存编辑
                var newContent = editArea.value.trim();
                if (newContent) {
                    this.app.state.currentDraft.paragraphs[index] = newContent;
                    contentDiv.textContent = newContent;
                    // 更新字数
                    var wordsEl = card.querySelector('.review-para-words');
                    if (wordsEl) wordsEl.textContent = this.app.countWords(newContent) + ' 字';
                }
                editArea.style.display = 'none';
                contentDiv.style.display = '';
                btn.innerHTML = '<i class="fas fa-edit"></i> 编辑';
            }
        }

        // ==== AI 改写段落（内联版） ====
        async _onAIRewriteInline(index, draft, aiBtn) {
            var self = this;
            var card = this.app.elements.reviewInline.querySelector('.review-paragraph-card[data-index="' + index + '"]');
            if (!card) return;

            var originalContent = draft.paragraphs[index];

            // 收集用户修改意见
            var opinionInput = card.querySelector('.opinion-input');
            var userOpinion = (opinionInput ? opinionInput.value : '').trim();

            // 收集选中的审核问题和建议
            var selectedIssues = [];
            var selectedSuggestions = [];
            var issuesArea = card.querySelector('.review-para-issues');
            if (issuesArea) {
                issuesArea.querySelectorAll('input[data-issue-index]:checked').forEach(function(cb) {
                    var idx = parseInt(cb.dataset.issueIndex);
                    var review = draft.reviewResults.find(function(r) { return r.index === index + 1; });
                    if (review && review.issues[idx]) selectedIssues.push(review.issues[idx]);
                });
                issuesArea.querySelectorAll('input[data-suggestion-index]:checked').forEach(function(cb) {
                    var idx = parseInt(cb.dataset.suggestionIndex);
                    var review = draft.reviewResults.find(function(r) { return r.index === index + 1; });
                    if (review && review.suggestions[idx]) selectedSuggestions.push(review.suggestions[idx]);
                });
            }

            if (!userOpinion && selectedIssues.length === 0 && selectedSuggestions.length === 0) {
                this.app.showToast('请输入修改意见或勾选审核问题/建议');
                return;
            }

            aiBtn.disabled = true;
            var originalHtml = aiBtn.innerHTML;
            aiBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 改写中...';

            try {
                var result = await this.app.reviewer.rewriteParagraph(
                    index + 1,
                    originalContent,
                    selectedIssues.join('\n'),
                    selectedSuggestions.join('\n'),
                    userOpinion
                );

                var newContent = result.content || originalContent;
                draft.paragraphs[index] = newContent;

                // 更新 DOM
                var contentDiv = card.querySelector('.review-para-content');
                var editArea = card.querySelector('.review-para-edit-area');
                if (contentDiv) contentDiv.textContent = newContent;
                if (editArea) editArea.value = newContent;

                var wordsEl = card.querySelector('.review-para-words');
                if (wordsEl) wordsEl.textContent = this.app.countWords(newContent) + ' 字';

                // 清空修改意见
                if (opinionInput) opinionInput.value = '';

                this.app.showToast('段落 ' + (index + 1) + ' 已改写');
            } catch (e) {
                this.app.showToast('AI 改写失败: ' + e.message);
            } finally {
                aiBtn.disabled = false;
                aiBtn.innerHTML = originalHtml;
            }
        }

        // ==== 删除段落（内联版） ====
        async _onDeleteParagraphInline(index, draft) {
            if (!confirm('确定删除第 ' + (index + 1) + ' 段？')) return;

            var prevParagraph = index > 0 ? draft.paragraphs[index - 1] : null;
            var nextParagraph = index < draft.paragraphs.length - 1 ? draft.paragraphs[index + 1] : null;

            // 从数组中移除
            draft.paragraphs.splice(index, 1);
            // 移除审核结果
            draft.reviewResults = draft.reviewResults.filter(function(r) { return r.index !== index + 1; });
            // 重新编号审核结果
            draft.reviewResults.forEach(function(r, i) { r.index = i + 1; });

            // 重新渲染
            this.showHumanApproval(draft);

            // 询问是否需要生成过渡段
            if (prevParagraph && nextParagraph) {
                var needTransition = confirm('删除段落后上下文可能不连贯，是否需要 AI 生成过渡段？');
                if (needTransition) {
                    try {
                        this.app.showToast('正在生成过渡段...');
                        var result = await this.app.reviewer.generateTransition(
                            index, prevParagraph, index + 1, nextParagraph
                        );
                        if (result.content) {
                            draft.paragraphs.splice(index, 0, result.content);
                            this.showHumanApproval(draft);
                            this.app.showToast('过渡段已生成');
                        }
                    } catch (e) {
                        this.app.showToast('过渡段生成失败: ' + e.message);
                    }
                }
            }
        }

        // ==== 认可并继续（审核通过） ====
        async _onApproveChapter() {
            var self = this;
            var draft = this.app.state.currentDraft;
            if (!draft) return;

            // 收集最终段落内容（可能被编辑过）
            this._collectFinalParagraphsInline(draft);

            // 隐藏审核面板
            this.app.elements.reviewInline.style.display = 'none';
            this.app.elements.reviewInline.innerHTML = '';
            this.app.elements.reviewEmpty.style.display = '';
            this.app.showToast('正在生成摘要...');

            try {
                // 1. 生成摘要 + 下章推理
                await this._generateSummaryAndNext(draft);

                // 2. 保存章节
                var state = this.app.state;
                var chapterContent = draft.paragraphs.join('\n\n');
                var chapter = {
                    index: draft.chapterIndex,
                    title: '第 ' + draft.chapterIndex + ' 章',
                    outline: draft.chapterOutline,
                    direction: draft.direction || draft.chapterOutline,
                    content: chapterContent,
                    summary: draft.summary || '',
                    status: CHAPTER_STATUS.APPROVED,
                    tokens: 0,
                    tokenBreakdown: {},
                    approvedAt: new Date().toISOString(),
                    editedAt: null,
                    revisions: []
                };

                // 计算 tokenBreakdown
                var chapterCalls = state.callLog.filter(function(c) {
                    return c.chapterIndex === draft.chapterIndex - 1;
                });
                chapter.tokens = chapterCalls.reduce(function(sum, c) { return sum + (c.totalTokens || 0); }, 0);
                chapterCalls.forEach(function(c) {
                    var step = c.step || 'other';
                    chapter.tokenBreakdown[step] = (chapter.tokenBreakdown[step] || 0) + (c.totalTokens || 0);
                });

                state.chapters.push(chapter);
                state.chapterIndex = state.chapters.length;

                var nextDirection = draft.nextDirection || '';
                var nextCriteria = draft.nextCriteria || '';

                state.currentDraft = null;

                state.callLog = state.callLog.filter(function(c) {
                    return c.chapterIndex !== draft.chapterIndex - 1;
                });

                await this.app.saveState();

                // 更新 UI
                this.app.currentChapterView = state.chapters.length - 1;
                this.app.renderChapterView();
                this.app.updateChapterNav();
                this.app.renderTokenBoard();
                this.app.setPhase(PHASE.IDLE);
                await this.app.saveState();

                // 如果有下章走向，显示待确认卡片
                if (nextDirection) {
                    this.app.elements.nextDirectionPreview.value = nextDirection;
                    this.app.elements.nextCriteriaPreview.value = nextCriteria;
                    this.app.elements.nextDirectionCard.style.display = '';
                }

                this.app.showToast('章节 ' + chapter.index + ' 审核通过，摘要已生成');

            } catch (e) {
                this.app.showToast('摘要生成失败: ' + e.message);
                this.app.setPhase(PHASE.IDLE);
            }
        }

        // ==== 收集最终段落内容（内联版） ====
        _collectFinalParagraphsInline(draft) {
            var reviewInline = this.app.elements.reviewInline;
            if (!reviewInline) return;
            var editAreas = reviewInline.querySelectorAll('.review-para-edit-area');
            editAreas.forEach(function(area) {
                var card = area.closest('.review-paragraph-card');
                if (!card) return;
                var idx = parseInt(card.dataset.index);
                if (area.value.trim()) {
                    draft.paragraphs[idx] = area.value.trim();
                }
            });
        }

        // ==== 生成摘要 + 下章推理 ====
        async _generateSummaryAndNext(draft) {
            var state = this.app.state;
            var chapterContent = draft.paragraphs.join('\n\n');
            var prevSummary = this._getPrevSummary(draft.chapterIndex);

            var prompt = await global.PromptLoader.loadAndFill('generate_summary', {
                outline: state.outline || '',
                chapterOutline: draft.chapterOutline || '',
                chapterContent: chapterContent,
                prevSummary: prevSummary || '（无）',
                chapterIndex: draft.chapterIndex
            });

            var messages = [
                { role: 'system', content: '你是一位专业的小说编辑。请严格按照 JSON 格式返回章节摘要和下一章走向推断。不要添加任何额外文字。' },
                { role: 'user', content: prompt }
            ];

            var result = await this.app.config.callChat(messages, {
                temperature: 0.5,
                maxTokens: 2048
            });

            this.app.config.trackToken({
                step: 'generate_summary',
                model: this.app.config.getConfig().name,
                inputTokens: result.inputTokens,
                outputTokens: result.outputTokens,
                totalTokens: result.totalTokens
            });

            var parsed = this._parseSummaryResult(result.content);
            draft.summary = parsed.summary || '';

            var nextChapterIdx = draft.chapterIndex;
            var hasNextOutline = state.chapterOutlines && state.chapterOutlines[nextChapterIdx];

            if (!hasNextOutline && parsed.nextDirection) {
                draft.nextDirection = parsed.nextDirection;
                draft.nextCriteria = parsed.nextCriteria || '';
            } else if (hasNextOutline) {
                draft.nextDirection = '';
                draft.nextCriteria = '';
            } else {
                draft.nextDirection = parsed.nextDirection || '';
                draft.nextCriteria = parsed.nextCriteria || '';
            }
        }

        // ==== 获取前几章摘要 ====
        _getPrevSummary(chapterIndex) {
            var state = this.app.state;
            var summaries = [];
            var start = Math.max(0, chapterIndex - 3);
            for (var i = start; i < chapterIndex; i++) {
                if (state.chapters[i] && state.chapters[i].summary) {
                    summaries.push('第 ' + (i + 1) + ' 章：' + state.chapters[i].summary);
                }
            }
            return summaries.join('\n');
        }

        // ==== 解析摘要结果 JSON ====
        _parseSummaryResult(text) {
            if (!text) return {};
            var codeBlockMatch = text.match(/```(?:json)?\s*\n?([\s\S]*?)\n?```/);
            var jsonStr = codeBlockMatch ? codeBlockMatch[1].trim() : text.trim();
            try {
                var parsed = JSON.parse(jsonStr);
                return {
                    summary: parsed.summary || '',
                    nextDirection: parsed.nextDirection || '',
                    nextCriteria: parsed.nextCriteria || ''
                };
            } catch (e) {
                return {
                    summary: text.substring(0, 500),
                    nextDirection: '',
                    nextCriteria: ''
                };
            }
        }

        // ==== 重写整章 ====
        _onRewriteFullChapter() {
            if (!confirm('确定要重写整章？当前草稿将被丢弃，从头开始生成。')) return;

            this.app.elements.reviewInline.style.display = 'none';
            this.app.elements.reviewInline.innerHTML = '';
            this.app.elements.reviewEmpty.style.display = '';
            var state = this.app.state;
            state.currentDraft = null;
            state.chapterIndex = Math.max(0, state.chapterIndex);

            this.app.setPhase(PHASE.IDLE);
            if (this.app.workflow) {
                this.app.workflow.startChapter();
            }
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
    global.UI = UI;
})(typeof window !== 'undefined' ? window : this);