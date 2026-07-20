/**
 * 导出模块
 * 提供 TXT 纯文本导出和 JSON 状态备份导出
 */

(function(global) {
    'use strict';

    // ==== Exporter 类 ====
    class Exporter {
        constructor(app) {
            this.app = app;
        }

        // ==== 执行导出 ====
        async export(format) {
            var state = this.app.state;

            if (format === 'txt') {
                this._exportTXT(state);
            } else if (format === 'json') {
                this._exportJSON(state);
            } else {
                this.app.showToast('不支持的导出格式');
            }
        }

        // ==== TXT 纯文本导出 ====
        _exportTXT(state) {
            var chapters = state.chapters || [];
            if (chapters.length === 0) {
                this.app.showToast('没有可导出的章节');
                return;
            }

            var lines = [];

            // 标题
            lines.push('========================================');
            lines.push('  ' + (state.config.novelTitle || '小说工作室 Pro 作品'));
            lines.push('========================================');
            lines.push('');

            // 总体大纲（可选）
            if (state.outline) {
                lines.push('【总体大纲】');
                lines.push(state.outline);
                lines.push('');
            }

            // 各章节
            chapters.forEach(function(chapter) {
                if (chapter.status !== 'approved') return;

                lines.push('────────────────────────────────────────');
                lines.push('第 ' + chapter.index + ' 章');
                lines.push('────────────────────────────────────────');
                lines.push('');
                lines.push(chapter.content || '');
                lines.push('');

                // 章节摘要（附在末尾）
                if (chapter.summary) {
                    lines.push('【章节摘要】' + chapter.summary);
                    lines.push('');
                }
            });

            lines.push('========================================');
            lines.push('  全文完');
            lines.push('========================================');

            var content = lines.join('\n');
            this._download(content, 'novel_export.txt', 'text/plain;charset=utf-8');
            this.app.showToast('TXT 导出成功');
        }

        // ==== JSON 状态备份导出 ====
        _exportJSON(state) {
            var chapters = state.chapters || [];
            if (chapters.length === 0 && !state.outline) {
                this.app.showToast('没有可导出的数据');
                return;
            }

            // 构建导出对象（去除内部运行时字段）
            var exportData = {
                version: 3,
                exportedAt: new Date().toISOString(),
                config: {
                    wordMin: state.config.wordMin,
                    wordMax: state.config.wordMax,
                    paragraphTarget: state.config.paragraphTarget,
                    novelTitle: state.config.novelTitle || ''
                },
                outline: state.outline || '',
                criteria: state.criteria || '',
                chapterOutlines: state.chapterOutlines || [],
                chapters: chapters.map(function(ch) {
                    return {
                        index: ch.index,
                        title: ch.title || '',
                        outline: ch.outline || '',
                        direction: ch.direction || '',
                        content: ch.content || '',
                        summary: ch.summary || '',
                        status: ch.status,
                        tokens: ch.tokens || 0,
                        tokenBreakdown: ch.tokenBreakdown || {},
                        approvedAt: ch.approvedAt || '',
                        editedAt: ch.editedAt || '',
                        revisions: ch.revisions || []
                    };
                }),
                totalTokens: state.totalTokens || 0
            };

            var content = JSON.stringify(exportData, null, 2);
            this._download(content, 'novel_backup.json', 'application/json;charset=utf-8');
            this.app.showToast('JSON 备份导出成功');
        }

        // ==== 触发浏览器下载 ====
        _download(content, filename, mimeType) {
            var blob = new Blob([content], { type: mimeType });
            var url = URL.createObjectURL(blob);
            var a = document.createElement('a');
            a.href = url;
            a.download = filename;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            setTimeout(function() { URL.revokeObjectURL(url); }, 1000);
        }
    }

    // ==== 暴露到全局 ====
    global.Exporter = Exporter;
})(typeof window !== 'undefined' ? window : this);
