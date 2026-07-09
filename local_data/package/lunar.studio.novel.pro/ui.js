/**
 * UI 模块
 * 负责 DOM 渲染、模态框、Toast、段落选择交互、知识点面板双 tab、配置开关等。
 *
 * 设计契约：
 *   - 沿用 lunar.studio.novel 的玻璃拟态风格 + Font Awesome 图标
 *   - 知识点双 tab（快速单条 + 批量长文切片预览，grilling 问题 14）
 *   - 步骤 8 段落级选择交互（grilling 问题 6/7）
 *   - 步骤 9 下一章走向确认弹窗（grilling 问题 10）
 *   - 中断恢复提示（grilling 问题 12）
 */

(function (global) {
    'use strict';

    /**
     * UI 类
     */
    class UI {
        /**
         * @param {object} elements DOM 元素引用集合
         * @param {object} callbacks 回调集合
         */
        constructor(elements, callbacks) {
            this.elements = elements || {};
            this.callbacks = callbacks || {};
            this._modalStack = [];
        }

        /** ==================== Toast ==================== */

        /**
         * 显示 Toast 提示。
         *
         * @param {string} message 提示消息
         * @param {string} level 级别 'info' | 'success' | 'warning' | 'error'
         * @param {number} duration 持续时间（毫秒）
         */
        toast(message, level, duration) {
            const lvl = level || 'info';
            const dur = duration || 3000;
            let toastEl = document.getElementById('toast');
            if (!toastEl) {
                toastEl = document.createElement('div');
                toastEl.id = 'toast';
                toastEl.className = 'toast';
                document.body.appendChild(toastEl);
            }
            toastEl.textContent = message;
            toastEl.className = 'toast visible ' + lvl;
            clearTimeout(this._toastTimer);
            this._toastTimer = setTimeout(function () {
                toastEl.className = 'toast ' + lvl;
            }, dur);
        }

        /** ==================== 确认弹窗 ==================== */

        /**
         * 显示确认弹窗（返回 Promise<boolean>）。
         *
         * @param {string} title 标题
         * @param {string} message 消息（支持 \n 换行）
         * @returns {Promise<boolean>}
         */
        confirm(title, message) {
            const self = this;
            return new Promise(function (resolve) {
                const overlay = self._createOverlay();
                const panel = self._createGlassPanel();

                const titleEl = document.createElement('h3');
                titleEl.textContent = title;
                titleEl.className = 'modal-title';
                panel.appendChild(titleEl);

                const msgEl = document.createElement('pre');
                msgEl.textContent = message;
                msgEl.className = 'modal-message';
                panel.appendChild(msgEl);

                const btnGroup = document.createElement('div');
                btnGroup.className = 'modal-btn-group';

                const cancelBtn = document.createElement('button');
                cancelBtn.className = 'btn-glass btn-glass-danger';
                cancelBtn.innerHTML = '<i class="fas fa-times"></i> 取消';
                cancelBtn.onclick = function () {
                    self._closeModal(overlay);
                    resolve(false);
                };

                const okBtn = document.createElement('button');
                okBtn.className = 'btn-glass btn-glass-primary';
                okBtn.innerHTML = '<i class="fas fa-check"></i> 确认';
                okBtn.onclick = function () {
                    self._closeModal(overlay);
                    resolve(true);
                };

                btnGroup.appendChild(cancelBtn);
                btnGroup.appendChild(okBtn);
                panel.appendChild(btnGroup);
                overlay.appendChild(panel);
                document.body.appendChild(overlay);
                self._modalStack.push(overlay);

                // 点击遮罩关闭 = 取消
                overlay.addEventListener('click', function (e) {
                    if (e.target === overlay) {
                        self._closeModal(overlay);
                        resolve(false);
                    }
                });
            });
        }

        /** ==================== 模态框基础设施 ==================== */

        _createOverlay() {
            const overlay = document.createElement('div');
            overlay.className = 'modal-overlay';
            return overlay;
        }

        _createGlassPanel() {
            const panel = document.createElement('div');
            panel.className = 'modal-panel glass-panel';
            return panel;
        }

        _closeModal(overlay) {
            if (overlay && overlay.parentNode) {
                overlay.parentNode.removeChild(overlay);
            }
            const idx = this._modalStack.indexOf(overlay);
            if (idx !== -1) this._modalStack.splice(idx, 1);
        }

        closeAllModals() {
            while (this._modalStack.length > 0) {
                const overlay = this._modalStack.pop();
                if (overlay && overlay.parentNode) {
                    overlay.parentNode.removeChild(overlay);
                }
            }
        }

        /** ==================== 步骤 8: 人工审核弹窗 ==================== */

        /**
         * 显示人工审核弹窗，返回用户操作结果。
         *
         * @param {object} chapterData {chapterIndex, paragraphs, reviewResults, formatLog, revisions}
         * @returns {Promise<object>} {action: 'approve'|'rewrite_all'|'edit_paragraphs', opinion, paragraphOpinions}
         */
        showHumanApproval(chapterData) {
            const self = this;
            return new Promise(function (resolve) {
                const overlay = self._createOverlay();
                const panel = self._createGlassPanel();
                panel.style.maxWidth = '900px';
                panel.style.maxHeight = '85vh';
                panel.style.overflow = 'auto';

                // 标题
                const title = document.createElement('h3');
                title.textContent = '步骤 8：人工审核 - 第 ' + (chapterData.chapterIndex + 1) + ' 章';
                title.className = 'modal-title';
                panel.appendChild(title);

                // 评审结果摘要
                const summary = self._renderReviewSummary(chapterData.reviewResults, chapterData.formatLog);
                panel.appendChild(summary);

                // 段落列表（可点击选中）
                const paraList = document.createElement('div');
                paraList.className = 'paragraph-list';
                const selectedParagraphs = new Set();

                chapterData.paragraphs.forEach(function (para, idx) {
                    const paraEl = document.createElement('div');
                    paraEl.className = 'paragraph-item';
                    paraEl.dataset.index = idx;

                    const review = chapterData.reviewResults.paragraphs[idx] || {};
                    const scoreClass = review.score >= 75 ? 'score-good' : (review.score >= 60 ? 'score-pass' : 'score-fail');

                    const header = document.createElement('div');
                    header.className = 'paragraph-header';
                    header.innerHTML =
                        '<span class="paragraph-index">段落 ' + (idx + 1) + '</span>' +
                        '<span class="paragraph-score ' + scoreClass + '">评分: ' + (review.score !== undefined ? review.score : 'N/A') + '</span>' +
                        '<span class="paragraph-words">' + para.content.length + ' 字</span>';
                    paraEl.appendChild(header);

                    const content = document.createElement('div');
                    content.className = 'paragraph-content';
                    content.textContent = para.content;
                    paraEl.appendChild(content);

                    // 评审问题展开
                    if (review.issues && review.issues.length > 0) {
                        const issuesEl = document.createElement('div');
                        issuesEl.className = 'paragraph-issues';
                        issuesEl.style.display = 'none';
                        review.issues.forEach(function (iss) {
                            const issEl = document.createElement('div');
                            issEl.className = 'issue-item severity-' + iss.severity;
                            issEl.innerHTML =
                                '<i class="fas fa-' + (iss.severity === 'high' ? 'exclamation-circle' : (iss.severity === 'medium' ? 'exclamation-triangle' : 'info-circle')) + '"></i>' +
                                '<span class="issue-location">' + iss.location + '</span>' +
                                '<span class="issue-text">' + iss.issue + '</span>' +
                                '<span class="issue-suggestion">建议: ' + iss.suggestion + '</span>';
                            issuesEl.appendChild(issEl);
                        });
                        paraEl.appendChild(issuesEl);

                        const toggleBtn = document.createElement('button');
                        toggleBtn.className = 'btn-text';
                        toggleBtn.innerHTML = '<i class="fas fa-chevron-down"></i> 显示评审问题';
                        toggleBtn.onclick = function (e) {
                            e.stopPropagation();
                            if (issuesEl.style.display === 'none') {
                                issuesEl.style.display = 'block';
                                toggleBtn.innerHTML = '<i class="fas fa-chevron-up"></i> 隐藏评审问题';
                            } else {
                                issuesEl.style.display = 'none';
                                toggleBtn.innerHTML = '<i class="fas fa-chevron-down"></i> 显示评审问题';
                            }
                        };
                        paraEl.appendChild(toggleBtn);
                    }

                    // 改动记录（如有）
                    if (para.revisions && para.revisions.length > 0) {
                        const revInfo = document.createElement('div');
                        revInfo.className = 'revision-info';
                        revInfo.innerHTML = '<i class="fas fa-history"></i> 已重写 ' + para.revisions.length + ' 次';
                        paraEl.appendChild(revInfo);
                    }

                    // 点击切换选中状态
                    paraEl.onclick = function () {
                        if (selectedParagraphs.has(idx)) {
                            selectedParagraphs.delete(idx);
                            paraEl.classList.remove('selected');
                        } else {
                            selectedParagraphs.add(idx);
                            paraEl.classList.add('selected');
                        }
                    };

                    paraList.appendChild(paraEl);
                });
                panel.appendChild(paraList);

                // 段落改进意见输入区（仅当选中段落时显示）
                const opinionArea = document.createElement('div');
                opinionArea.className = 'opinion-area';
                opinionArea.style.display = 'none';
                opinionArea.innerHTML =
                    '<label>对选中段落的改进意见：</label>' +
                    '<textarea class="opinion-input" rows="3" placeholder="输入对该段落的改进意见..."></textarea>';
                panel.appendChild(opinionArea);

                // 选中变化时显示/隐藏意见区
                const updateOpinionArea = function () {
                    opinionArea.style.display = selectedParagraphs.size > 0 ? 'block' : 'none';
                };
                paraList.addEventListener('click', updateOpinionArea);

                // 整章重写意见
                const rewriteAllArea = document.createElement('div');
                rewriteAllArea.className = 'opinion-area rewrite-all-area';
                rewriteAllArea.style.display = 'none';
                rewriteAllArea.innerHTML =
                    '<label>整章重写的指导意见：</label>' +
                    '<textarea class="rewrite-all-input" rows="3" placeholder="输入整章重写的指导意见..."></textarea>';
                panel.appendChild(rewriteAllArea);

                // 按钮组
                const btnGroup = document.createElement('div');
                btnGroup.className = 'modal-btn-group';

                const approveBtn = document.createElement('button');
                approveBtn.className = 'btn-glass btn-glass-primary';
                approveBtn.innerHTML = '<i class="fas fa-check"></i> 认可并生成摘要';
                approveBtn.onclick = function () {
                    self._closeModal(overlay);
                    resolve({ action: 'approve' });
                };

                const editBtn = document.createElement('button');
                editBtn.className = 'btn-glass';
                editBtn.innerHTML = '<i class="fas fa-edit"></i> 提交段落改进意见';
                editBtn.onclick = function () {
                    if (selectedParagraphs.size === 0) {
                        self.toast('请先选择要改进的段落', 'warning');
                        return;
                    }
                    const opinion = opinionArea.querySelector('.opinion-input').value.trim();
                    if (!opinion) {
                        self.toast('请输入改进意见', 'warning');
                        return;
                    }
                    const paragraphOpinions = Array.from(selectedParagraphs).map(function (idx) {
                        return { index: idx, opinion: opinion };
                    });
                    // 不关闭弹窗，UI 类不直接处理流程；通过 resolve 通知 workflow
                    self._closeModal(overlay);
                    resolve({ action: 'edit_paragraphs', paragraphOpinions: paragraphOpinions });
                };

                const rewriteBtn = document.createElement('button');
                rewriteBtn.className = 'btn-glass btn-glass-danger';
                rewriteBtn.innerHTML = '<i class="fas fa-redo"></i> 重写整章';
                rewriteBtn.onclick = function () {
                    rewriteAllArea.style.display = 'block';
                    const opinion = rewriteAllArea.querySelector('.rewrite-all-input').value.trim();
                    self._closeModal(overlay);
                    resolve({ action: 'rewrite_all', opinion: opinion });
                };

                btnGroup.appendChild(approveBtn);
                btnGroup.appendChild(editBtn);
                btnGroup.appendChild(rewriteBtn);
                panel.appendChild(btnGroup);

                overlay.appendChild(panel);
                document.body.appendChild(overlay);
                self._modalStack.push(overlay);
            });
        }

        /**
         * 渲染评审结果摘要。
         */
        _renderReviewSummary(reviewResults, formatLog) {
            const summary = document.createElement('div');
            summary.className = 'review-summary';

            const co = reviewResults.chapterOutline || {};
            const to = reviewResults.totalOutline || {};
            const paras = reviewResults.paragraphs || [];

            const passed = paras.filter(function (r) { return r.score >= 60; }).length;
            const failed = paras.length - passed;

            summary.innerHTML =
                '<div class="review-item"><strong>步骤 5 章节大纲评审：</strong> ' +
                (co.pass ? '通过' : '未通过') + ' (评分: ' + (co.score || 'N/A') + ')</div>' +
                '<div class="review-item"><strong>步骤 6 总体大纲评审：</strong> ' +
                (to.pass ? '通过' : '未通过') + ' (评分: ' + (to.score || 'N/A') + ')</div>' +
                '<div class="review-item"><strong>步骤 7 段落评审：</strong> 共 ' + paras.length + ' 段，' +
                '通过 ' + passed + ' 段，未通过 ' + failed + ' 段</div>' +
                (formatLog && formatLog.length > 0 ?
                    '<div class="review-item"><strong>格式校验日志：</strong> ' + formatLog.length + ' 处清理</div>' : '');

            return summary;
        }

        /** ==================== 步骤 9: 下一章走向确认弹窗 ==================== */

        /**
         * 显示下一章走向确认弹窗。
         *
         * @param {string} nextDirection AI 建议的下一章走向
         * @param {string} nextCriteria AI 建议的下一章评判标准
         * @returns {Promise<object>} {accepted: bool, direction, criteria}
         */
        showNextDirectionConfirm(nextDirection, nextCriteria) {
            const self = this;
            return new Promise(function (resolve) {
                const overlay = self._createOverlay();
                const panel = self._createGlassPanel();
                panel.style.maxWidth = '700px';

                const title = document.createElement('h3');
                title.textContent = '步骤 9 完成：是否采用 AI 建议的下一章走向？';
                title.className = 'modal-title';
                panel.appendChild(title);

                const desc = document.createElement('p');
                desc.textContent = 'AI 已生成当前章节摘要并写入向量库。下面是建议的下一章走向与评判标准，可直接编辑后采用，或拒绝后自行输入。';
                desc.className = 'modal-desc';
                panel.appendChild(desc);

                const dirLabel = document.createElement('label');
                dirLabel.textContent = '下一章走向：';
                dirLabel.className = 'form-label';
                panel.appendChild(dirLabel);

                const dirInput = document.createElement('textarea');
                dirInput.className = 'form-textarea';
                dirInput.rows = 6;
                dirInput.value = nextDirection || '';
                panel.appendChild(dirInput);

                const criLabel = document.createElement('label');
                criLabel.textContent = '下一章评判标准：';
                criLabel.className = 'form-label';
                panel.appendChild(criLabel);

                const criInput = document.createElement('textarea');
                criInput.className = 'form-textarea';
                criInput.rows = 4;
                criInput.value = nextCriteria || '';
                panel.appendChild(criInput);

                const btnGroup = document.createElement('div');
                btnGroup.className = 'modal-btn-group';

                const rejectBtn = document.createElement('button');
                rejectBtn.className = 'btn-glass btn-glass-danger';
                rejectBtn.innerHTML = '<i class="fas fa-times"></i> 拒绝建议';
                rejectBtn.onclick = function () {
                    self._closeModal(overlay);
                    resolve({ accepted: false });
                };

                const acceptBtn = document.createElement('button');
                acceptBtn.className = 'btn-glass btn-glass-primary';
                acceptBtn.innerHTML = '<i class="fas fa-check"></i> 采用并开始下一章';
                acceptBtn.onclick = function () {
                    self._closeModal(overlay);
                    resolve({
                        accepted: true,
                        direction: dirInput.value.trim(),
                        criteria: criInput.value.trim()
                    });
                };

                btnGroup.appendChild(rejectBtn);
                btnGroup.appendChild(acceptBtn);
                panel.appendChild(btnGroup);

                overlay.appendChild(panel);
                document.body.appendChild(overlay);
                self._modalStack.push(overlay);
            });
        }

        /** ==================== 中断恢复提示 ==================== */

        /**
         * 显示中断恢复提示。
         *
         * @param {object} interruptContext 中断上下文
         * @returns {Promise<boolean>} 是否恢复
         */
        showInterruptResume(interruptContext) {
            const self = this;
            return new Promise(function (resolve) {
                const overlay = self._createOverlay();
                const panel = self._createGlassPanel();

                const title = document.createElement('h3');
                title.textContent = '检测到未完成的章节生成';
                title.className = 'modal-title';
                panel.appendChild(title);

                const stepNames = {
                    1: '步骤 1：大纲处理',
                    2: '步骤 2：段落大纲预构建',
                    3: '步骤 3：段落内容生成',
                    4: '步骤 4：格式校验',
                    5: '步骤 5：章节大纲评审',
                    6: '步骤 6：总体大纲评审',
                    7: '步骤 7：段落评审'
                };

                const msg = document.createElement('p');
                msg.className = 'modal-desc';
                let text = '上次中断于：' + (stepNames[interruptContext.step] || '步骤 ' + interruptContext.step);
                if (interruptContext.paragraphIndex) {
                    text += '（段落 ' + (interruptContext.paragraphIndex + 1) + '）';
                }
                if (interruptContext.attempt) {
                    text += '（第 ' + interruptContext.attempt + ' 次重写）';
                }
                msg.textContent = text;
                panel.appendChild(msg);

                if (!interruptContext.canResume) {
                    const warn = document.createElement('p');
                    warn.className = 'modal-desc warning';
                    warn.textContent = '该中断点不可恢复（可能发生在整章重写过程中），将回退到本章起点。';
                    panel.appendChild(warn);
                }

                const btnGroup = document.createElement('div');
                btnGroup.className = 'modal-btn-group';

                const discardBtn = document.createElement('button');
                discardBtn.className = 'btn-glass btn-glass-danger';
                discardBtn.innerHTML = '<i class="fas fa-trash"></i> 丢弃草稿';
                discardBtn.onclick = function () {
                    self._closeModal(overlay);
                    resolve(false);
                };

                const resumeBtn = document.createElement('button');
                resumeBtn.className = 'btn-glass btn-glass-primary';
                resumeBtn.innerHTML = '<i class="fas fa-play"></i> 恢复生成';
                resumeBtn.disabled = !interruptContext.canResume;
                resumeBtn.onclick = function () {
                    self._closeModal(overlay);
                    resolve(true);
                };

                btnGroup.appendChild(discardBtn);
                btnGroup.appendChild(resumeBtn);
                panel.appendChild(btnGroup);

                overlay.appendChild(panel);
                document.body.appendChild(overlay);
                self._modalStack.push(overlay);
            });
        }

        /** ==================== 知识点面板 ==================== */

        /**
         * 渲染知识点面板（双 tab）。
         *
         * @param {object} container 知识点面板容器 DOM 元素
         * @param {object[]} items 知识点列表（不含 embedding）
         * @param {object} callbacks {onAddSingle, onAddBulk, onDelete}
         */
        renderKnowledgePanel(container, items, callbacks) {
            const self = this;
            container.innerHTML = '';

            // Tab 切换
            const tabBar = document.createElement('div');
            tabBar.className = 'tab-bar';
            const tabQuick = document.createElement('button');
            tabQuick.className = 'tab-btn active';
            tabQuick.textContent = '快速单条';
            const tabBulk = document.createElement('button');
            tabBulk.className = 'tab-btn';
            tabBulk.textContent = '批量长文';

            tabBar.appendChild(tabQuick);
            tabBar.appendChild(tabBulk);
            container.appendChild(tabBar);

            // 快速单条面板
            const quickPanel = document.createElement('div');
            quickPanel.className = 'tab-panel active';

            const quickInput = document.createElement('textarea');
            quickInput.className = 'form-textarea';
            quickInput.rows = 3;
            quickInput.placeholder = '输入单条知识点...';
            quickPanel.appendChild(quickInput);

            const quickPathRow = document.createElement('div');
            quickPathRow.className = 'form-row';
            quickPathRow.innerHTML =
                '<input type="text" class="form-input" placeholder="路径（可选，如：人物设定/子幽/背景" style="flex:1">';
            const quickPathInput = quickPathRow.querySelector('input');
            quickPanel.appendChild(quickPathRow);

            const quickAddBtn = document.createElement('button');
            quickAddBtn.className = 'btn-glass btn-glass-primary';
            quickAddBtn.innerHTML = '<i class="fas fa-plus"></i> 新增';
            quickAddBtn.onclick = async function () {
                const content = quickInput.value.trim();
                if (!content) {
                    self.toast('请输入知识点内容', 'warning');
                    return;
                }
                const path = quickPathInput.value.trim();
                if (callbacks.onAddSingle) {
                    await callbacks.onAddSingle(content, path);
                }
                quickInput.value = '';
                quickPathInput.value = '';
            };
            quickPanel.appendChild(quickAddBtn);
            container.appendChild(quickPanel);

            // 批量长文面板
            const bulkPanel = document.createElement('div');
            bulkPanel.className = 'tab-panel';
            bulkPanel.style.display = 'none';

            const bulkInput = document.createElement('textarea');
            bulkInput.className = 'form-textarea';
            bulkInput.rows = 10;
            bulkInput.placeholder = '粘贴 Markdown 或长文本...';
            bulkPanel.appendChild(bulkInput);

            const bulkPreviewBtn = document.createElement('button');
            bulkPreviewBtn.className = 'btn-glass';
            bulkPreviewBtn.innerHTML = '<i class="fas fa-cut"></i> 切片预览';
            bulkPreviewBtn.onclick = function () {
                const text = bulkInput.value.trim();
                if (!text) {
                    self.toast('请先输入文本', 'warning');
                    return;
                }
                const chunks = global.splitTextToStrings(text, { idealLen: 800 });
                self._renderBulkPreview(bulkPanel, chunks, async function (selectedChunks) {
                    if (callbacks.onAddBulk) {
                        await callbacks.onAddBulk(selectedChunks);
                    }
                    bulkInput.value = '';
                    // 清理预览
                    const preview = bulkPanel.querySelector('.bulk-preview');
                    if (preview) preview.remove();
                });
            };
            bulkPanel.appendChild(bulkPreviewBtn);
            container.appendChild(bulkPanel);

            // Tab 切换逻辑
            tabQuick.onclick = function () {
                tabQuick.classList.add('active');
                tabBulk.classList.remove('active');
                quickPanel.style.display = 'block';
                bulkPanel.style.display = 'none';
            };
            tabBulk.onclick = function () {
                tabBulk.classList.add('active');
                tabQuick.classList.remove('active');
                bulkPanel.style.display = 'block';
                quickPanel.style.display = 'none';
            };

            // 知识点列表
            const listSection = document.createElement('div');
            listSection.className = 'knowledge-list-section';
            const listTitle = document.createElement('div');
            listTitle.className = 'list-title';
            listTitle.innerHTML = '<i class="fas fa-database"></i> 知识点列表 <span class="count">(' + items.length + ')</span>';
            listSection.appendChild(listTitle);

            const list = document.createElement('div');
            list.className = 'knowledge-list';
            if (items.length === 0) {
                const empty = document.createElement('div');
                empty.className = 'empty-state';
                empty.innerHTML = '<i class="fas fa-inbox"></i><p>暂无知识点</p>';
                list.appendChild(empty);
            } else {
                items.forEach(function (item) {
                    const itemEl = document.createElement('div');
                    itemEl.className = 'knowledge-item';
                    const preview = item.content.slice(0, 50) + (item.content.length > 50 ? '...' : '');
                    itemEl.innerHTML =
                        '<div class="item-path">' + (item.path || '（无路径）') + '</div>' +
                        '<div class="item-content">' + preview + '</div>' +
                        '<div class="item-meta">' + item.content.length + ' 字</div>';
                    const delBtn = document.createElement('button');
                    delBtn.className = 'btn-icon btn-icon-danger';
                    delBtn.innerHTML = '<i class="fas fa-trash"></i>';
                    delBtn.onclick = async function (e) {
                        e.stopPropagation();
                        if (callbacks.onDelete) {
                            await callbacks.onDelete(item.id);
                        }
                    };
                    itemEl.appendChild(delBtn);
                    list.appendChild(itemEl);
                });
            }
            listSection.appendChild(list);
            container.appendChild(listSection);
        }

        /**
         * 渲染批量切片预览。
         */
        _renderBulkPreview(panel, chunks, onConfirm) {
            const self = this;
            // 移除已有预览
            const oldPreview = panel.querySelector('.bulk-preview');
            if (oldPreview) oldPreview.remove();

            const preview = document.createElement('div');
            preview.className = 'bulk-preview';

            const header = document.createElement('div');
            header.className = 'preview-header';
            header.innerHTML = '<i class="fas fa-list"></i> 共切出 <strong>' + chunks.length + '</strong> 段';
            preview.appendChild(header);

            const selected = new Set();
            chunks.forEach(function (_, idx) { selected.add(idx); });

            const list = document.createElement('div');
            list.className = 'preview-list';
            chunks.forEach(function (chunk, idx) {
                const item = document.createElement('div');
                item.className = 'preview-item selected';

                // 提取 path（格式 *标题> 路径*）
                const pathMatch = chunk.match(/^\*标题>\s*([^*]+)\*/);
                const path = pathMatch ? pathMatch[1].trim() : '';
                const content = pathMatch ? chunk.slice(pathMatch[0].length).trim() : chunk;

                item.innerHTML =
                    '<div class="item-path">' + (path || '（无路径）') + '</div>' +
                    '<div class="item-content">' + content.slice(0, 80) + (content.length > 80 ? '...' : '') + '</div>' +
                    '<div class="item-meta">' + chunk.length + ' 字</div>';

                item.onclick = function () {
                    if (selected.has(idx)) {
                        selected.delete(idx);
                        item.classList.remove('selected');
                    } else {
                        selected.add(idx);
                        item.classList.add('selected');
                    }
                };
                list.appendChild(item);
            });
            preview.appendChild(list);

            const btnGroup = document.createElement('div');
            btnGroup.className = 'preview-btn-group';

            const cancelBtn = document.createElement('button');
            cancelBtn.className = 'btn-glass btn-glass-danger';
            cancelBtn.innerHTML = '<i class="fas fa-times"></i> 取消';
            cancelBtn.onclick = function () {
                preview.remove();
            };

            const confirmBtn = document.createElement('button');
            confirmBtn.className = 'btn-glass btn-glass-primary';
            confirmBtn.innerHTML = '<i class="fas fa-check"></i> 全部入库 (' + chunks.length + ')';
            const updateConfirmText = function () {
                confirmBtn.innerHTML = '<i class="fas fa-check"></i> 入库 (' + selected.size + '/' + chunks.length + ')';
            };
            list.addEventListener('click', updateConfirmText);
            confirmBtn.onclick = async function () {
                const selectedChunks = chunks.filter(function (_, idx) { return selected.has(idx); });
                if (selectedChunks.length === 0) {
                    self.toast('请至少选择一段', 'warning');
                    return;
                }
                await onConfirm(selectedChunks);
                self.toast('已入库 ' + selectedChunks.length + ' 段知识点', 'success');
            };

            btnGroup.appendChild(cancelBtn);
            btnGroup.appendChild(confirmBtn);
            preview.appendChild(btnGroup);
            panel.appendChild(preview);
        }

        /** ==================== 章节内容渲染 ==================== */

        /**
         * 渲染章节内容到指定容器。
         *
         * @param {object} container DOM 容器
         * @param {object} chapter 章节对象
         */
        renderChapter(container, chapter) {
            container.innerHTML = '';
            if (!chapter) {
                const empty = document.createElement('div');
                empty.className = 'empty-state';
                empty.innerHTML = '<i class="fas fa-book"></i><p>暂无章节内容</p>';
                container.appendChild(empty);
                return;
            }
            const title = document.createElement('h2');
            title.textContent = chapter.title || ('第 ' + (chapter.index + 1) + ' 章');
            title.className = 'chapter-title';
            container.appendChild(title);

            const content = document.createElement('div');
            content.className = 'chapter-content markdown-body';
            // 使用 marked.js 渲染（标准依赖库已加载）
            if (typeof marked !== 'undefined') {
                content.innerHTML = marked.parse(chapter.content || '');
            } else {
                content.textContent = chapter.content || '';
            }
            container.appendChild(content);

            // Token 信息
            if (chapter.tokens) {
                const meta = document.createElement('div');
                meta.className = 'chapter-meta';
                meta.innerHTML = '<i class="fas fa-coins"></i> tokens: ' + chapter.tokens;
                container.appendChild(meta);
            }
        }

        /**
         * 渲染当前草稿（生成中状态）。
         */
        renderDraft(container, draft) {
            container.innerHTML = '';
            if (!draft) {
                const empty = document.createElement('div');
                empty.className = 'empty-state';
                empty.innerHTML = '<i class="fas fa-pen-fancy"></i><p>暂无正在生成的草稿</p>';
                container.appendChild(empty);
                return;
            }

            const title = document.createElement('h3');
            title.textContent = '正在生成：第 ' + (draft.chapterIndex + 1) + ' 章（步骤 ' + draft.currentStep + '/10）';
            title.className = 'draft-title';
            container.appendChild(title);

            if (draft.paragraphs && draft.paragraphs.length > 0) {
                draft.paragraphs.forEach(function (para) {
                    const p = document.createElement('div');
                    p.className = 'draft-paragraph';
                    p.innerHTML =
                        '<div class="paragraph-header">' +
                        '<span class="paragraph-index">段落 ' + (para.index + 1) + '</span>' +
                        '<span class="paragraph-words">' + para.content.length + ' 字</span>' +
                        '</div>' +
                        '<div class="paragraph-content">' + (typeof marked !== 'undefined' ? marked.parse(para.content) : para.content) + '</div>';
                    container.appendChild(p);
                });
            } else {
                const info = document.createElement('p');
                info.className = 'modal-desc';
                info.textContent = '正在生成中...';
                container.appendChild(info);
            }
        }

        /** ==================== 状态栏更新 ==================== */

        /**
         * 更新状态栏。
         */
        updateStatus(phase, currentStep, totalTokens) {
            if (this.elements.statusText) {
                const phaseNames = {
                    idle: '空闲',
                    building: '生成中',
                    reviewing: '人工审核',
                    interrupted: '已中断'
                };
                let text = phaseNames[phase] || phase;
                if (currentStep) text += ' · 步骤 ' + currentStep + '/10';
                if (totalTokens !== undefined) text += ' · 累计 ' + totalTokens.toLocaleString() + ' tokens';
                this.elements.statusText.textContent = text;
            }
        }

        /**
         * 更新进度条。
         */
        updateProgress(step, message) {
            if (this.elements.progressText) {
                this.elements.progressText.textContent = message || '';
            }
            if (this.elements.progressBar) {
                const percent = step > 0 ? (step / 10 * 100) : 0;
                this.elements.progressBar.style.width = percent + '%';
            }
        }
    }

    // 暴露到全局
    global.UI = UI;
})(typeof window !== 'undefined' ? window : this);
