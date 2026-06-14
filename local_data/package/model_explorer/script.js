// ==== 全局状态 ====
let configData = {};
let currentModels = [];
let originalModels = [];
let isConnecting = false;
let chatMessages = [];
let isChatting = false;
let isDarkMode = false;

// 默认本地端口
const DEFAULT_LOCAL_URL = 'http://127.0.0.1:36789/v1';

// ==== DOM 元素 ====
const elements = {
    // 导航
    navStatus: document.getElementById('nav-status'),
    btnTheme: document.getElementById('btn-theme'),
    // 配置页
    configUrl: document.getElementById('config-url'),
    configKey: document.getElementById('config-key'),
    configMultimodal: document.getElementById('config-multimodal'),
    configEmbedding: document.getElementById('config-embedding'),
    apiBaseUrl: document.getElementById('api-base-url'),
    apiKey: document.getElementById('api-key'),
    requestTimeout: document.getElementById('request-timeout'),
    maxRetries: document.getElementById('max-retries'),
    testConnectionBtn: document.getElementById('test-connection-btn'),
    queryModelsBtn: document.getElementById('query-models-btn'),
    resetFormBtn: document.getElementById('reset-form-btn'),
    toggleKeyVisibility: document.getElementById('toggle-key-visibility'),
    // 模型列表页
    modelListContainer: document.getElementById('model-list-container'),
    modelCount: document.getElementById('model-count'),
    tabModelCount: document.getElementById('tab-model-count'),
    filterBar: document.getElementById('filter-bar'),
    filterToggleBtn: document.getElementById('filter-toggle-btn'),
    modelSearchInput: document.getElementById('model-search-input'),
    modelOwnerFilter: document.getElementById('model-owner-filter'),
    modelSortSelect: document.getElementById('model-sort-select'),
    exportJsonBtn: document.getElementById('export-json-btn'),
    // 对话页
    chatModelSelect: document.getElementById('chat-model-select'),
    chatMessagesContainer: document.getElementById('chat-messages-container'),
    chatInput: document.getElementById('chat-input'),
    chatSendBtn: document.getElementById('chat-send-btn'),
    clearChatBtn: document.getElementById('clear-chat-btn'),
    // Toast
    toastContainer: document.getElementById('toast-container')
};

// ==== 初始化 ====
function init() {
    bindEvents();
    loadTheme();
    loadConfig();
    loadSavedConnection();
}

// ==== 事件绑定 ====
function bindEvents() {
    // 导航标签切换
    document.querySelectorAll('.nav-tab').forEach(tab => {
        tab.addEventListener('click', switchPage);
    });

    // 主题切换
    elements.btnTheme.addEventListener('click', toggleTheme);

    // 连接配置
    elements.testConnectionBtn.addEventListener('click', testConnection);
    elements.queryModelsBtn.addEventListener('click', queryModels);
    elements.resetFormBtn.addEventListener('click', resetForm);
    elements.toggleKeyVisibility.addEventListener('click', toggleKeyVisibility);

    // 输入时保存配置
    elements.apiBaseUrl.addEventListener('input', saveConnectionConfig);
    elements.apiKey.addEventListener('input', saveConnectionConfig);
    elements.requestTimeout.addEventListener('change', saveConnectionConfig);
    elements.maxRetries.addEventListener('change', saveConnectionConfig);

    // 筛选
    elements.filterToggleBtn.addEventListener('click', toggleFilterBar);
    elements.modelSearchInput.addEventListener('input', filterAndRenderModels);
    elements.modelOwnerFilter.addEventListener('change', filterAndRenderModels);
    elements.modelSortSelect.addEventListener('change', filterAndRenderModels);
    elements.exportJsonBtn.addEventListener('click', exportModelsJson);

    // 对话
    elements.chatSendBtn.addEventListener('click', sendChatMessage);
    elements.clearChatBtn.addEventListener('click', clearChat);
    elements.chatInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            sendChatMessage();
        }
    });

    // 全局快捷键
    document.addEventListener('keydown', handleKeyboardShortcuts);
}

