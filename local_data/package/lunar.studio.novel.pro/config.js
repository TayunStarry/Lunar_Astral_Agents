/**
 * AI 配置管理模块
 * 管理 AI Name/Key、聊天/嵌入 API 调用封装、记忆库初始化
 * 
 * AI 调用始终走同源代理 /v1/ 路由，避免 CORS 问题
 * 用户配置的 URL 仅用于记忆库 init 的 base_url
 */

(function(global) {
    'use strict';

    // ==== 默认 AI 配置 ====
    var DEFAULT_AI_CONFIG = {
        url: 'http://localhost:36789',
        name: 'system-multimodal',
        key: '2000-0218'
    };

    // ==== 同源代理路径 ====
    var PROXY_CHAT_URL = '/v1/chat/completions';
    var PROXY_EMBED_URL = '/v1/embeddings';

    // ==== ConfigManager 类 ====
    class ConfigManager {
        constructor(app) {
            this.app = app;
            this._configResolve = null;
        }

        // ==== 获取当前 AI 配置 ====
        getConfig() {
            return this.app.state.config.ai;
        }

        // ==== 从 AI URL 中剥离 /v1，供记忆库 init 使用 ====
        // 例: "http://localhost:36789/v1" → "http://localhost:36789"
        // 例: "http://localhost:36789" → "http://localhost:36789"
        getMemoryBaseUrl() {
            return (this.getConfig().url || '').replace(/\/v1\/?$/, '');
        }

        // ==== 聊天 API 调用（始终走同源代理） ====
        // POST /v1/chat/completions
        // 返回: { content, inputTokens, outputTokens, totalTokens }
        async callChat(messages, options) {
            var config = this.getConfig();
            var opt = options || {};
            var body = {
                model: config.name,
                messages: messages,
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

        // ==== 嵌入 API 调用（始终走同源代理） ====
        // POST /v1/embeddings
        // 返回: { embedding, tokens }
        async callEmbed(text) {
            var resp = await fetch(PROXY_EMBED_URL, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ model: 'system-embedding', input: text })
            });
            if (!resp.ok) throw new Error('嵌入调用失败: ' + resp.status);
            var data = await resp.json();
            return {
                embedding: (data.data && data.data[0] && data.data[0].embedding) || null,
                tokens: (data.usage && data.usage.prompt_tokens) || 0
            };
        }

        // ==== 初始化记忆库实例 ====
        // POST /memory/init { base_url, api_key }
        async initMemory() {
            var baseUrl = this.getMemoryBaseUrl();
            var apiKey = this.getConfig().key;
            var resp = await fetch('/memory/init', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ base_url: baseUrl, api_key: apiKey })
            });
            if (!resp.ok) {
                var errText = await resp.text();
                throw new Error('记忆库初始化失败: ' + errText);
            }
            return await resp.json();
        }

        // ==== 确保 AI 已配置（首次进入时弹出配置模态框） ====
        async ensureConfigured() {
            var config = this.getConfig();
            if (!config.url || !config.name) {
                await this.showConfigModal();
            }
        }

        // ==== 显示 AI 配置模态框 ====
        showConfigModal() {
            var self = this;
            return new Promise(function(resolve) {
                var modal = document.getElementById('configModal');
                var urlInput = document.getElementById('aiUrlInput');
                var nameInput = document.getElementById('aiNameInput');
                var keyInput = document.getElementById('aiKeyInput');
                var config = self.getConfig();
                urlInput.value = config.url || DEFAULT_AI_CONFIG.url;
                nameInput.value = config.name || DEFAULT_AI_CONFIG.name;
                keyInput.value = config.key || DEFAULT_AI_CONFIG.key;
                modal.classList.add('active');
                self._configResolve = resolve;
            });
        }

        // ==== 保存 AI 配置（由模态框保存按钮调用） ====
        async saveConfig() {
            var urlInput = document.getElementById('aiUrlInput');
            var nameInput = document.getElementById('aiNameInput');
            var keyInput = document.getElementById('aiKeyInput');
            this.app.state.config.ai = {
                url: urlInput.value.trim() || DEFAULT_AI_CONFIG.url,
                name: nameInput.value.trim() || DEFAULT_AI_CONFIG.name,
                key: keyInput.value.trim() || DEFAULT_AI_CONFIG.key
            };
            await this.app.saveState();
            // 初始化记忆库
            try {
                await this.initMemory();
            } catch (e) {
                console.warn('记忆库初始化失败:', e);
            }
            // 关闭模态框
            document.getElementById('configModal').classList.remove('active');
            if (this._configResolve) {
                this._configResolve();
                this._configResolve = null;
            }
            this.app.showToast('AI 配置已保存');
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
