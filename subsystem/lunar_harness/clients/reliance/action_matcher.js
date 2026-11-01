// ==== action_matcher.js — 前端动作匹配 ====
// 后端只推送消息，不再计算动作；
// 引擎收到 context 文本推送后，按 50% 概率调用嵌入模型，在自己的动作列表里
// 选出语义最接近的一个并播放。嵌入失败时退回字面包含匹配，避免动作完全失效。
//
// 用法：const matcher = new LunarActionMatcher({ endpoint: '/agent/embeddings' });
//       matcher.consider(text, names, name => playAction(name));

(function () {
    const DEFAULT_MIN_SIMILARITY = 0.30;
    const DEFAULT_TRIGGER_RATE = 0.5;

    class LunarActionMatcher {
        constructor(options = {}) {
            this.endpoint = options.endpoint || '/agent/embeddings';
            this.triggerRate = typeof options.triggerRate === 'number'
                ? options.triggerRate
                : DEFAULT_TRIGGER_RATE;
            this.minSimilarity = typeof options.minSimilarity === 'number'
                ? options.minSimilarity
                : DEFAULT_MIN_SIMILARITY;
            this.timeoutMs = options.timeoutMs || 15000;
            this.cache = { signature: '', vectors: [] };
            this.running = false;
        }

        /** 每条消息独立按概率决定是否触发 */
        shouldTrigger() {
            return Math.random() < this.triggerRate;
        }

        /**
         * 处理一条推送消息：命中概率时匹配动作并回调。
         * @param {string} text 消息正文
         * @param {string[]} names 当前可用动作名
         * @param {(name: string) => void} onMatch 匹配成功回调
         * @returns {Promise<string>} 匹配到的动作名（未触发或未命中为空串）
         */
        async consider(text, names, onMatch) {
            if (this.running) return '';
            if (!text || !Array.isArray(names) || names.length === 0) return '';
            if (!this.shouldTrigger()) return '';
            this.running = true;
            try {
                const name = await this.match(text, names);
                if (name && typeof onMatch === 'function') onMatch(name);
                return name;
            } finally {
                this.running = false;
            }
        }

        /** 嵌入匹配：动作名作为文档、消息正文作为查询，取余弦相似度最高者 */
        async match(text, names) {
            const vectors = await this.documentVectors(names);
            if (!vectors || vectors.length !== names.length) {
                return lexicalMatch(text, names);
            }
            const query = await this.embed([text]);
            if (!query.length || !query[0].length) {
                return lexicalMatch(text, names);
            }
            let bestName = '';
            let bestScore = -2;
            for (let i = 0; i < vectors.length; i++) {
                const score = cosine(query[0], vectors[i]);
                if (score > bestScore) {
                    bestScore = score;
                    bestName = names[i];
                }
            }
            if (bestName && bestScore >= this.minSimilarity) {
                return bestName;
            }
            return lexicalMatch(text, names);
        }

        /** 动作名向量缓存（动作列表变化时自动重建） */
        async documentVectors(names) {
            const signature = names.join('\u0000');
            if (this.cache.signature === signature && this.cache.vectors.length === names.length) {
                return this.cache.vectors;
            }
            const vectors = await this.embed(names);
            if (vectors.length === names.length) {
                this.cache = { signature, vectors };
            }
            return vectors;
        }

        /** 调用后端嵌入端点，返回每条文本的向量 */
        async embed(inputs) {
            const controller = new AbortController();
            const timer = setTimeout(() => controller.abort(), this.timeoutMs);
            try {
                const resp = await fetch(this.endpoint, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ input: inputs }),
                    signal: controller.signal,
                });
                if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
                const json = await resp.json();
                const data = Array.isArray(json.data) ? json.data : [];
                return data
                    .map(item => (Array.isArray(item && item.embedding) ? item.embedding : []))
                    .filter(vector => vector.length > 0);
            } catch (err) {
                console.warn('[动作匹配] 嵌入请求失败:', (err && err.message) || err);
                return [];
            } finally {
                clearTimeout(timer);
            }
        }
    }

    /** 余弦相似度（长度不一致时视为不可比较） */
    function cosine(a, b) {
        if (!a || !b || a.length !== b.length || a.length === 0) return -2;
        let dot = 0;
        let normA = 0;
        let normB = 0;
        for (let i = 0; i < a.length; i++) {
            dot += a[i] * b[i];
            normA += a[i] * a[i];
            normB += b[i] * b[i];
        }
        if (normA === 0 || normB === 0) return 0;
        return dot / (Math.sqrt(normA) * Math.sqrt(normB));
    }

    /** 字面兜底：完全相同优先，其次互相包含，取更长者 */
    function lexicalMatch(text, names) {
        const query = String(text || '').trim().toLowerCase();
        if (!query) return '';
        let best = '';
        for (const name of names) {
            const candidate = String(name || '').trim().toLowerCase();
            if (!candidate) continue;
            if (candidate === query) return name;
            if (query.includes(candidate) || candidate.includes(query)) {
                if (String(name).length > String(best).length) best = name;
            }
        }
        return best;
    }

    if (typeof window !== 'undefined') {
        window.LunarActionMatcher = LunarActionMatcher;
    }
})();