// ==== 配置加载 ====
async function loadConfig() {
    try {
        const response = await fetch('/file/read/local_data/lunar_config.json');
        if (response.ok) {
            configData = await response.json();
        }
    } catch (e) {
        console.log('加载配置文件失败:', e);
    }
    updateConfigDisplay();
}

function updateConfigDisplay() {
    const cloud = configData.cloud || {};
    const url = cloud.cloud_model_url || null;
    const key = cloud.cloud_model_key || null;
    const multimodal = cloud.multimodal_model_name || null;
    const embedding = cloud.embedding_model_name || null;

    setConfigValue(elements.configUrl, url, DEFAULT_LOCAL_URL);
    setConfigValue(elements.configKey, key, null, true);
    setConfigValue(elements.configMultimodal, multimodal, 'system-multimodal');
    setConfigValue(elements.configEmbedding, embedding, 'system-embedding');

    // 如果自定义输入框为空，自动填充云端配置
    if (!elements.apiBaseUrl.value && url) {
        elements.apiBaseUrl.value = url;
    }
    if (!elements.apiKey.value && key) {
        elements.apiKey.value = key;
    }
}

function setConfigValue(el, value, fallback, isSecret = false) {
    if (value) {
        el.textContent = isSecret ? maskSecret(value) : value;
        el.classList.remove('empty');
    } else if (fallback) {
        el.textContent = (isSecret ? '' : fallback) + ' (默认)';
        el.classList.add('empty');
    } else {
        el.textContent = '未配置';
        el.classList.add('empty');
    }
}

function maskSecret(secret) {
    if (!secret || secret.length <= 8) return '••••••••';
    return secret.slice(0, 4) + '••••' + secret.slice(-4);
}

// ==== 获取有效连接参数 ====
function getEffectiveConnection() {
    const customUrl = elements.apiBaseUrl.value.trim();
    const customKey = elements.apiKey.value.trim();
    const cloud = configData.cloud || {};

    return {
        baseUrl: customUrl || cloud.cloud_model_url || DEFAULT_LOCAL_URL,
        apiKey: customKey || cloud.cloud_model_key || ''
    };
}

// ==== 页面切换 ====
function switchPage(e) {
    let navBtn = e.target;
    while (navBtn && !navBtn.dataset.page) { navBtn = navBtn.parentElement; }
    if (!navBtn) return;

    const pageName = navBtn.dataset.page;
    document.querySelectorAll('.nav-tab').forEach(btn => btn.classList.remove('active'));
    navBtn.classList.add('active');
    document.querySelectorAll('.app-page').forEach(page => page.classList.remove('active'));
    document.querySelector(`.app-page[data-page="${pageName}"]`).classList.add('active');

    // 切换到对话页时填充模型选择器
    if (pageName === 'chat') {
        populateChatModelSelect();
    }
}

// ==== 主题切换 ====
function toggleTheme() {
    isDarkMode = !isDarkMode;
    document.body.classList.toggle('dark-mode', isDarkMode);
    const icon = elements.btnTheme.querySelector('i');
    icon.className = isDarkMode ? 'fas fa-sun' : 'fas fa-moon';
    localStorage.setItem('model_explorer_theme', isDarkMode ? 'dark' : 'light');
}

function loadTheme() {
    const saved = localStorage.getItem('model_explorer_theme');
    if (saved === 'dark') {
        isDarkMode = true;
        document.body.classList.add('dark-mode');
        const icon = elements.btnTheme.querySelector('i');
        icon.className = 'fas fa-sun';
    }
}

// ==== 连接配置持久化 ====
function saveConnectionConfig() {
    const config = {
        baseUrl: elements.apiBaseUrl.value.trim(),
        apiKey: elements.apiKey.value.trim(),
        timeout: elements.requestTimeout.value,
        maxRetries: elements.maxRetries.value
    };
    localStorage.setItem('model_explorer_connection', JSON.stringify(config));
}

