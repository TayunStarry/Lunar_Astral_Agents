/**
 * 知识库面板模块
 * 负责知识库 UI 交互：快速单条入库、AI 智能摘要拆解入库、知识点列表渲染
 */

(function(global) {
    'use strict';

    // ==== KnowledgePanel 类 ====
    class KnowledgePanel {
        constructor(app) {
            this.app = app;
            this._currentPage = 0;
            this._pageSize = 50;
            this._totalCount = 0;
            this._loading = false;
        }

        // ==== 快速单条入库 ====
        async addSingle() {
            var el = this.app.elements;
            var content = (el.spContentInput.value || '').trim();
            if (!content) {
                this.app.showToast('请输入知识点内容');
                return;
            }

            el.spAddBtn.disabled = true;
            try {
                await this.app.memory.addKnowledge(content, 'user');
                el.spContentInput.value = '';
                this.app.showToast('知识点已入库');
                await this.loadAndRender();
            } catch (e) {
                this.app.showToast('入库失败: ' + e.message);
            } finally {
                el.spAddBtn.disabled = false;
            }
        }

        // ==== AI 摘要入库 ====
        // 1. 读取用户粘贴的长文本
        // 2. 调用 AI 拆解为知识点摘要
        // 3. 解析 AI 返回的 JSON 数组
        // 4. 逐条入库
        async aiSummarize() {
            var el = this.app.elements;
            var rawText = (el.bulkTextInput.value || '').trim();
            if (!rawText) {
                this.app.showToast('请输入长文本内容');
                return;
            }

            el.aiSummarizeBtn.disabled = true;
            try {
                // 加载 prompt 模板
                var prompt = await global.PromptLoader.loadAndFill('summarize_knowledge', {
                    rawText: rawText
                });

                var messages = [
                    { role: 'system', content: '你是一个专业的知识库编辑助手。请将用户提供的长文本拆解为独立的知识点摘要，以 JSON 数组格式返回。每个元素是一个字符串，代表一个知识点。不要添加任何额外的解释，只返回 JSON 数组。' },
                    { role: 'user', content: prompt }
                ];

                var result = await this.app.config.callChat(messages, {
                    temperature: 0.3,
                    maxTokens: 4096
                });

                // 记录 token
                this.app.config.trackToken({
                    step: 'knowledge_summarize',
                    model: this.app.config.getConfig().name,
                    inputTokens: result.inputTokens,
                    outputTokens: result.outputTokens,
                    totalTokens: result.totalTokens
                });

                // 解析 AI 返回的知识点数组
                var knowledgeItems = this._parseKnowledgeArray(result.content);
                if (!knowledgeItems || knowledgeItems.length === 0) {
                    this.app.showToast('AI 未返回有效的知识点，请检查内容');
                    return;
                }

                this.app.showToast('AI 拆解出 ' + knowledgeItems.length + ' 个知识点，正在入库...');

                // 批量入库
                var results = await this.app.memory.addBulkKnowledge(knowledgeItems);
                el.bulkTextInput.value = '';
                this.app.showToast('已入库 ' + results.length + ' 个知识点');
                await this.loadAndRender();
            } catch (e) {
                this.app.showToast('AI 摘要失败: ' + e.message);
            } finally {
                el.aiSummarizeBtn.disabled = false;
            }
        }

        // ==== 解析 AI 返回的知识点数组 ====
        // 支持多种格式：纯 JSON 数组、```json 包裹、每行一条
        _parseKnowledgeArray(text) {
            if (!text) return null;

            // 尝试提取 ```json ... ``` 代码块
            var codeBlockMatch = text.match(/```(?:json)?\s*\n?([\s\S]*?)\n?```/);
            var jsonStr = codeBlockMatch ? codeBlockMatch[1].trim() : text.trim();

            // 尝试 JSON 解析
            try {
                var parsed = JSON.parse(jsonStr);
                if (Array.isArray(parsed)) {
                    // 过滤非字符串元素，对象转字符串
                    return parsed.map(function(item) {
                        if (typeof item === 'string') return item;
                        if (typeof item === 'object' && item !== null) {
                            // 支持 [{content: "..."}, ...] 格式
                            return item.content || item.text || item.summary || JSON.stringify(item);
                        }
                        return String(item);
                    }).filter(function(s) { return s && s.trim(); });
                }
            } catch (e) {
                // JSON 解析失败，尝试按行分割
            }

            // 按行分割（过滤空行和纯数字编号行）
            var lines = jsonStr.split('\n')
                .map(function(l) { return l.replace(/^\s*[\d]+\.\s*/, '').trim(); })
                .filter(function(l) { return l.length > 2; });

            return lines.length > 0 ? lines : null;
        }

        // ==== 加载并渲染知识点列表 ====
        async loadAndRender() {
            if (this._loading) return;
            this._loading = true;
            try {
                var data = await this.app.memory.listKnowledge(this._currentPage * this._pageSize, this._pageSize);
                this._totalCount = data.total || 0;
                this.renderList(data.documents || []);
            } catch (e) {
                console.warn('加载知识点列表失败:', e);
                this.renderList([]);
            } finally {
                this._loading = false;
            }
        }

        // ==== 渲染知识点列表 ====
        renderList(documents) {
            var el = this.app.elements;
            var self = this;

            // 更新计数
            el.memoriesCounter.textContent = this._totalCount;

            if (!documents || documents.length === 0) {
                el.memoriesList.innerHTML = '<div class="empty-state-small">暂无知识点</div>';
                return;
            }

            // 渲染列表
            el.memoriesList.innerHTML = documents.map(function(doc) {
                var preview = doc.content.length > 80 ? doc.content.substring(0, 80) + '...' : doc.content;
                return '<div class="memory-item" data-id="' + self._escapeAttr(doc.id) + '">' +
                    '<div class="memory-content">' + self._escapeHtml(preview) + '</div>' +
                    '<div class="memory-meta">' +
                        '<span class="memory-id">#' + self._escapeHtml(doc.id.substring(0, 8)) + '</span>' +
                        '<span class="memory-role">' + self._escapeHtml(doc.role) + '</span>' +
                    '</div>' +
                    '<button class="memory-delete-btn" data-id="' + self._escapeAttr(doc.id) + '" title="删除此知识点">' +
                        '<i class="fas fa-trash-alt"></i>' +
                    '</button>' +
                '</div>';
            }).join('');

            // 绑定删除按钮事件
            el.memoriesList.querySelectorAll('.memory-delete-btn').forEach(function(btn) {
                btn.addEventListener('click', function() {
                    var id = btn.dataset.id;
                    self.deleteAndRefresh(id);
                });
            });

            // 绑定点击展开全文
            el.memoriesList.querySelectorAll('.memory-content').forEach(function(contentEl) {
                contentEl.addEventListener('click', function() {
                    var item = contentEl.closest('.memory-item');
                    var id = item.dataset.id;
                    var doc = documents.find(function(d) { return d.id === id; });
                    if (doc && contentEl.textContent.length < doc.content.length) {
                        if (contentEl.classList.contains('expanded')) {
                            contentEl.textContent = doc.content.substring(0, 80) + '...';
                            contentEl.classList.remove('expanded');
                        } else {
                            contentEl.textContent = doc.content;
                            contentEl.classList.add('expanded');
                        }
                    }
                });
            });

            // 分页控件
            this._renderPagination();
        }

        // ==== 渲染分页 ====
        _renderPagination() {
            var el = this.app.elements;
            var totalPages = Math.ceil(this._totalCount / this._pageSize);
            if (totalPages <= 1) return;

            var self = this;
            var paginationHtml = '<div class="memories-pagination">';
            if (this._currentPage > 0) {
                paginationHtml += '<button class="btn-glass btn-glass-small page-btn" data-page="' + (this._currentPage - 1) + '"><i class="fas fa-chevron-left"></i></button>';
            }
            paginationHtml += '<span class="page-info">' + (this._currentPage + 1) + ' / ' + totalPages + '</span>';
            if (this._currentPage < totalPages - 1) {
                paginationHtml += '<button class="btn-glass btn-glass-small page-btn" data-page="' + (this._currentPage + 1) + '"><i class="fas fa-chevron-right"></i></button>';
            }
            paginationHtml += '</div>';

            // 追加分页到列表底部
            var existing = el.memoriesList.querySelector('.memories-pagination');
            if (existing) existing.remove();
            el.memoriesList.insertAdjacentHTML('beforeend', paginationHtml);

            el.memoriesList.querySelectorAll('.page-btn').forEach(function(btn) {
                btn.addEventListener('click', function() {
                    self._currentPage = parseInt(btn.dataset.page);
                    self.loadAndRender();
                });
            });
        }

        // ==== 删除知识点并刷新列表 ====
        async deleteAndRefresh(id) {
            if (!confirm('确定删除此知识点？')) return;
            try {
                await this.app.memory.deleteKnowledge(id);
                this.app.showToast('知识点已删除');
                await this.loadAndRender();
            } catch (e) {
                this.app.showToast('删除失败: ' + e.message);
            }
        }

        // ==== 工具方法 ====
        _escapeHtml(text) {
            var div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        _escapeAttr(text) {
            return text.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
        }
    }

    // ==== 暴露到全局 ====
    global.KnowledgePanel = KnowledgePanel;
})(typeof window !== 'undefined' ? window : this);
