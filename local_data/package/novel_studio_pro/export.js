/**
 * 导出模块
 * 提供 TXT/MD 章节导出、CSV 工作日志（token 消耗明细）与 JSON 状态备份三类导出能力。
 *
 * 设计契约：
 *   - TXT/MD：导出大纲 + 已审核章节 + 当前草稿（grilling 问题 18 方案 A）
 *   - CSV 工作日志：分四段输出——调用明细 / 步骤级 token 分布 / 章节汇总 / 全局累计
 *     · 已审核章节的 callLog 在审批时已被压缩为 tokenBreakdown，故调用明细仅来源于 currentDraft
 *     · 步骤级分布与章节汇总对 approved + draft 全量可用
 *   - JSON 状态备份：完整 status.json 副本 + 导出元信息（时间戳、版本、章节数）
 *
 * 依赖：
 *   - status 对象（由 script.js 主类注入，结构与 workflow.js _initDraft / step9 一致）
 *   - formatters.countWords（可选，用于章节字数统计列）
 */

(function (global) {
    'use strict';

    /** 步骤中文名映射，用于 CSV 可读性 */
    const STEP_LABELS = {
        step1: '步骤1 大纲处理',
        step2: '步骤2 段落大纲',
        step3_paragraphs: '步骤3 段落生成',
        step4_format: '步骤4 格式校验',
        step5_review: '步骤5 章节大纲评审',
        step6_review: '步骤6 总体大纲评审',
        step7_review: '步骤7 段落评审',
        step7_rewrite: '步骤7 段落重写',
        step9_summary: '步骤9 摘要生成',
        step10_edit: '步骤10 后期编辑'
    };

    /** 全部步骤 key 列表，保证 CSV 列稳定 */
    const STEP_KEYS = [
        'step1', 'step2', 'step3_paragraphs', 'step4_format',
        'step5_review', 'step6_review', 'step7_review', 'step7_rewrite',
        'step9_summary', 'step10_edit'
    ];

    /** 导出文件名前缀 */
    const FILE_PREFIX = 'novel_studio_pro_';

    /**
     * Exporter 类
     * 负责所有导出操作，不直接操作 DOM，仅通过回调反馈结果。
     */
    class Exporter {
        /**
         * @param {object} status status.json 引用
         * @param {object} callbacks 回调集合
         * @param {function} [callbacks.onToast] 导出完成/失败提示 (message, level)
         * @param {function} [callbacks.onConfirm] 确认弹窗 (title, message) => Promise<boolean>
         */
        constructor(status, callbacks) {
            this.status = status;
            this.callbacks = callbacks || {};
        }

        /** ==================== TXT 导出 ==================== */

        /**
         * 导出 TXT 纯文本。
         * 结构：标题 → 大纲 → 已审核章节 → 当前草稿（如有）。
         *
         * @param {object} [options] 选项
         * @param {boolean} [options.includeOutline=true] 是否包含大纲
         * @param {boolean} [options.includeDraft=true] 是否包含当前草稿
         * @returns {Promise<boolean>} 是否成功
         */
        async exportTxt(options) {
            const opt = Object.assign({ includeOutline: true, includeDraft: true }, options || {});
            try {
                const sep = '='.repeat(60);
                const subSep = '-'.repeat(60);
                let content = '';

                content += '『 星月智能 · 小说工作室 Pro 』\n';
                content += '导出时间：' + new Date().toLocaleString('zh-CN') + '\n';
                content += sep + '\n\n';

                // 大纲
                if (opt.includeOutline && this.status.outline) {
                    content += '【总体大纲】\n\n';
                    content += this.status.outline + '\n\n';
                    content += sep + '\n\n';
                }

                // 章节走向（当前生效）
                if (this.status.direction) {
                    content += '【当前章节走向】\n\n';
                    content += this.status.direction + '\n\n';
                    if (this.status.nextCriteria) {
                        content += '【评判标准】\n\n' + this.status.nextCriteria + '\n\n';
                    }
                    content += sep + '\n\n';
                }

                // 已审核章节
                const approved = this._getApprovedChapters();
                approved.forEach(function (ch) {
                    content += ch.title + '\n\n';
                    content += ch.content + '\n\n';
                    content += subSep + '\n\n';
                });

                // 当前草稿
                if (opt.includeDraft) {
                    const draft = this._getCurrentDraftContent();
                    if (draft) {
                        content += draft.title + '（草稿，未审核）\n\n';
                        content += draft.content + '\n\n';
                    }
                }

                this._triggerDownload(
                    FILE_PREFIX + 'novel_' + this._timestamp() + '.txt',
                    content,
                    'text/plain;charset=utf-8'
                );
                this._notify('TXT 导出成功（共 ' + approved.length + ' 章）', 'success');
                return true;
            } catch (e) {
                console.error('TXT 导出失败', e);
                this._notify('TXT 导出失败：' + e.message, 'error');
                return false;
            }
        }

        /** ==================== MD 导出 ==================== */

        /**
         * 导出 Markdown。
         * 结构：H1 标题 → 大纲 → 章节 H2 → 草稿。
         *
         * @param {object} [options] 选项（同 exportTxt）
         * @returns {Promise<boolean>} 是否成功
         */
        async exportMd(options) {
            const opt = Object.assign({ includeOutline: true, includeDraft: true }, options || {});
            try {
                const lines = [];

                lines.push('# 星月智能 · 小说工作室 Pro');
                lines.push('');
                lines.push('> 导出时间：' + new Date().toLocaleString('zh-CN'));
                lines.push('');
                lines.push('---');
                lines.push('');

                // 大纲
                if (opt.includeOutline && this.status.outline) {
                    lines.push('## 总体大纲');
                    lines.push('');
                    lines.push(this.status.outline);
                    lines.push('');
                    lines.push('---');
                    lines.push('');
                }

                // 当前章节走向
                if (this.status.direction) {
                    lines.push('## 当前章节走向');
                    lines.push('');
                    lines.push(this.status.direction);
                    lines.push('');
                    if (this.status.nextCriteria) {
                        lines.push('### 评判标准');
                        lines.push('');
                        lines.push(this.status.nextCriteria);
                        lines.push('');
                    }
                    lines.push('---');
                    lines.push('');
                }

                // 已审核章节
                const approved = this._getApprovedChapters();
                approved.forEach(function (ch) {
                    // 若章节内容首行已为 # 标题，则避免重复；否则补 H2
                    const content = ch.content || '';
                    if (/^#\s+/.test(content.trim())) {
                        lines.push(content);
                    } else {
                        lines.push('## ' + ch.title);
                        lines.push('');
                        lines.push(content);
                    }
                    lines.push('');
                    lines.push('---');
                    lines.push('');
                });

                // 当前草稿
                if (opt.includeDraft) {
                    const draft = this._getCurrentDraftContent();
                    if (draft) {
                        lines.push('## ' + draft.title + '（草稿，未审核）');
                        lines.push('');
                        lines.push(draft.content);
                        lines.push('');
                    }
                }

                const content = lines.join('\n');
                this._triggerDownload(
                    FILE_PREFIX + 'novel_' + this._timestamp() + '.md',
                    content,
                    'text/markdown;charset=utf-8'
                );
                this._notify('MD 导出成功（共 ' + approved.length + ' 章）', 'success');
                return true;
            } catch (e) {
                console.error('MD 导出失败', e);
                this._notify('MD 导出失败：' + e.message, 'error');
                return false;
            }
        }

        /** ==================== CSV 工作日志 ==================== */

        /**
         * 导出 CSV 工作日志（token 消耗明细）。
         * 分四段：
         *   1. 调用明细（仅当前草稿 currentDraft.callLog，已审核章节 callLog 已压缩）
         *   2. 步骤级 token 分布（approved + draft 的 tokenBreakdown）
         *   3. 章节汇总（每章总 token、字数、审核时间）
         *   4. 全局累计
         *
         * @returns {Promise<boolean>} 是否成功
         */
        async exportCsv() {
            try {
                const rows = [];
                const BOM = '\uFEFF'; // Excel 兼容中文

                // ====== 段 1：调用明细 ======
                rows.push('# 段1 调用明细（仅当前草稿，已审核章节的 callLog 在审批时已压缩）');
                rows.push('章节,步骤,段落索引,尝试次数,调用类型,模型,输入Token,输出Token,总Token,时间');
                const draft = this.status.currentDraft;
                if (draft && Array.isArray(draft.callLog) && draft.callLog.length > 0) {
                    const draftIdx = draft.chapterIndex;
                    draft.callLog.forEach(function (log) {
                        rows.push([
                            '第' + draftIdx + '章(草稿)',
                            log.step || '',
                            log.paragraphIndex !== undefined && log.paragraphIndex >= 0 ? log.paragraphIndex : '',
                            log.attempt || 0,
                            log.type || 'chat',
                            log.model || '',
                            log.inputTokens || 0,
                            log.outputTokens || 0,
                            log.totalTokens || 0,
                            log.timestamp || ''
                        ].map(function (v) { return Exporter._csvCell(v); }).join(','));
                    });
                } else {
                    rows.push('（无草稿或草稿已审核，无调用明细）,,,,,,,,,');
                }
                rows.push('');

                // ====== 段 2：步骤级 token 分布 ======
                rows.push('# 段2 步骤级 token 分布（含已审核章节 + 当前草稿）');
                const header2 = ['章节', '状态'].concat(STEP_KEYS.map(function (k) { return STEP_LABELS[k] || k; }), ['章节合计']);
                rows.push(header2.map(function (v) { return Exporter._csvCell(v); }).join(','));

                // 已审核章节
                const approved = this._getApprovedChapters();
                approved.forEach(function (ch) {
                    const breakdown = ch.tokenBreakdown || {};
                    let total = 0;
                    const cells = ['第' + ch.index + '章', '已审核'];
                    STEP_KEYS.forEach(function (k) {
                        const v = breakdown[k] || 0;
                        total += v;
                        cells.push(v);
                    });
                    cells.push(total);
                    rows.push(cells.map(function (v) { return Exporter._csvCell(v); }).join(','));
                });

                // 当前草稿
                if (draft && draft.tokenBreakdown) {
                    let draftTotal = 0;
                    const cells = ['第' + draft.chapterIndex + '章', '草稿'];
                    STEP_KEYS.forEach(function (k) {
                        const v = draft.tokenBreakdown[k] || 0;
                        draftTotal += v;
                        cells.push(v);
                    });
                    cells.push(draftTotal);
                    rows.push(cells.map(function (v) { return Exporter._csvCell(v); }).join(','));
                }
                rows.push('');

                // ====== 段 3：章节汇总 ======
                rows.push('# 段3 章节汇总');
                rows.push('章节,状态,字数,Token总量,审核时间,修订次数,摘要字数');
                approved.forEach(function (ch) {
                    const wordCount = Exporter._countWords(ch.content);
                    const summaryLen = ch.summary ? ch.summary.length : 0;
                    const revCount = Array.isArray(ch.revisions) ? ch.revisions.length : 0;
                    rows.push([
                        '第' + ch.index + '章',
                        '已审核',
                        wordCount,
                        ch.tokens || 0,
                        ch.approvedAt || '',
                        revCount,
                        summaryLen
                    ].map(function (v) { return Exporter._csvCell(v); }).join(','));
                });
                if (draft) {
                    const draftContent = (draft.paragraphs || []).join('\n\n');
                    const draftWords = Exporter._countWords(draftContent);
                    rows.push([
                        '第' + draft.chapterIndex + '章',
                        '草稿',
                        draftWords,
                        draft.tokens || 0,
                        draft.startedAt || '',
                        Array.isArray(draft.revisions) ? draft.revisions.length : 0,
                        0
                    ].map(function (v) { return Exporter._csvCell(v); }).join(','));
                }
                rows.push('');

                // ====== 段 4：全局累计 ======
                rows.push('# 段4 全局累计');
                rows.push('指标,数值');
                const globalTotal = this._computeGlobalTokens();
                const approvedWords = approved.reduce(function (s, ch) {
                    return s + Exporter._countWords(ch.content);
                }, 0);
                rows.push('已审核章节数,' + approved.length);
                rows.push('已审核总字数,' + approvedWords);
                rows.push('全局 Token 总量,' + globalTotal.total);
                rows.push('  其中 输入 Token,' + globalTotal.input);
                rows.push('  其中 输出 Token,' + globalTotal.output);
                rows.push('当前草稿步骤,' + (draft ? draft.currentStep : '无'));
                rows.push('导出时间,' + new Date().toLocaleString('zh-CN'));

                const content = BOM + rows.join('\n');
                this._triggerDownload(
                    FILE_PREFIX + 'worklog_' + this._timestamp() + '.csv',
                    content,
                    'text/csv;charset=utf-8'
                );
                this._notify('CSV 工作日志导出成功', 'success');
                return true;
            } catch (e) {
                console.error('CSV 导出失败', e);
                this._notify('CSV 导出失败：' + e.message, 'error');
                return false;
            }
        }

        /** ==================== JSON 状态备份 ==================== */

        /**
         * 导出 JSON 状态备份。
         * 输出完整 status.json 副本，附带导出元信息，便于跨设备迁移或归档。
         *
         * @param {object} [options] 选项
         * @param {boolean} [options.includeVectorEmbeddings=false] 是否包含向量库 embedding（默认排除以减小体积）
         * @returns {Promise<boolean>} 是否成功
         */
        async exportJson(options) {
            const opt = Object.assign({ includeVectorEmbeddings: false }, options || {});
            try {
                // 深拷贝避免污染原 status
                const snapshot = JSON.parse(JSON.stringify(this.status));

                // 可选剥离向量库 embedding 字段（保留元信息）
                if (!opt.includeVectorEmbeddings && snapshot.vectorStoreCache) {
                    // vectorStoreCache 为运行时缓存字段，导出时移除
                    delete snapshot.vectorStoreCache;
                }

                // 附带导出元信息
                const backup = {
                    _meta: {
                        exporter: 'novel_studio_pro',
                        version: (snapshot.config && snapshot.config.version) || '1.0.0',
                        exportedAt: new Date().toISOString(),
                        chapterCount: (snapshot.chapters || []).length,
                        hasDraft: !!snapshot.currentDraft,
                        includeVectorEmbeddings: opt.includeVectorEmbeddings
                    },
                    status: snapshot
                };

                const content = JSON.stringify(backup, null, 2);
                this._triggerDownload(
                    FILE_PREFIX + 'backup_' + this._timestamp() + '.json',
                    content,
                    'application/json;charset=utf-8'
                );
                this._notify('状态备份导出成功', 'success');
                return true;
            } catch (e) {
                console.error('JSON 备份导出失败', e);
                this._notify('JSON 备份导出失败：' + e.message, 'error');
                return false;
            }
        }

        /** ==================== 统一导出入口 ==================== */

        /**
         * 根据 format 字符串分发导出。
         *
         * @param {string} format 'txt' | 'md' | 'csv' | 'json'
         * @param {object} [options] 各导出函数的选项
         * @returns {Promise<boolean>} 是否成功
         */
        async exportByFormat(format, options) {
            switch (String(format).toLowerCase()) {
                case 'txt':
                    return await this.exportTxt(options);
                case 'md':
                    return await this.exportMd(options);
                case 'csv':
                    return await this.exportCsv();
                case 'json':
                case 'backup':
                    return await this.exportJson(options);
                default:
                    this._notify('不支持的导出格式：' + format, 'error');
                    return false;
            }
        }

        /** ==================== 内部工具 ==================== */

        /**
         * 获取已审核通过章节列表（按章节序号升序）。
         * @returns {object[]}
         */
        _getApprovedChapters() {
            const chapters = Array.isArray(this.status.chapters) ? this.status.chapters : [];
            return chapters.slice().sort(function (a, b) {
                return (a.index || 0) - (b.index || 0);
            });
        }

        /**
         * 获取当前草稿的标题与内容（合并 paragraphs）。
         * @returns {object|null} {title, content}
         */
        _getCurrentDraftContent() {
            const draft = this.status.currentDraft;
            if (!draft || !Array.isArray(draft.paragraphs) || draft.paragraphs.length === 0) {
                return null;
            }
            const idx = draft.chapterIndex;
            const title = '第' + idx + '章';
            const content = draft.paragraphs.join('\n\n');
            return { title: title, content: content };
        }

        /**
         * 计算全局 token 累计。
         * 已审核章节用 ch.tokens，草稿用 tokenBreakdown 求和。
         * @returns {object} {total, input, output}
         */
        _computeGlobalTokens() {
            let total = 0;
            let input = 0;
            let output = 0;

            // 已审核章节（callLog 已压缩，仅 totalTokens 可用）
            const approved = this._getApprovedChapters();
            approved.forEach(function (ch) {
                total += ch.tokens || 0;
            });

            // 当前草稿
            const draft = this.status.currentDraft;
            if (draft) {
                if (draft.tokens) {
                    total += draft.tokens;
                } else if (draft.tokenBreakdown) {
                    let draftTotal = 0;
                    STEP_KEYS.forEach(function (k) {
                        draftTotal += draft.tokenBreakdown[k] || 0;
                    });
                    total += draftTotal;
                }
                // 草稿 callLog 保留明细，可统计 input/output
                if (Array.isArray(draft.callLog)) {
                    draft.callLog.forEach(function (log) {
                        input += log.inputTokens || 0;
                        output += log.outputTokens || 0;
                    });
                }
            }

            // 已审核章节的 input/output 已无法恢复（callLog 压缩），仅草稿可统计
            return { total: total, input: input, output: output };
        }

        /**
         * 触发浏览器下载。
         * @param {string} filename 文件名
         * @param {string} content 内容
         * @param {string} mimeType MIME 类型
         */
        _triggerDownload(filename, content, mimeType) {
            const blob = new Blob([content], { type: mimeType });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = filename;
            a.style.display = 'none';
            document.body.appendChild(a);
            a.click();
            // 释放需延迟，避免某些浏览器未完成下载即 revoke
            setTimeout(function () {
                document.body.removeChild(a);
                URL.revokeObjectURL(url);
            }, 100);
        }

        /**
         * 生成时间戳字符串（用于文件名，避免特殊字符）。
         * @returns {string} 形如 20260627_153045
         */
        _timestamp() {
            const d = new Date();
            const p = function (n) { return String(n).padStart(2, '0'); };
            return d.getFullYear() +
                p(d.getMonth() + 1) +
                p(d.getDate()) + '_' +
                p(d.getHours()) +
                p(d.getMinutes()) +
                p(d.getSeconds());
        }

        /**
         * Toast 通知封装。
         * @param {string} message
         * @param {string} level
         */
        _notify(message, level) {
            if (this.callbacks.onToast) {
                this.callbacks.onToast(message, level);
            }
        }

        /** ==================== 静态工具 ==================== */

        /**
         * CSV 单元格转义。
         * 含逗号、引号、换行时用双引号包裹，内部双引号转义为两个双引号。
         * @param {*} value
         * @returns {string}
         */
        static _csvCell(value) {
            if (value === null || value === undefined) return '';
            const str = String(value);
            if (/[",\n\r]/.test(str)) {
                return '"' + str.replace(/"/g, '""') + '"';
            }
            return str;
        }

        /**
         * 字数统计（与 formatters.countWords 行为一致，避免循环依赖内联实现）。
         * @param {string} text
         * @returns {number}
         */
        static _countWords(text) {
            if (!text) return 0;
            const stripped = text.replace(/[\s，。、；：！？""''（）【】《》\-—…·,.!?;:'"()\[\]{}<>\\\/|`~@#$%^&*_+=]/g, '');
            return stripped.length;
        }
    }

    // 暴露到全局
    global.NovelStudioProExporter = Exporter;
})(typeof window !== 'undefined' ? window : this);