function loadSavedConnection() {
    try {
        const saved = localStorage.getItem('model_explorer_connection');
        if (saved) {
            const config = JSON.parse(saved);
            if (config.baseUrl) elements.apiBaseUrl.value = config.baseUrl;
            if (config.apiKey) elements.apiKey.value = config.apiKey;
            if (config.timeout) elements.requestTimeout.value = config.timeout;
            if (config.maxRetries) elements.maxRetries.value = config.maxRetries;
        }
    } catch (e) {
        console.log('加载保存的连接配置失败:', e);
    }
}

// ==== API Key 显示/隐藏 ====
function toggleKeyVisibility() {
    const isPassword = elements.apiKey.type === 'password';
    elements.apiKey.type = isPassword ? 'text' : 'password';
    const icon = elements.toggleKeyVisibility.querySelector('i');
    icon.className = isPassword ? 'fas fa-eye-slash' : 'fas fa-eye';
}

// ==== 连接状态更新 ====
function updateConnectionStatus(status, message) {
    const dot = elements.navStatus.querySelector('.status-dot');
    const text = elements.navStatus.querySelector('.status-text');
    dot.className = 'status-dot';

    if (status === 'connected') {
        dot.classList.add('status-dot-active');
        text.textContent = message || '已连接';
    } else if (status === 'connecting') {
        dot.classList.add('status-dot-connecting');
        text.textContent = message || '连接中...';
    } else {
        dot.classList.add('status-dot-inactive');
        text.textContent = message || '未连接';
    }
}

// ==== 测试连接 ====
async function testConnection() {
    const { baseUrl, apiKey } = getEffectiveConnection();

    if (!baseUrl) {
        showToast('请配置 API 基础地址', 'error');
        return;
    }

    setButtonsLoading(true);
    updateConnectionStatus('connecting', '测试连接中...');

    try {
        const timeout = parseInt(elements.requestTimeout.value) || 30;
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), timeout * 1000);

        const response = await fetch('/api/proxy/models', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ base_url: baseUrl, api_key: apiKey }),
            signal: controller.signal
        });

        clearTimeout(timeoutId);

        if (response.ok) {
            const data = await response.json();
            if (data.success && data.data) {
                const count = data.data.data ? data.data.data.length : 0;
                updateConnectionStatus('connected', '连接成功');
                showToast(`连接成功! 发现 ${count} 个模型`, 'success');
            } else {
                updateConnectionStatus('inactive', '连接失败');
                showToast(`连接失败: ${data.error || '未知错误'}`, 'error');
            }
        } else {
            const errorData = await response.json().catch(() => null);
            updateConnectionStatus('inactive', '连接失败');
            showToast(`连接失败: ${errorData?.error || response.statusText}`, 'error');
        }
    } catch (error) {
        updateConnectionStatus('inactive', '连接失败');
        if (error.name === 'AbortError') {
            showToast(`请求超时 (${elements.requestTimeout.value}秒)`, 'error');
        } else {
            showToast(`连接错误: ${error.message}`, 'error');
        }
    } finally {
        setButtonsLoading(false);
    }
}

