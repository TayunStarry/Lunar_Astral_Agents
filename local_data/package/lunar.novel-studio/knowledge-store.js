/**
 * 知识库客户端模块
 * 知识点存放在通用知识库 knowledge.db 的独立表中，通过原生 SQL 端点读写：
 *   POST /knowledge/ { sql, params, db: 'knowledge' }
 * 返回格式：{ success: true, results: [OperationResult] } 或 { success: false, error: "..." }
 */

(function(global) {
    'use strict';

    // ==== KnowledgeStore 类 ====
    class KnowledgeStore {
        constructor(app) {
            this.app = app;
            this.table = app.state.config.knowledgeTableName || 'novel_knowledge';
            this.ready = false;
        }

        // ==== 原生 SQL 通道（返回首个 OperationResult） ====
        async sql(statement, params) {
            var resp = await fetch('/knowledge/', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ sql: statement, params: params || [], db: 'knowledge' })
            });
            var json = await resp.json();
            if (!resp.ok || json.success === false) {
                throw new Error((json && json.error) || ('知识库请求失败: ' + resp.status));
            }
            return (json.results && json.results[0]) || { success: true };
        }

        // ==== 确保知识点表存在（幂等，进程内只建一次） ====
        async ensureTable() {
            if (this.ready) return;
            await this.sql(
                'CREATE TABLE IF NOT EXISTS ' + this.table + ' (' +
                'id INTEGER PRIMARY KEY AUTOINCREMENT, ' +
                'content TEXT NOT NULL, ' +
                'role TEXT NOT NULL, ' +
                'created_at TEXT NOT NULL)'
            );
            this.ready = true;
        }

        // ==== 添加知识点 ====
        // 返回 { id, role, content }
        async addKnowledge(content, role) {
            await this.ensureTable();
            var itemRole = role || 'user';
            var result = await this.sql(
                'INSERT INTO ' + this.table + ' (content, role, created_at) VALUES (?, ?, datetime(\'now\', \'localtime\'))',
                [content, itemRole]
            );
            return { id: result.last_insert_id, role: itemRole, content: content };
        }

        // ==== 批量添加知识点 ====
        async addBulkKnowledge(items) {
            var results = [];
            for (var i = 0; i < items.length; i++) {
                var item = items[i];
                try {
                    var result = await this.addKnowledge(
                        typeof item === 'string' ? item : item.content,
                        typeof item === 'string' ? 'user' : (item.role || 'user')
                    );
                    results.push(result);
                } catch (e) {
                    console.warn('批量添加知识点失败 (第' + (i + 1) + '条):', e.message);
                }
            }
            return results;
        }

        // ==== 查询知识点（按关键词匹配内容） ====
        // 关键词按空白与标点切分，2 字以上参与匹配；返回 [{ id, content, role }, ...]
        async queryKnowledge(queryText, topK) {
            await this.ensureTable();
            var k = topK || 50;
            var tokens = String(queryText || '')
                .split(/[\s,，。；;、:：!！?？]+/)
                .filter(function(t) { return t.length >= 2; });
            var statement, params;
            if (tokens.length === 0) {
                statement = 'SELECT id, content, role FROM ' + this.table + ' ORDER BY id DESC LIMIT ?';
                params = [k];
            } else {
                var where = tokens.map(function() { return 'content LIKE ?'; }).join(' OR ');
                statement = 'SELECT id, content, role FROM ' + this.table + ' WHERE ' + where + ' ORDER BY id DESC LIMIT ?';
                params = tokens.map(function(t) { return '%' + t + '%'; });
                params.push(k);
            }
            var result = await this.sql(statement, params);
            return Array.isArray(result.rows) ? result.rows : [];
        }

        // ==== 列出知识点（分页） ====
        // 返回: { documents: [...], total, offset, limit }
        async listKnowledge(offset, limit) {
            await this.ensureTable();
            var off = offset || 0;
            var lim = limit || 100;
            var countResult = await this.sql('SELECT COUNT(*) AS total FROM ' + this.table);
            var total = (countResult.rows && countResult.rows[0] && countResult.rows[0].total) || 0;
            var listResult = await this.sql(
                'SELECT id, content, role FROM ' + this.table + ' ORDER BY id DESC LIMIT ? OFFSET ?',
                [lim, off]
            );
            return {
                documents: Array.isArray(listResult.rows) ? listResult.rows : [],
                total: total,
                offset: off,
                limit: lim
            };
        }

        // ==== 删除知识点 ====
        async deleteKnowledge(id) {
            await this.ensureTable();
            await this.sql('DELETE FROM ' + this.table + ' WHERE id = ?', [id]);
        }

        // ==== 获取知识点上下文文本 ====
        // 供段落生成时查询相关知识点，返回拼接的文本
        async getKnowledgeContext(queryText, topK) {
            var results = await this.queryKnowledge(queryText, topK || 50);
            if (!results || results.length === 0) return '';
            return results.map(function(r) {
                return r.content;
            }).join('\n');
        }
    }

    // ==== 暴露到全局 ====
    global.KnowledgeStore = KnowledgeStore;
})(typeof window !== 'undefined' ? window : this);
