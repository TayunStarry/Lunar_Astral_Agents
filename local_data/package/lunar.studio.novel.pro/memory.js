/**
 * 记忆库客户端模块
 * 封装 /memory/ RESTful API，提供知识点增删查改与集合管理能力
 *
 * API 响应格式：{ success: true, data: {...} } 或 { success: false, error: "..." }
 */

(function(global) {
    'use strict';

    // ==== MemoryClient 类 ====
    class MemoryClient {
        constructor(app) {
            this.app = app;
            this.tableName = app.state.config.memoryTableName || 'novel_knowledge';
            this.initialized = false;
        }

        // ==== 通用请求封装 ====
        async _request(method, path, body) {
            var opts = {
                method: method,
                headers: { 'Content-Type': 'application/json' }
            };
            if (body !== undefined) {
                opts.body = JSON.stringify(body);
            }
            var resp = await fetch('/memory/' + path, opts);
            var json = await resp.json();
            if (!resp.ok || json.success === false) {
                throw new Error((json.error || json.message) || ('记忆库请求失败: ' + resp.status));
            }
            return json.data;
        }

        // ==== 确保记忆库已初始化 ====
        // 1. 检查全局初始化状态
        // 2. 未初始化则调用 /memory/init
        // 3. 检查集合是否存在，不存在则创建
        async ensureInitialized() {
            // 步骤 1: 检查全局初始化
            try {
                var stats = await this._request('GET', 'stats');
                if (!stats.initialized) {
                    await this._initInstance();
                }
            } catch (e) {
                // stats 接口异常，尝试直接初始化
                await this._initInstance();
            }

            // 步骤 2: 确保集合存在
            await this._ensureCollection();

            this.initialized = true;
        }

        // ==== 初始化记忆库实例 ====
        async _initInstance() {
            var baseUrl = this.app.config.getMemoryBaseUrl();
            var apiKey = this.app.config.getConfig().key;
            try {
                await this._request('POST', 'init', {
                    base_url: baseUrl,
                    api_key: apiKey
                });
            } catch (e) {
                // 已初始化不算错误
                if (!e.message.includes('已初始化')) {
                    throw e;
                }
            }
        }

        // ==== 确保集合已创建 ====
        async _ensureCollection() {
            var tableName = this.tableName;
            try {
                // 尝试获取集合统计，存在则直接返回
                await this._request('GET', tableName + '/stats');
                return;
            } catch (e) {
                // 集合不存在，创建之
            }
            var modelName = this.app.state.config.embeddingModel || 'system-embedding';
            await this._request('POST', tableName, {
                model_name: modelName
            });
        }

        // ==== 添加知识点 ====
        // POST /memory/{name}/messages { role, content }
        // 返回: { id, role, content }
        async addKnowledge(content, role) {
            var data = await this._request('POST', this.tableName + '/messages', {
                role: role || 'user',
                content: content
            });
            return data;
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

        // ==== 查询知识点（语义搜索） ====
        // GET /memory/{name}/messages?query=...&top_k=50
        // 客户端过滤：只保留 role='user' 的条目
        // 返回: [{ id, role, content, similarity }, ...]
        async queryKnowledge(queryText, topK) {
            var k = topK || 50;
            var data = await this._request('GET',
                this.tableName + '/messages?query=' + encodeURIComponent(queryText) + '&top_k=' + k
            );
            // 客户端过滤：只保留 role='user' 的知识点
            var results = (data.results || []).filter(function(r) {
                return r.role === 'user';
            });
            return results;
        }

        // ==== 列出知识点（分页） ====
        // GET /memory/{name}/documents?offset=0&limit=100
        // 返回: { documents: [...], total, offset, limit }
        async listKnowledge(offset, limit) {
            var off = offset || 0;
            var lim = limit || 100;
            var data = await this._request('GET',
                this.tableName + '/documents?offset=' + off + '&limit=' + lim
            );
            return data;
        }

        // ==== 删除知识点 ====
        // DELETE /memory/{name}/messages { id }
        async deleteKnowledge(id) {
            var data = await this._request('DELETE', this.tableName + '/messages', {
                id: id
            });
            return data;
        }

        // ==== 获取集合统计信息 ====
        // GET /memory/{name}/stats
        // 返回: { document_count, initialized, entry_count, sync_mismatch }
        async getStats() {
            var data = await this._request('GET', this.tableName + '/stats');
            return data;
        }

        // ==== 重建集合（删除维度不符文档） ====
        // POST /memory/{name}/rebuild
        async rebuild() {
            var data = await this._request('POST', this.tableName + '/rebuild');
            return data;
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
    global.MemoryClient = MemoryClient;
})(typeof window !== 'undefined' ? window : this);