// ==== 查询模型 ====
async function queryModels() {
    const { baseUrl, apiKey } = getEffectiveConnection();

    if (!baseUrl) {
        showToast('请配置 API 基础地址', 'error');
        return;
    }

    if (isConnecting) {
        showToast('正在请求中，请稍候...', 'info');
        return;
    }

    isConnecting = true;
    setButtonsLoading(true);
    elements.queryModelsBtn.innerHTML = '<span class="loading-spinner"></span> 查询中...';
    updateConnectionStatus('connecting', '查询模型中...');

    const maxRetries = parseInt(elements.maxRetries.value) || 0;
    let success = false;

    for (let attempt = 0; attempt <= maxRetries; attempt++) {
        if (attempt > 0) {
            showToast(`重试中... (${attempt}/${maxRetries})`, 'info');
            await sleep(1000 * attempt);
        }

        try {
            const timeout = parseInt(elements.requestTimeout.value) || 30;
            const controller = new AbortController();
            const timeoutId = setTimeout(() => controller.abort(), timeout * 1000);

            const response = await fetch('/api/proxy/models', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ base_url: baseUrl, api_key: apiKey }),
                signal: controller.signal
            });

            clearTimeout(timeoutId);

            if (!response.ok) {
                const errorData = await response.json().catch(() => null);
                throw new Error(`HTTP ${response.status}: ${errorData?.error || response.statusText}`);
            }

            const data = await response.json();

            if (data.success && data.data && data.data.data && Array.isArray(data.data.data)) {
                originalModels = [...data.data.data];
                populateOwnerFilter();
                filterAndRenderModels();

                // 自动切换到模型列表页
                const modelsTab = document.querySelector('.nav-tab[data-page="models"]');
                if (modelsTab) modelsTab.click();

                updateConnectionStatus('connected', `已加载 ${data.data.data.length} 个模型`);
                showToast(`查询成功! 共找到 ${data.data.data.length} 个模型`, 'success');
                success = true;
                break;
            } else {
                throw new Error(data.error || '响应格式无效');
            }
        } catch (error) {
            console.error(`查询失败 (尝试 ${attempt + 1}):`, error);
            if (error.name === 'AbortError') {
                showToast(`请求超时 (${elements.requestTimeout.value}秒)`, 'error');
            } else if (attempt === maxRetries) {
                updateConnectionStatus('inactive', '查询失败');
                showToast(`查询失败: ${error.message}`, 'error');
            }
        }
    }

    isConnecting = false;
    setButtonsLoading(false);
    elements.queryModelsBtn.innerHTML = '<i class="fas fa-search"></i> 查询模型';
}

// ==== 渲染模型列表 ====
function renderModels(models) {
    if (!models || models.length === 0) {
        elements.modelListContainer.innerHTML = `
            <div class="empty-state">
                <div class="empty-state-icon"><i class="fas fa-search"></i></div>
                <h3>没有找到匹配的模型</h3>
                <p>尝试调整筛选条件</p>
            </div>`;
        return;
    }

    const modelsHTML = models.map(model => {
        const modelId = model.id || 'unknown';
        const owner = model.owned_by || 'unknown';
        const created = model.created ? formatTimestamp(model.created) : '未知';

        return `
            <div class="model-card">
                <div class="model-card-header">
                    <div class="model-icon-wrapper">
                        <i class="fas fa-cube"></i>
                    </div>
                    <div class="model-info">
                        <div class="model-id" title="${escapeAttr(modelId)}">${escapeHtml(modelId)}</div>
                        <div class="model-owner">
                            <i class="fas fa-user"></i> ${escapeHtml(owner)}
                        </div>
                    </div>
                </div>
                <div class="model-meta">
                    <span class="model-meta-item">
                        <i class="fas fa-calendar"></i> ${created}
                    </span>
                </div>
                <div class="model-actions">
                    <button class="model-btn model-btn-copy" onclick="copyModelId('${escapeJs(modelId)}')">
                        <i class="fas fa-copy"></i> 复制
                    </button>
                    <button class="model-btn model-btn-chat" onclick="chatWithModel('${escapeJs(modelId)}')">
                        <i class="fas fa-comment"></i> 对话
                    </button>
                    <button class="model-btn model-btn-detail" onclick="showModelDetail('${escapeJs(modelId)}')">
                        <i class="fas fa-info-circle"></i> 详情
                    </button>
                </div>
            </div>`;
    }).join('');

    elements.modelListContainer.innerHTML = `<div class="model-grid">${modelsHTML}</div>`;
    elements.modelCount.textContent = `${models.length} 个模型`;

    // 更新标签页徽标
    elements.tabModelCount.textContent = models.length;
    elements.tabModelCount.style.display = models.length > 0 ? 'inline-flex' : 'none';
}

