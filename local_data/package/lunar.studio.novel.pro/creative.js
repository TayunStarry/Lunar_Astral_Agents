/**
 * 创意构建模块
 * 负责：文件导入、系统导出格式检测、AI 提取纲要（总体大纲/评判标准/章节大纲）、确认初始化
 */

(function(global) {
    'use strict';

    // ==== CreativeBuilder 类 ====
    class CreativeBuilder {
        constructor(app) {
            this.app = app;
            this._extracting = false;
        }

        // ==== 文件选择回调 ====
        async onFileSelected(e) {
            var files = e.target.files;
            if (!files || files.length === 0) return;
            await this._importFile(files[0]);
            // 重置 file input，允许再次选择同一文件
            e.target.value = '';
        }

        // ==== 文件拖放回调 ====
        async onFileDropped(e) {
            var files = e.dataTransfer.files;
            if (!files || files.length === 0) return;
            await this._importFile(files[0]);
        }

        // ==== 导入文件处理 ====
        async _importFile(file) {
            var el = this.app.elements;
            var ext = file.name.split('.').pop().toLowerCase();

            if (!['txt', 'md', 'json'].includes(ext)) {
                this.app.showToast('仅支持 .txt / .md / .json 文件');
                return;
            }

            try {
                var text = await this._readFileText(file);

                // 检测是否为系统导出格式（JSON 中含 version + chapters 字段）
                if (ext === 'json') {
                    var isExportFormat = this._detectExportFormat(text);
                    if (isExportFormat) {
                        await this._handleExportImport(text);
                        return;
                    }
                }

                // 普通文本：填入输入框
                el.rawTextInput.value = text;
                this._showImportHint('已导入文件: ' + file.name + ' (' + text.length + ' 字)');
                this.app.showToast('文件内容已导入，可点击「AI 提取纲要」');
            } catch (e) {
                this.app.showToast('读取文件失败: ' + e.message);
            }
        }

        // ==== 读取文件文本 ====
        _readFileText(file) {
            return new Promise(function(resolve, reject) {
                var reader = new FileReader();
                reader.onload = function(e) { resolve(e.target.result); };
                reader.onerror = function() { reject(new Error('FileReader 错误')); };
                reader.readAsText(file, 'UTF-8');
            });
        }

        // ==== 检测系统导出格式 ====
        // JSON 中含 version + chapters 字段即为系统导出格式
        _detectExportFormat(text) {
            try {
                var data = JSON.parse(text);
                return !!(data && data.version && Array.isArray(data.chapters));
            } catch (e) {
                return false;
            }
        }

        // ==== 处理系统导出格式导入 ====
        async _handleExportImport(text) {
            var data = JSON.parse(text);
            var chapterCount = data.chapters ? data.chapters.length : 0;

            var doImport = confirm(
                '检测到系统导出格式（版本 ' + data.version + '，' + chapterCount + ' 个章节）。\n' +
                '是否导入已有章节？\n\n' +
                '注意：即使导入章节，纲要提取仍需执行。'
            );

            if (!doImport) {
                // 不导入章节，仅填入文本区域供参考
                this.app.elements.rawTextInput.value = text;
                this._showImportHint('已将导出文件内容填入输入框（未导入章节）');
                return;
            }

            // 导入章节和纲要
            var state = this.app.state;

            // 导入纲要
            if (data.outline) state.outline = data.outline;
            if (data.criteria) state.criteria = data.criteria;
            if (data.chapterOutlines && data.chapterOutlines.length > 0) {
                state.chapterOutlines = data.chapterOutlines;
            }

            // 导入章节
            if (data.chapters && data.chapters.length > 0) {
                state.chapters = data.chapters;
                state.chapterIndex = data.chapters.length - 1;
            }

            // 导入配置（如果存在）
            if (data.config) {
                if (data.config.wordMin) state.config.wordMin = data.config.wordMin;
                if (data.config.wordMax) state.config.wordMax = data.config.wordMax;
                if (data.config.paragraphTarget) state.config.paragraphTarget = data.config.paragraphTarget;
            }

            // 导入 token 统计
            if (data.totalTokens) state.totalTokens = data.totalTokens;

            // 仍需检查纲要是否完整
            var hasOutline = !!state.outline;
            var hasCriteria = !!state.criteria;
            var hasChapterOutlines = state.chapterOutlines && state.chapterOutlines.length > 0;

            if (hasOutline && hasCriteria && hasChapterOutlines) {
                // 纲要完整，但提取仍需执行以验证
                this.app.elements.rawTextInput.value = '已导入 ' + chapterCount + ' 个章节和纲要。如需调整请编辑右侧纲要区域。';
                this._showImportHint('已导入 ' + chapterCount + ' 个章节。纲要已填写完整，可直接确认初始化。');
            } else {
                this.app.elements.rawTextInput.value = '已导入 ' + chapterCount + ' 个章节，但纲要不完整，仍需 AI 提取。';
                this._showImportHint('章节已导入，但纲要缺失，请点击「AI 提取纲要」补全。');
            }

            // 更新右侧纲要编辑区
            this.app.elements.outlineInput.value = state.outline;
            this.app.elements.criteriaInput.value = state.criteria;

            await this.app.saveState();
            this.app.renderCreative();
            this.app.renderTokenBoard();
            this.app.showToast('已导入 ' + chapterCount + ' 个章节');
        }

        // ==== 显示导入提示 ====
        _showImportHint(msg) {
            var el = this.app.elements;
            el.importHint.textContent = msg;
            el.importHint.style.display = 'block';
            clearTimeout(this._hintTimer);
            this._hintTimer = setTimeout(function() {
                el.importHint.style.display = 'none';
            }, 10000);
        }

        // ==== AI 提取纲要（三步顺序执行） ====
        async extractOutline() {
            if (this._extracting) return;

            var el = this.app.elements;
            var rawText = (el.rawTextInput.value || '').trim();

            // 允许从右侧已有纲要+手动输入来提取
            // 如果输入框为空但右侧已有大纲，则基于大纲提取
            if (!rawText) {
                var existingOutline = (el.outlineInput.value || '').trim();
                if (!existingOutline) {
                    this.app.showToast('请先输入或导入原始文本');
                    return;
                }
                rawText = existingOutline;
            }

            this._extracting = true;
            el.extractOutlineBtn.disabled = true;
            el.extractOutlineBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 提取中...';

            try {
                // 步骤 1: 提取总体大纲
                this.app.showToast('正在提取总体大纲...');
                var outline = await this._extractStep('extract_outline', { rawText: rawText });
                if (outline) {
                    el.outlineInput.value = outline;
                    this.app.state.outline = outline;
                    await this.app.saveState();
                }

                // 步骤 2: 提取评判标准（依赖大纲）
                this.app.showToast('正在提取评判标准...');
                var criteria = await this._extractStep('extract_criteria', {
                    outline: el.outlineInput.value || outline,
                    rawText: rawText
                });
                if (criteria) {
                    el.criteriaInput.value = criteria;
                    this.app.state.criteria = criteria;
                    await this.app.saveState();
                }

                // 步骤 3: 提取各章节大纲（依赖大纲+评判标准）
                this.app.showToast('正在提取各章节大纲...');
                var chapterOutlines = await this._extractChapterOutlines(
                    el.outlineInput.value || outline,
                    el.criteriaInput.value || criteria
                );
                if (chapterOutlines && chapterOutlines.length > 0) {
                    this.app.state.chapterOutlines = chapterOutlines;
                    await this.app.saveState();
                }

                // 步骤 4: 自动将原始文本拆解为知识点入库
                this.app.showToast('正在从原始文本提取知识点...');
                await this._autoExtractKnowledge(rawText);

                // 更新 UI
                this.app.renderCreative();
                this.app.showToast('纲要提取完成，知识库已自动构建');
            } catch (e) {
                this.app.showToast('纲要提取失败: ' + e.message);
            } finally {
                this._extracting = false;
                el.extractOutlineBtn.disabled = false;
                el.extractOutlineBtn.innerHTML = '<i class="fas fa-magic"></i> AI 提取纲要';
            }
        }

        // ==== 单步提取：总体大纲 / 评判标准 ====
        async _extractStep(promptName, vars) {
            var prompt = await global.PromptLoader.loadAndFill(promptName, vars);
            var messages = [
                { role: 'system', content: '你是一位专业的小说创作顾问。请严格按照用户要求的格式输出，不要添加额外解释。' },
                { role: 'user', content: prompt }
            ];

            var result = await this.app.config.callChat(messages, {
                temperature: 0.7,
                maxTokens: 4096
            });

            // 记录 token
            this.app.config.trackToken({
                step: 'creative_' + promptName,
                model: this.app.config.getConfig().name,
                inputTokens: result.inputTokens,
                outputTokens: result.outputTokens,
                totalTokens: result.totalTokens
            });

            return (result.content || '').trim();
        }

        // ==== 提取各章节大纲（需要 JSON 解析） ====
        async _extractChapterOutlines(outline, criteria) {
            var prompt = await global.PromptLoader.loadAndFill('extract_chapter_outlines', {
                outline: outline,
                criteria: criteria
            });
            var messages = [
                { role: 'system', content: '你是一位专业的小说大纲编辑。请严格按照 JSON 格式返回章节大纲数组，不要添加任何额外文字。' },
                { role: 'user', content: prompt }
            ];

            var result = await this.app.config.callChat(messages, {
                temperature: 0.7,
                maxTokens: 4096
            });

            // 记录 token
            this.app.config.trackToken({
                step: 'creative_extract_chapter_outlines',
                model: this.app.config.getConfig().name,
                inputTokens: result.inputTokens,
                outputTokens: result.outputTokens,
                totalTokens: result.totalTokens
            });

            // 解析 JSON
            return this._parseChapterOutlines(result.content);
        }

        // ==== 自动从原始文本提取知识点并入库 ====
        async _autoExtractKnowledge(rawText) {
            if (!rawText || rawText.length < 20) return;

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
                step: 'creative_knowledge_extract',
                model: this.app.config.getConfig().name,
                inputTokens: result.inputTokens,
                outputTokens: result.outputTokens,
                totalTokens: result.totalTokens
            });

            // 解析知识点
            var knowledgeItems = this._parseKnowledgeArray(result.content);
            if (!knowledgeItems || knowledgeItems.length === 0) return;

            // 批量入库
            await this.app.memory.addBulkKnowledge(knowledgeItems);

            // 刷新知识库列表
            await this.app.knowledge.loadAndRender();
        }

        // ==== 解析 AI 返回的知识点数组 ====
        _parseKnowledgeArray(text) {
            if (!text) return null;

            // 尝试提取 ```json ... ``` 代码块
            var codeBlockMatch = text.match(/```(?:json)?\s*\n?([\s\S]*?)\n?```/);
            var jsonStr = codeBlockMatch ? codeBlockMatch[1].trim() : text.trim();

            // 尝试 JSON 解析
            try {
                var parsed = JSON.parse(jsonStr);
                if (Array.isArray(parsed)) {
                    return parsed.map(function(item) {
                        if (typeof item === 'string') return item;
                        if (typeof item === 'object' && item !== null) {
                            return item.content || item.text || item.summary || JSON.stringify(item);
                        }
                        return String(item);
                    }).filter(function(s) { return s && s.trim(); });
                }
            } catch (e) {
                // JSON 解析失败，尝试按行分割
            }

            // 按行分割
            var lines = jsonStr.split('\n')
                .map(function(l) { return l.replace(/^\s*[\d]+\.\s*/, '').trim(); })
                .filter(function(l) { return l.length > 2; });

            return lines.length > 0 ? lines : null;
        }

        // ==== 解析章节大纲 JSON ====
        _parseChapterOutlines(text) {
            if (!text) return null;

            // 尝试提取 ```json ... ``` 代码块
            var codeBlockMatch = text.match(/```(?:json)?\s*\n?([\s\S]*?)\n?```/);
            var jsonStr = codeBlockMatch ? codeBlockMatch[1].trim() : text.trim();

            // 尝试直接 JSON 解析
            try {
                var parsed = JSON.parse(jsonStr);
                if (Array.isArray(parsed)) {
                    return parsed.map(function(item, idx) {
                        if (typeof item === 'string') {
                            return { index: idx + 1, outline: item };
                        }
                        return {
                            index: item.index || idx + 1,
                            outline: item.outline || item.content || item.text || ''
                        };
                    }).filter(function(co) { return co.outline.trim(); });
                }
                // 单对象包装
                if (parsed.chapters && Array.isArray(parsed.chapters)) {
                    return parsed.chapters.map(function(item, idx) {
                        return {
                            index: item.index || idx + 1,
                            outline: item.outline || item.content || item.text || ''
                        };
                    }).filter(function(co) { return co.outline.trim(); });
                }
            } catch (e) {
                // JSON 解析失败，尝试按行分割
            }

            // 按行分割：形如 "1. xxx" 或 "第一章 xxx" 的格式
            var lines = jsonStr.split('\n').filter(function(l) { return l.trim(); });
            var outlines = [];
            for (var i = 0; i < lines.length; i++) {
                var line = lines[i].trim();
                // 移除序号前缀
                line = line.replace(/^\s*(?:第[一二三四五六七八九十百千\d]+章|[\d]+[\.、\)）])\s*/, '');
                if (line.length > 2) {
                    outlines.push({ index: outlines.length + 1, outline: line });
                }
            }

            return outlines.length > 0 ? outlines : null;
        }

        // ==== 确认初始化 ====
        async confirmInit() {
            var el = this.app.elements;
            var state = this.app.state;

            // 收集当前编辑区的值
            var outline = (el.outlineInput.value || '').trim();
            var criteria = (el.criteriaInput.value || '').trim();
            var chapterOutlines = this._collectChapterOutlines();

            // 最终验证
            if (!outline) {
                this.app.showToast('总体大纲不能为空');
                return;
            }
            if (!criteria) {
                this.app.showToast('章节评判标准不能为空');
                return;
            }
            if (!chapterOutlines || chapterOutlines.length === 0) {
                this.app.showToast('各章节大纲不能为空');
                return;
            }

            // 写入状态
            state.outline = outline;
            state.criteria = criteria;
            state.chapterOutlines = chapterOutlines;
            state.initialized = true;

            // 如果尚无方向和标准，取第一章
            if (!state.nextDirection && chapterOutlines.length > 0) {
                state.nextDirection = chapterOutlines[0].outline;
            }

            await this.app.saveState();

            // 初始化完成，折叠创意构建区
            this.app.renderSidebarState();
            this.app.renderAll();
            this.app.showToast('初始化完成，可以开始构建章节');
        }

        // ==== 收集各章节大纲 ====
        _collectChapterOutlines() {
            var list = this.app.elements.chapterOutlinesList;
            var textareas = list.querySelectorAll('.chapter-outline-text');
            if (textareas.length === 0) {
                // 如果没有可编辑的 textarea，直接用 state 中的值
                return this.app.state.chapterOutlines || [];
            }

            var outlines = [];
            textareas.forEach(function(ta) {
                var idx = parseInt(ta.dataset.index);
                var text = ta.value.trim();
                if (text) {
                    outlines.push({ index: idx + 1, outline: text });
                }
            });

            // 如果 textarea 都为空（未展开编辑过），回退到 state
            return outlines.length > 0 ? outlines : (this.app.state.chapterOutlines || []);
        }
    }

    // ==== 暴露到全局 ====
    global.CreativeBuilder = CreativeBuilder;
})(typeof window !== 'undefined' ? window : this);
