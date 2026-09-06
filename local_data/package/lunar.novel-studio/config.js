/**
 * AI 配置管理模块
 * 从 lunar_config.json 的 agent 分组读取多模态/嵌入模型配置，
 * 封装聊天/嵌入 API 调用。
 *
 * AI 调用始终走同源代理 /v1/ 路由，避免 CORS 问题。
 */

(function(global) {
    'use strict';

    // ==== 默认 agent 配置（读取失败时兜底） ====
    var DEFAULT_AGENT_CONFIG = {
        multimodal_model: 'system-multimodal',
        multimodal_url: 'http://127.0.0.1:36789/v1',
        multimodal_key: '',
        embedding_model: 'system-embedding',
        embedding_url: 'http://127.0.0.1:36789/v1',
        embedding_key: ''
    };

    // ==== 同源代理路径 ====
    var PROXY_CHAT_URL = '/v1/chat/completions';
    var PROXY_EMBED_URL = '/v1/embeddings';

    // ==== ConfigManager 类 ====
    class ConfigManager {
        constructor(app) {
            this.app = app;
            this.agentConfig = null;
        }

        // ==== 从 lunar_config.json 读取 agent 配置 ====
        async loadAgentConfig() {
            try {
                var resp = await fetch('/file/read/lunar_config.json');
                if (!resp.ok) throw new Error('读取配置文件失败: ' + resp.status);
                var data = await resp.json();
                this.agentConfig = Object.assign({}, DEFAULT_AGENT_CONFIG, data.agent || {});
            } catch (e) {
                console.warn('AI 配置加载失败，使用默认配置:', e);
                this.agentConfig = Object.assign({}, DEFAULT_AGENT_CONFIG);
            }
        }

        // ==== 获取多模态模型配置（保持 {url,name,key} 兼容形态） ====
        getConfig() {
            var cfg = this.agentConfig || DEFAULT_AGENT_CONFIG;
            return {
                url: cfg.multimodal_url || '',
                name: cfg.multimodal_model || 'system-multimodal',
                key: cfg.multimodal_key || ''
            };
        }

        // ==== 获取嵌入模型名 ====
        getEmbeddingModel() {
            var cfg = this.agentConfig || DEFAULT_AGENT_CONFIG;
            return cfg.embedding_model || 'system-embedding';
        }

        // ==== 聊天 API 调用（始终走同源代理） ====
        // 返回: { content, inputTokens, outputTokens, totalTokens }
        // options.imageUrl 存在时，将图片以多模态 image_url 附加到末条用户消息
        async callChat(messages, options) {
            var config = this.getConfig();
            var opt = options || {};
            var body = {
                model: config.name,
                messages: this._buildMessages(messages, opt.imageUrl),
                temperature: opt.temperature !== undefined ? opt.temperature : 0.7,
                max_tokens: opt.maxTokens || 4096,
                stream: false
            };
            var resp = await fetch(PROXY_CHAT_URL, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body)
            });
            if (!resp.ok) {
                var errText = await resp.text();
                throw new Error('AI 调用失败: ' + resp.status + ' ' + errText);
            }
            var data = await resp.json();
            var content = (data.choices && data.choices[0] && data.choices[0].message && data.choices[0].message.content) || '';
            var usage = data.usage || {};
            return {
                content: content,
                inputTokens: usage.prompt_tokens || 0,
                outputTokens: usage.completion_tokens || 0,
                totalTokens: usage.total_tokens || 0
            };
        }

        // ==== 构造多模态消息 ====
        _buildMessages(messages, imageUrl) {
            if (!imageUrl) return messages;
            return messages.map(function(m, idx) {
                if (m.role === 'user' && idx === messages.length - 1) {
                    return {
                        role: 'user',
                        content: [
                            { type: 'text', text: m.content },
                            { type: 'image_url', image_url: { url: imageUrl } }
                        ]
                    };
                }
                return m;
            });
        }

        // ==== 嵌入 API 调用（始终走同源代理） ====
        // 返回: { embedding, tokens }
        async callEmbed(text) {
            var resp = await fetch(PROXY_EMBED_URL, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ model: this.getEmbeddingModel(), input: text })
            });
            if (!resp.ok) throw new Error('嵌入调用失败: ' + resp.status);
            var data = await resp.json();
            return {
                embedding: (data.data && data.data[0] && data.data[0].embedding) || null,
                tokens: (data.usage && data.usage.prompt_tokens) || 0
            };
        }

        // ==== Token 跟踪记录 ====
        trackToken(tokenInfo) {
            var state = this.app.state;
            state.totalTokens = (state.totalTokens || 0) + (tokenInfo.totalTokens || 0);
            state.callLog.push(Object.assign({
                timestamp: new Date().toISOString(),
                chapterIndex: state.chapterIndex
            }, tokenInfo));
            // 保留最近 500 条
            if (state.callLog.length > 500) state.callLog.shift();
        }
    }

    // ==== 暴露到全局 ====
    global.ConfigManager = ConfigManager;
})(typeof window !== 'undefined' ? window : this);