// ==== 筛选与渲染 ====
function filterAndRenderModels() {
    let filtered = [...originalModels];

    const searchTerm = elements.modelSearchInput.value.trim().toLowerCase();
    if (searchTerm) {
        filtered = filtered.filter(model => {
            const id = (model.id || '').toLowerCase();
            const owner = (model.owned_by || '').toLowerCase();
            return id.includes(searchTerm) || owner.includes(searchTerm);
        });
    }

    const ownerFilter = elements.modelOwnerFilter.value;
    if (ownerFilter) {
        filtered = filtered.filter(model => model.owned_by === ownerFilter);
    }

    const sortValue = elements.modelSortSelect.value;
    filtered.sort((a, b) => {
        switch (sortValue) {
            case 'id-asc': return (a.id || '').localeCompare(b.id || '');
            case 'id-desc': return (b.id || '').localeCompare(a.id || '');
            case 'owner-asc': return (a.owned_by || '').localeCompare(b.owned_by || '');
            case 'owner-desc': return (b.owned_by || '').localeCompare(a.owned_by || '');
            default: return 0;
        }
    });

    currentModels = filtered;
    renderModels(filtered);
}

// ==== 所有者过滤器 ====
function populateOwnerFilter() {
    const owners = new Set();
    originalModels.forEach(model => {
        if (model.owned_by) owners.add(model.owned_by);
    });

    const currentValue = elements.modelOwnerFilter.value;
    elements.modelOwnerFilter.innerHTML = '<option value="">全部所有者</option>';
    Array.from(owners).sort().forEach(owner => {
        const option = document.createElement('option');
        option.value = owner;
        option.textContent = owner;
        elements.modelOwnerFilter.appendChild(option);
    });
    elements.modelOwnerFilter.value = currentValue;
}

// ==== 筛选栏切换 ====
function toggleFilterBar() {
    const bar = elements.filterBar;
    const isVisible = bar.classList.contains('visible');
    bar.classList.toggle('visible', !isVisible);
    const icon = elements.filterToggleBtn.querySelector('i');
    icon.className = isVisible ? 'fas fa-filter' : 'fas fa-times';
}

