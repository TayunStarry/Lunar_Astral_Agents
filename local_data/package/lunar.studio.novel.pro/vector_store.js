/**
 * 向量存储模块
 * 前端自托管的向量检索系统，调用 /v1/embeddings 嵌入文本，将向量与内容存为 JSON 文件，
 * 查询时客户端计算余弦相似度返回 top_k。
 *
 * 设计契约：
 *   - 嵌入模型固定 system-embedding（grilling 问题 3）
 *   - 表名机制 = 不同 JSON 文件（story_points.json / chapter_summaries.json，grilling 问题 2 转向）
 *   - 启动时校验维度，变更时弹窗确认清空（grilling 问题 17）
 *   - 首次嵌入时探测维度（grilling 问题 17）
 *   - 嵌入调用计入 callLog token 跟踪（grilling 问题 16）
 */

(function (global) {
    'use strict';

    /** 嵌入模型名（固定，UI 不暴露） */
    const EMBEDDING_MODEL = 'system-embedding';

    /** 向量文件根目录（相对 /file/read/package/ 的路径） */
    const VECTOR_STORE_DIR = 'lunar.studio.novel_pro/vector_store';

    /** 表名 → 文件名映射，构成"表名机制" */
    const TABLE_FILES = {
        story_points: 'story_points.json',
        chapter_summaries: 'chapter_summaries.json'
    };

    /**
     * VectorStore 类
     * 负责嵌入、存储、检索、维度校验等向量数据管理。
     */
    class VectorStore {
        /**
         * @param {object} statusRef status.json 引用（用于读写 config.vectorDimension 等字段）
         * @param {object} callbacks 回调集合
         * @param {function} callbacks.onTokenUsed token 跟踪回调 ({type, model, inputTokens, outputTokens, totalTokens, paragraphIndex, step})
         * @param {function} callbacks.onConfirm 用户确认弹窗回调 (title, message) => Promise<boolean>
         * @param {function} callbacks.onToast 提示回调 (message, level)
         */
        constructor(statusRef, callbacks) {
            this.status = statusRef;
            this.callbacks = callbacks || {};
            this._dimensionDetected = false;
        }

        /** ==================== 嵌入调用 ==================== */

        /**
         * 调用 /v1/embeddings 嵌入文本，返回向量与 token 消耗。
         *
         * @param {string} text 待嵌入文本
         * @param {object} meta token 跟踪元信息 {step, paragraphIndex}
         * @returns {Promise<object>} {embedding, tokens}
         */
        async embed(text, meta) {
            if (!text || !text.trim()) {
                return { embedding: null, tokens: 0 };
            }
            const response = await fetch('/v1/embeddings', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    model: EMBEDDING_MODEL,
                    input: text
                })
            });
            if (!response.ok) {
                const errText = await response.text();
                throw new Error('嵌入调用失败: ' + response.status + ' ' + errText);
            }
            const data = await response.json();
            const embedding = (data && data.data && data.data[0] && data.data[0].embedding) || null;
            const usage = (data && data.usage) || {};
            const inputTokens = usage.prompt_tokens || usage.input_tokens || 0;

            // token 跟踪
            if (this.callbacks.onTokenUsed) {
                this.callbacks.onTokenUsed({
                    type: 'embedding',
                    model: EMBEDDING_MODEL,
                    inputTokens: inputTokens,
                    outputTokens: 0,
                    totalTokens: inputTokens,
                    paragraphIndex: (meta && meta.paragraphIndex !== undefined) ? meta.paragraphIndex : -1,
                    step: (meta && meta.step) || 0
                });
            }

            // 首次嵌入后探测维度
            if (embedding && !this._dimensionDetected) {
                this._dimensionDetected = true;
                if (!this.status.config.vectorDimension || this.status.config.vectorDimension === 0) {
                    this.status.config.vectorDimension = embedding.length;
                }
            }

            // 维度一致性校验
            if (embedding && this.status.config.vectorDimension && this.status.config.vectorDimension !== embedding.length) {
                throw new Error(
                    '嵌入维度不一致: 配置维度 ' + this.status.config.vectorDimension +
                    ', 实际维度 ' + embedding.length + '。请检查嵌入模型配置或清空向量存储。'
                );
            }

            return { embedding: embedding, tokens: inputTokens };
        }

        /**
         * 探测嵌入维度（首次启动调用）。
         *
         * @returns {Promise<number>} 维度
         */
        async detectDimension() {
            const result = await this.embed('维度探测', { step: 0, paragraphIndex: -3 });
            this._dimensionDetected = true;
            return result.embedding ? result.embedding.length : 0;
        }

        /** ==================== 文件读写 ==================== */

        /**
         * 读取向量文件。
         *
         * @param {string} tableName 表名（story_points / chapter_summaries）
         * @returns {Promise<object>} 文件内容 {model, dimension, items: []}
         */
        async loadFile(tableName) {
            const fileName = TABLE_FILES[tableName];
            if (!fileName) throw new Error('未知表名: ' + tableName);
            const url = '/file/read/package/' + VECTOR_STORE_DIR + '/' + fileName;
            try {
                const response = await fetch(url);
                if (!response.ok) {
                    if (response.status === 404) return null; // 文件不存在视为空表
                    throw new Error('读取向量文件失败: ' + response.status);
                }
                const text = await response.text();
                if (!text || !text.trim()) return null;
                return JSON.parse(text);
            } catch (e) {
                if (this.callbacks.onToast) this.callbacks.onToast('读取向量文件失败: ' + e.message, 'warning');
                return null;
            }
        }

        /**
         * 保存向量文件。
         *
         * @param {string} tableName 表名
         * @param {object} data 文件内容
         */
        async saveFile(tableName, data) {
            const fileName = TABLE_FILES[tableName];
            if (!fileName) throw new Error('未知表名: ' + tableName);
            // 确保 dimension/model 一致
            data.model = EMBEDDING_MODEL;
            data.dimension = this.status.config.vectorDimension || (data.items[0] && data.items[0].embedding ? data.items[0].embedding.length : 0);

            const response = await fetch('/file/write', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-File-Name': VECTOR_STORE_DIR + '/' + fileName
                },
                body: JSON.stringify(data, null, 2)
            });
            if (!response.ok) {
                throw new Error('保存向量文件失败: ' + response.status);
            }
        }

        /**
         * 初始化空表（首次写入前）。
         */
        async ensureFile(tableName) {
            let data = await this.loadFile(tableName);
            if (!data) {
                data = {
                    model: EMBEDDING_MODEL,
                    dimension: this.status.config.vectorDimension || 0,
                    items: []
                };
                await this.saveFile(tableName, data);
            }
            return data;
        }

        /** ==================== 增删查 ==================== */

        /**
         * 添加一条向量记录。
         *
         * @param {string} tableName 表名
         * @param {object} item 记录 {id, content, path, level, title, embedding, createdAt, source, ...}
         */
        async addItem(tableName, item) {
            const data = await this.ensureFile(tableName);
            // 维度校验
            if (data.dimension && item.embedding && data.dimension !== item.embedding.length) {
                throw new Error(
                    '向量维度不匹配: 文件维度 ' + data.dimension +
                    ', 新向量维度 ' + item.embedding.length
                );
            }
            // 自动生成 ID（若未指定）
            if (!item.id) {
                const seq = data.items.length + 1;
                const prefix = tableName === 'story_points' ? 'sp' : (tableName === 'chapter_summaries' ? 'ch' : 'item');
                item.id = prefix + '-' + String(seq).padStart(4, '0');
            }
            if (!item.createdAt) item.createdAt = new Date().toISOString();
            data.items.push(item);
            await this.saveFile(tableName, data);
            return item;
        }

        /**
         * 批量添加向量记录。
         *
         * @param {string} tableName 表名
         * @param {object[]} items 记录数组
         */
        async addItems(tableName, items) {
            if (!items || items.length === 0) return [];
            const data = await this.ensureFile(tableName);
            const added = [];
            for (const item of items) {
                if (data.dimension && item.embedding && data.dimension !== item.embedding.length) {
                    throw new Error('向量维度不匹配: 文件维度 ' + data.dimension + ', 新向量维度 ' + item.embedding.length);
                }
                if (!item.id) {
                    const seq = data.items.length + 1;
                    const prefix = tableName === 'story_points' ? 'sp' : (tableName === 'chapter_summaries' ? 'ch' : 'item');
                    item.id = prefix + '-' + String(seq).padStart(4, '0');
                }
                if (!item.createdAt) item.createdAt = new Date().toISOString();
                data.items.push(item);
                added.push(item);
            }
            await this.saveFile(tableName, data);
            return added;
        }

        /**
         * 删除一条记录。
         *
         * @param {string} tableName 表名
         * @param {string} id 记录 ID
         */
        async deleteItem(tableName, id) {
            const data = await this.loadFile(tableName);
            if (!data) return false;
            const before = data.items.length;
            data.items = data.items.filter(function (item) { return item.id !== id; });
            if (data.items.length === before) return false;
            await this.saveFile(tableName, data);
            return true;
        }

        /**
         * 列出所有记录（不返回 embedding）。
         *
         * @param {string} tableName 表名
         * @returns {Promise<object[]>} 记录数组（不含 embedding）
         */
        async listItems(tableName) {
            const data = await this.loadFile(tableName);
            if (!data) return [];
            return data.items.map(function (item) {
                const copy = Object.assign({}, item);
                delete copy.embedding;
                return copy;
            });
        }

        /**
         * 按 ID 删除指定章节摘要（专用于 chapter_summaries）。
         *
         * @param {string} tableName 表名
         * @param {number} chapterIndex 章节序号
         */
        async deleteByChapterIndex(tableName, chapterIndex) {
            const data = await this.loadFile(tableName);
            if (!data) return false;
            const before = data.items.length;
            data.items = data.items.filter(function (item) { return item.chapterIndex !== chapterIndex; });
            if (data.items.length === before) return false;
            await this.saveFile(tableName, data);
            return true;
        }

        /** ==================== 查询 ==================== */

        /**
         * 语义检索：嵌入 queryText，对表中所有向量计算余弦相似度，返回 top_k。
         *
         * @param {string} tableName 表名
         * @param {string} queryText 查询文本
         * @param {number} topK 返回前 K 条
         * @param {object} meta token 跟踪元信息
         * @returns {Promise<object[]>} [{item, score}]，按 score 降序
         */
        async query(tableName, queryText, topK, meta) {
            const data = await this.loadFile(tableName);
            if (!data || !data.items || data.items.length === 0) return [];

            const embedResult = await this.embed(queryText, meta);
            if (!embedResult.embedding) return [];

            const cosine = global.NovelStudioProFormatters.cosineSimilarity;
            const scored = data.items.map(function (item) {
                return {
                    item: item,
                    score: cosine(embedResult.embedding, item.embedding)
                };
            });
            scored.sort(function (a, b) { return b.score - a.score; });
            return scored.slice(0, topK);
        }

        /**
         * 多表联合查询（如同时查 story_points + chapter_summaries）。
         *
         * @param {string[]} tableNames 表名数组
         * @param {string} queryText 查询文本
         * @param {number} topK 总返回前 K 条
         * @param {object} meta token 跟踪元信息
         * @returns {Promise<object[]>} [{item, score, table}]
         */
        async queryMulti(tableNames, queryText, topK, meta) {
            const allResults = [];
            for (const tableName of tableNames) {
                const results = await this.query(tableName, queryText, topK, meta);
                for (const r of results) {
                    allResults.push({ item: r.item, score: r.score, table: tableName });
                }
            }
            allResults.sort(function (a, b) { return b.score - a.score; });
            return allResults.slice(0, topK);
        }

        /** ==================== 维度校验 ==================== */

        /**
         * 启动时校验所有向量文件的维度一致性。
         * 若发现不一致，弹窗提示用户确认是否清空。
         *
         * @returns {Promise<object>} {valid: boolean, mismatchedFiles: string[]}
         */
        async validateDimension() {
            const configDim = this.status.config.vectorDimension;
            const mismatchedFiles = [];

            for (const tableName of Object.keys(TABLE_FILES)) {
                const data = await this.loadFile(tableName);
                if (!data) continue;
                if (data.dimension && configDim && data.dimension !== configDim) {
                    mismatchedFiles.push({ tableName: tableName, dimension: data.dimension, itemCount: data.items.length });
                }
            }

            if (mismatchedFiles.length === 0) {
                return { valid: true, mismatchedFiles: [] };
            }

            // 弹窗确认
            if (this.callbacks.onConfirm) {
                const messages = mismatchedFiles.map(function (m) {
                    return '• ' + m.tableName + '.json: 文件维度 ' + m.dimension + ', 含 ' + m.itemCount + ' 条向量';
                });
                const msg = '检测到嵌入模型变更（当前配置维度 ' + configDim + '），以下向量文件维度不一致：\n\n' +
                    messages.join('\n') + '\n\n清空后需重新录入知识点与摘要。是否清空这些文件？';
                const confirmed = await this.callbacks.onConfirm('向量维度不一致', msg);
                if (confirmed) {
                    for (const m of mismatchedFiles) {
                        await this.saveFile(m.tableName, {
                            model: EMBEDDING_MODEL,
                            dimension: configDim,
                            items: []
                        });
                    }
                    if (this.callbacks.onToast) this.callbacks.onToast('向量文件已清空，请重新录入知识点', 'info');
                    return { valid: true, mismatchedFiles: mismatchedFiles, cleared: true };
                } else {
                    // 用户拒绝清空，标记应用为维度冲突状态
                    this.status.config.vectorDimensionMismatch = true;
                    if (this.callbacks.onToast) this.callbacks.onToast('向量检索已禁用，请手动清空向量文件后重启', 'warning');
                    return { valid: false, mismatchedFiles: mismatchedFiles, cleared: false };
                }
            }
            return { valid: false, mismatchedFiles: mismatchedFiles, cleared: false };
        }

        /** ==================== 统计 ==================== */

        /**
         * 返回各表的统计信息。
         */
        async getStats() {
            const stats = {};
            for (const tableName of Object.keys(TABLE_FILES)) {
                const data = await this.loadFile(tableName);
                stats[tableName] = {
                    dimension: data ? data.dimension : 0,
                    count: data ? data.items.length : 0,
                    model: data ? data.model : EMBEDDING_MODEL
                };
            }
            return stats;
        }
    }

    // 暴露到全局
    global.VectorStore = VectorStore;
    global.VECTOR_STORE_TABLES = TABLE_FILES;
    global.EMBEDDING_MODEL_NAME = EMBEDDING_MODEL;
})(typeof window !== 'undefined' ? window : this);