// ==== 导出 JSON ====
function exportModelsJson() {
    if (originalModels.length === 0) {
        showToast('没有可导出的模型数据', 'error');
        return;
    }

    const data = JSON.stringify(originalModels, null, 2);
    const blob = new Blob([data], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
    link.download = `models_${timestamp}.json`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    showToast(`已导出 ${originalModels.length} 个模型`, 'success');
}

// ==== 复制模型 ID ====
function copyModelId(modelId) {
    navigator.clipboard.writeText(modelId).then(() => {
        showToast(`已复制: ${modelId}`, 'success');
    }).catch(() => {
        const textarea = document.createElement('textarea');
        textarea.value = modelId;
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
        showToast(`已复制: ${modelId}`, 'success');
    });
}

// ==== 跳转到对话 ====
function chatWithModel(modelId) {
    const chatTab = document.querySelector('.nav-tab[data-page="chat"]');
    if (chatTab) chatTab.click();

    // 等待 DOM 更新后设置选中
    setTimeout(() => {
        elements.chatModelSelect.value = modelId;
        if (!elements.chatModelSelect.value) {
            // 如果精确匹配失败，尝试模糊匹配
            const options = elements.chatModelSelect.options;
            for (let i = 0; i < options.length; i++) {
                if (options[i].value === modelId || options[i].textContent === modelId) {
                    elements.chatModelSelect.selectedIndex = i;
                    break;
                }
            }
        }
        elements.chatInput.focus();
    }, 100);
}

// ==== 模型详情弹窗 ====
function showModelDetail(modelId) {
    const model = originalModels.find(m => m.id === modelId);
    if (!model) {
        showToast('未找到模型信息', 'error');
        return;
    }

    const detail = JSON.stringify(model, null, 2);
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.innerHTML = `
        <div class="modal-content glass-panel">
            <div class="modal-header">
                <h3><i class="fas fa-cube"></i> ${escapeHtml(modelId)}</h3>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">
                    <i class="fas fa-times"></i>
                </button>
            </div>
            <div class="modal-body">
                <pre class="model-detail-json">${escapeHtml(detail)}</pre>
            </div>
        </div>`;

    modal.addEventListener('click', (e) => {
        if (e.target === modal) modal.remove();
    });

    document.body.appendChild(modal);
}

// ==== 重置表单 ====
function resetForm() {
    elements.apiBaseUrl.value = '';
    elements.apiKey.value = '';
    elements.requestTimeout.value = '30';
    elements.maxRetries.value = '1';
    localStorage.removeItem('model_explorer_connection');
    updateConnectionStatus('inactive', '未连接');
    showToast('配置已重置', 'info');
}

// ==== 按钮加载状态 ====
function setButtonsLoading(loading) {
    elements.testConnectionBtn.disabled = loading;
    elements.queryModelsBtn.disabled = loading;
}

// ==== 对话功能 ====
function populateChatModelSelect() {
    const currentValue = elements.chatModelSelect.value;
    elements.chatModelSelect.innerHTML = '<option value="">选择模型...</option>';
    originalModels.forEach(model => {
        const option = document.createElement('option');
        option.value = model.id || model.name;
        option.textContent = model.id || model.name;
        elements.chatModelSelect.appendChild(option);
    });
    elements.chatModelSelect.value = currentValue;
}

async function sendChatMessage() {
    const message = elements.chatInput.value.trim();
    if (!message) return;

    const model = elements.chatModelSelect.value;
    if (!model) {
        showToast('请选择一个模型', 'error');
        return;
    }

    const { baseUrl, apiKey } = getEffectiveConnection();
    if (!baseUrl) {
        showToast('API 基础地址不能为空', 'error');
        return;
    }

    chatMessages.push({ role: 'user', content: message });
    elements.chatInput.value = '';
    renderChatMessages();
    showTypingIndicator();

    if (isChatting) {
        showToast('正在等待回复...', 'info');
        return;
    }

    isChatting = true;
    elements.chatSendBtn.disabled = true;

    try {
        const timeout = parseInt(elements.requestTimeout.value) || 60;
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), timeout * 1000);

        const response = await fetch('/api/proxy/chat', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                base_url: baseUrl,
                api_key: apiKey,
                model: model,
                messages: [...chatMessages]
            }),
            signal: controller.signal
        });

        clearTimeout(timeoutId);
        removeTypingIndicator();

        if (!response.ok) {
            const errorData = await response.json().catch(() => null);
            throw new Error(`HTTP ${response.status}: ${errorData?.error || response.statusText}`);
        }

        const data = await response.json();

        if (data.success && data.data && data.data.choices && data.data.choices.length > 0) {
            const assistantMessage = data.data.choices[0].message;
            chatMessages.push({ role: 'assistant', content: assistantMessage.content || '' });
            renderChatMessages();
        } else {
            throw new Error(data.error || '无效的响应格式');
        }
    } catch (error) {
        removeTypingIndicator();
        if (error.name === 'AbortError') {
            showToast(`请求超时 (${elements.requestTimeout.value}秒)`, 'error');
        } else {
            showToast(`发送失败: ${error.message}`, 'error');
        }
        // 发送失败时移除最后一条用户消息
        if (chatMessages.length > 0 && chatMessages[chatMessages.length - 1].role === 'user') {
            chatMessages.pop();
            elements.chatInput.value = message;
            renderChatMessages();
        }
    } finally {
        isChatting = false;
        elements.chatSendBtn.disabled = false;
    }
}

function renderChatMessages() {
    if (chatMessages.length === 0) {
        elements.chatMessagesContainer.innerHTML = `
            <div class="chat-welcome">
                <div class="chat-welcome-icon"><i class="fas fa-robot"></i></div>
                <h3>开始对话</h3>
                <p>选择一个模型，然后发送消息开始对话</p>
            </div>`;
        return;
    }

    const messagesHTML = chatMessages.map(msg => {
        const isUser = msg.role === 'user';
        const avatar = isUser ? '<i class="fas fa-user"></i>' : '<i class="fas fa-robot"></i>';
        const roleClass = isUser ? 'user' : 'assistant';
        const formattedContent = formatChatContent(msg.content);

        return `
            <div class="chat-message chat-message-${roleClass}">
                <div class="chat-avatar">${avatar}</div>
                <div class="chat-bubble">
                    <div class="chat-role-name">${isUser ? '你' : 'AI'}</div>
                    <div class="chat-content">${formattedContent}</div>
                </div>
            </div>`;
    }).join('');

    elements.chatMessagesContainer.innerHTML = messagesHTML;
    elements.chatMessagesContainer.scrollTop = elements.chatMessagesContainer.scrollHeight;
}

function formatChatContent(content) {
    if (!content) return '';
    let formatted = escapeHtml(content);
    formatted = formatted.replace(/\n/g, '<br>');
    return formatted;
}

function showTypingIndicator() {
    const indicator = document.createElement('div');
    indicator.className = 'chat-message chat-message-assistant chat-typing';
    indicator.id = 'typing-indicator';
    indicator.innerHTML = `
        <div class="chat-avatar"><i class="fas fa-robot"></i></div>
        <div class="chat-bubble">
            <div class="chat-role-name">AI</div>
            <div class="typing-dots"><span></span><span></span><span></span></div>
        </div>`;
    elements.chatMessagesContainer.appendChild(indicator);
    elements.chatMessagesContainer.scrollTop = elements.chatMessagesContainer.scrollHeight;
}

function removeTypingIndicator() {
    const indicator = document.getElementById('typing-indicator');
    if (indicator) indicator.remove();
}

function clearChat() {
    chatMessages = [];
    renderChatMessages();
    showToast('对话已清空', 'info');
}

// ==== Toast 提示 ====
function showToast(message, type = 'info', duration = 3000) {
    const toast = document.createElement('div');
    toast.className = `toast-glass toast-${type}`;

    const iconMap = {
        success: '<i class="fas fa-check-circle"></i>',
        error: '<i class="fas fa-times-circle"></i>',
        info: '<i class="fas fa-info-circle"></i>',
        warning: '<i class="fas fa-exclamation-circle"></i>'
    };

    toast.innerHTML = `
        <span class="toast-icon toast-${type}">${iconMap[type] || iconMap.info}</span>
        <span class="toast-message">${message}</span>
        <button class="toast-close" onclick="this.parentElement.remove()"><i class="fas fa-times"></i></button>`;

    elements.toastContainer.appendChild(toast);

    if (duration > 0) {
        setTimeout(() => {
            toast.classList.add('toast-exit');
            setTimeout(() => toast.remove(), 300);
        }, duration);
    }
}

// ==== 键盘快捷键 ====
function handleKeyboardShortcuts(e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault();
        queryModels();
    }
    if (e.key === 'Escape') {
        document.querySelectorAll('.modal-overlay').forEach(modal => modal.remove());
    }
}

// ==== 工具函数 ====
function formatTimestamp(timestamp) {
    const date = new Date(timestamp * 1000);
    return date.toLocaleString('zh-CN', {
        year: 'numeric', month: '2-digit', day: '2-digit',
        hour: '2-digit', minute: '2-digit'
    });
}

function escapeHtml(str) {
    if (!str) return '';
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

function escapeAttr(str) {
    return escapeHtml(str);
}

function escapeJs(str) {
    if (!str) return '';
    return str.replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/"/g, '\\"');
}

function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

// ==== 启动 ====
document.addEventListener('DOMContentLoaded', init);
