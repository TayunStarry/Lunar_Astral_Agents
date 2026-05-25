// 全局变量
let currentModels = [];
let originalModels = [];
let isConnecting = false;
let chatMessages = [];
let isChatting = false;

// DOM元素
const elements = {
    apiBaseUrl: document.getElementById('api-base-url'),
    apiKey: document.getElementById('api-key'),
    requestTimeout: document.getElementById('request-timeout'),
    maxRetries: document.getElementById('max-retries'),
    testConnectionBtn: document.getElementById('test-connection-btn'),
    queryModelsBtn: document.getElementById('query-models-btn'),
    resetFormBtn: document.getElementById('reset-form-btn'),
    toggleKeyVisibility: document.getElementById('toggle-key-visibility'),
    connectionStatus: document.getElementById('connection-status'),
    statusText: document.getElementById('status-text'),
    pageConnection: document.getElementById('page-connection'),
    pageResults: document.getElementById('page-results'),
    pageChat: document.getElementById('page-chat'),
    modelListContainer: document.getElementById('model-list-container'),
    modelCount: document.getElementById('model-count'),
    filterBar: document.getElementById('filter-bar'),
    filterInputToggle: document.getElementById('filter-input-toggle'),
    modelSearchInput: document.getElementById('model-search-input'),
    modelOwnerFilter: document.getElementById('model-owner-filter'),
    modelSortSelect: document.getElementById('model-sort-select'),
    exportJsonBtn: document.getElementById('export-json-btn'),
    backToConnectionBtn: document.getElementById('back-to-connection-btn'),
    switchToChatBtn: document.getElementById('switch-to-chat-btn'),
    backToModelsBtn: document.getElementById('back-to-models-btn'),
    chatModelSelect: document.getElementById('chat-model-select'),
    chatMessagesContainer: document.getElementById('chat-messages-container'),
    chatInput: document.getElementById('chat-input'),
    chatSendBtn: document.getElementById('chat-send-btn'),
    clearChatBtn: document.getElementById('clear-chat-btn'),
    quickQueryBtn: document.getElementById('quick-query-btn'),
    quickTestBtn: document.getElementById('quick-test-btn'),
    refreshBtn: document.getElementById('refresh-btn'),
    toastContainer: document.getElementById('toast-container'),
    bottomActionBar: document.getElementById('bottom-action-bar')
};

// 初始化事件监听
function initEventListeners() {
    elements.testConnectionBtn.addEventListener('click', testConnection);
    elements.queryModelsBtn.addEventListener('click', queryModels);
    elements.resetFormBtn.addEventListener('click', resetForm);
    elements.toggleKeyVisibility.addEventListener('click', toggleKeyVisibility);
    elements.quickQueryBtn.addEventListener('click', queryModels);
    elements.quickTestBtn.addEventListener('click', testConnection);
    elements.refreshBtn.addEventListener('click', refreshPage);
    elements.backToConnectionBtn.addEventListener('click', switchToConnection);
    elements.filterInputToggle.addEventListener('click', toggleFilterBar);
    elements.exportJsonBtn.addEventListener('click', exportModelsJson);
    elements.switchToChatBtn.addEventListener('click', switchToChat);
    elements.backToModelsBtn.addEventListener('click', switchToResults);
    elements.chatSendBtn.addEventListener('click', sendChatMessage);
    elements.clearChatBtn.addEventListener('click', clearChat);

    elements.modelSearchInput.addEventListener('input', filterAndRenderModels);
    elements.modelOwnerFilter.addEventListener('change', filterAndRenderModels);
    elements.modelSortSelect.addEventListener('change', filterAndRenderModels);

    elements.chatInput.addEventListener('keydown', (e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
            e.preventDefault();
            sendChatMessage();
        }
    });

    document.addEventListener('keydown', handleKeyboardShortcuts);

    elements.apiBaseUrl.addEventListener('input', saveConnectionConfig);
    elements.apiKey.addEventListener('input', saveConnectionConfig);

    loadConnectionConfig();
}

// 保存连接配置到 localStorage
function saveConnectionConfig() {
    const config = {
        baseUrl: elements.apiBaseUrl.value.trim(),
        apiKey: elements.apiKey.value.trim(),
        timeout: elements.requestTimeout.value,
        maxRetries: elements.maxRetries.value
    };
    localStorage.setItem('model_query_config', JSON.stringify(config));
}

// 加载连接配置
function loadConnectionConfig() {
    try {
        const saved = localStorage.getItem('model_query_config');
        if (saved) {
            const config = JSON.parse(saved);
            if (config.baseUrl) elements.apiBaseUrl.value = config.baseUrl;
            if (config.apiKey) elements.apiKey.value = config.apiKey;
            if (config.timeout) elements.requestTimeout.value = config.timeout;
            if (config.maxRetries) elements.maxRetries.value = config.maxRetries;
        }
    } catch (e) {
        console.log('加载配置失败:', e);
    }
}

// 切换 API Key 显示/隐藏
function toggleKeyVisibility() {
    const isPassword = elements.apiKey.type === 'password';
    elements.apiKey.type = isPassword ? 'text' : 'password';
    const icon = elements.toggleKeyVisibility.querySelector('i');
    icon.className = isPassword ? 'fas fa-eye-slash' : 'fas fa-eye';
}

// 更新连接状态显示
function updateConnectionStatus(status, message) {
    elements.connectionStatus.className = 'status-dot';
    if (status === 'connected') {
        elements.connectionStatus.classList.add('status-dot-active');
        elements.statusText.textContent = message || '已连接';
    } else if (status === 'connecting') {
        elements.connectionStatus.classList.add('status-dot-connecting');
        elements.statusText.textContent = message || '连接中...';
    } else {
        elements.connectionStatus.classList.add('status-dot-inactive');
        elements.statusText.textContent = message || '未连接';
    }
}

// 测试连接
async function testConnection() {
    const baseUrl = elements.apiBaseUrl.value.trim();
    const apiKey = elements.apiKey.value.trim();

    if (!baseUrl) {
        showToast('请输入 API 基础地址', 'error');
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
            body: JSON.stringify({
                base_url: baseUrl,
                api_key: apiKey
            }),
            signal: controller.signal
        });

        clearTimeout(timeoutId);

        if (response.ok) {
            const data = await response.json();
            if (data.success && data.data) {
                updateConnectionStatus('connected', '连接成功');
                const modelCount = data.data.data ? data.data.data.length : 0;
                showToast(`连接成功! 发现 ${modelCount} 个模型`, 'success');
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

// 查询模型
async function queryModels() {
    const baseUrl = elements.apiBaseUrl.value.trim();
    const apiKey = elements.apiKey.value.trim();

    if (!baseUrl) {
        showToast('请输入 API 基础地址', 'error');
        return;
    }

    if (isConnecting) {
        showToast('正在请求中，请稍候...', 'info');
        return;
    }

    isConnecting = true;
    setButtonsLoading(true);
    elements.queryModelsBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 查询中...';
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
                body: JSON.stringify({
                    base_url: baseUrl,
                    api_key: apiKey
                }),
                signal: controller.signal
            });

            clearTimeout(timeoutId);

            if (!response.ok) {
                const errorData = await response.json().catch(() => null);
                throw new Error(`HTTP ${response.status}: ${errorData?.error || response.statusText}`);
            }

            const data = await response.json();

            if (data.success && data.data && data.data.data && Array.isArray(data.data.data)) {
                currentModels = [...data.data.data];
                originalModels = [...data.data.data];
                populateOwnerFilter();
                filterAndRenderModels();
                switchToResults();
                updateConnectionStatus('connected', `已加载 ${data.data.data.length} 个模型`);
                showToast(`查询成功! 共找到 ${data.data.data.length} 个模型`, 'success');
                success = true;
                break;
            } else {
                throw new Error(data.error || '响应格式无效，未找到模型数据');
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

// 渲染模型列表
function renderModels(models) {
    if (!models || models.length === 0) {
        elements.modelListContainer.innerHTML = `
            <div class="empty-state">
                <div class="empty-state-icon"><i class="fas fa-search"></i></div>
                <p>没有找到匹配的模型</p>
                <p style="font-size: 0.9em; margin-top: 10px; color: #888;">尝试调整筛选条件</p>
            </div>`;
        return;
    }

    const modelsHTML = models.map(model => {
        const modelId = model.id || 'unknown';
        const owner = model.owned_by || 'unknown';
        const created = model.created ? formatTimestamp(model.created) : '未知';
        const permissions = model.permission ? model.permission.length : 0;

        return `
            <div class="model-card">
                <div class="model-card-header">
                    <div class="model-icon-wrapper">
                        <i class="fas fa-cube"></i>
                    </div>
                    <div class="model-info">
                        <div class="model-id" title="${modelId}">${modelId}</div>
                        <div class="model-owner">
                            <i class="fas fa-user"></i> ${owner}
                        </div>
                    </div>
                </div>
                <div class="model-meta">
                    <span class="model-meta-item">
                        <i class="fas fa-calendar"></i>
                        <span>创建时间: ${created}</span>
                    </span>
                    <span class="model-meta-item">
                        <i class="fas fa-shield-alt"></i>
                        <span>权限数: ${permissions}</span>
                    </span>
                </div>
                <div class="model-actions">
                    <button class="model-btn model-btn-copy" onclick="copyModelId('${modelId.replace(/'/g, "\\'")}')">
                        <i class="fas fa-copy"></i> 复制 ID
                    </button>
                    <button class="model-btn model-btn-detail" onclick="showModelDetail('${modelId.replace(/'/g, "\\'")}')">
                        <i class="fas fa-info-circle"></i> 详情
                    </button>
                </div>
            </div>`;
    }).join('');

    elements.modelListContainer.innerHTML = `
        <div class="model-grid">
            ${modelsHTML}
        </div>`;

    elements.modelCount.textContent = `${models.length} 个模型`;
}

// 筛选和渲染模型
function filterAndRenderModels() {
    let filtered = [...originalModels];

    // 搜索过滤
    const searchTerm = elements.modelSearchInput.value.trim().toLowerCase();
    if (searchTerm) {
        filtered = filtered.filter(model => {
            const id = (model.id || '').toLowerCase();
            const owner = (model.owned_by || '').toLowerCase();
            return id.includes(searchTerm) || owner.includes(searchTerm);
        });
    }

    // 所有者过滤
    const ownerFilter = elements.modelOwnerFilter.value;
    if (ownerFilter) {
        filtered = filtered.filter(model => model.owned_by === ownerFilter);
    }

    // 排序
    const sortValue = elements.modelSortSelect.value;
    filtered.sort((a, b) => {
        switch (sortValue) {
            case 'id-asc':
                return (a.id || '').localeCompare(b.id || '');
            case 'id-desc':
                return (b.id || '').localeCompare(a.id || '');
            case 'owner-asc':
                return (a.owned_by || '').localeCompare(b.owned_by || '');
            case 'owner-desc':
                return (b.owned_by || '').localeCompare(a.owned_by || '');
            default:
                return 0;
        }
    });

    currentModels = filtered;
    renderModels(filtered);
}

// 填充所有者过滤器
function populateOwnerFilter() {
    const owners = new Set();
    originalModels.forEach(model => {
        if (model.owned_by) {
            owners.add(model.owned_by);
        }
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

// 页面切换
function switchToResults() {
    elements.pageConnection.classList.add('hidden');
    elements.pageResults.classList.remove('hidden');
}

function switchToConnection() {
    elements.pageResults.classList.add('hidden');
    elements.pageConnection.classList.remove('hidden');
}

// 切换筛选栏
function toggleFilterBar() {
    const isVisible = elements.filterBar.style.display !== 'none';
    elements.filterBar.style.display = isVisible ? 'none' : 'flex';
    elements.filterInputToggle.querySelector('i').className = isVisible ? 'fas fa-filter' : 'fas fa-filter fa-rotate-180';
}

// 导出模型 JSON
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

// 复制模型 ID
function copyModelId(modelId) {
    navigator.clipboard.writeText(modelId).then(() => {
        showToast(`已复制: ${modelId}`, 'success');
    }).catch(() => {
        // 降级方案
        const textarea = document.createElement('textarea');
        textarea.value = modelId;
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
        showToast(`已复制: ${modelId}`, 'success');
    });
}

// 显示模型详情
function showModelDetail(modelId) {
    const model = originalModels.find(m => m.id === modelId);
    if (!model) {
        showToast('未找到模型信息', 'error');
        return;
    }

    const detail = JSON.stringify(model, null, 2);
    showToast(`模型详情 (长度: ${detail.length} 字符)`, 'info');
    console.log('模型详情:', model);

    // 可以在这里添加弹窗显示
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.innerHTML = `
        <div class="modal-content glass-panel">
            <div class="modal-header">
                <h3><i class="fas fa-cube"></i> 模型详情</h3>
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

// 重置表单
function resetForm() {
    if (confirm('确定要重置所有配置吗？')) {
        elements.apiBaseUrl.value = '';
        elements.apiKey.value = '';
        elements.requestTimeout.value = '30';
        elements.maxRetries.value = '1';
        localStorage.removeItem('model_query_config');
        updateConnectionStatus('inactive', '未连接');
        showToast('配置已重置', 'info');
    }
}

// 刷新页面
function refreshPage() {
    location.reload();
}

// 设置按钮加载状态
function setButtonsLoading(loading) {
    elements.testConnectionBtn.disabled = loading;
    elements.queryModelsBtn.disabled = loading;
}

// 格式化时间戳
function formatTimestamp(timestamp) {
    const date = new Date(timestamp * 1000);
    return date.toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
    });
}

// 格式化 JSON
function escapeHtml(str) {
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

// 显示提示消息
function showToast(message, type = 'info', duration = 3000) {
    const toast = document.createElement('div');
    toast.className = `toast-glass toast-${type}`;

    const iconMap = {
        success: '<i class="fas fa-check-circle"></i>',
        error: '<i class="fas fa-times-circle"></i>',
        info: '<i class="fas fa-info-circle"></i>'
    };

    toast.innerHTML = `
        <span class="toast-icon">${iconMap[type]}</span>
        <span class="toast-message">${message}</span>
        <button class="toast-close" onclick="this.parentElement.remove()"><i class="fas fa-times"></i></button>
    `;

    elements.toastContainer.appendChild(toast);

    if (duration > 0) {
        setTimeout(() => {
            toast.classList.add('toast-exit');
            setTimeout(() => toast.remove(), 300);
        }, duration);
    }
}

// 键盘快捷键
function handleKeyboardShortcuts(e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault();
        queryModels();
    }
    if (e.key === 'Escape') {
        const modals = document.querySelectorAll('.modal-overlay');
        modals.forEach(modal => modal.remove());
        const toasts = document.querySelectorAll('.toast-glass');
        toasts.forEach(toast => {
            toast.classList.add('toast-exit');
            setTimeout(() => toast.remove(), 300);
        });
    }
}

// 辅助函数：延迟
function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

// 页面切换：到对话页面
function switchToChat() {
    if (originalModels.length === 0) {
        showToast('请先查询模型', 'error');
        return;
    }
    populateChatModelSelect();
    elements.pageConnection.classList.add('hidden');
    elements.pageResults.classList.add('hidden');
    elements.pageChat.classList.remove('hidden');
}

// 填充聊天模型选择器
function populateChatModelSelect() {
    const currentValue = elements.chatModelSelect.value;
    elements.chatModelSelect.innerHTML = '<option value="">选择模型...</option>';
    originalModels.forEach(model => {
        const modelName = model.name || model.id;
        const option = document.createElement('option');
        option.value = modelName;
        option.textContent = modelName;
        elements.chatModelSelect.appendChild(option);
    });
    elements.chatModelSelect.value = currentValue;
}

// 发送聊天消息
async function sendChatMessage() {
    const message = elements.chatInput.value.trim();
    if (!message) {
        showToast('请输入消息', 'info');
        return;
    }

    const model = elements.chatModelSelect.value;
    if (!model) {
        showToast('请选择一个模型', 'error');
        return;
    }

    const baseUrl = elements.apiBaseUrl.value.trim();
    const apiKey = elements.apiKey.value.trim();

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
        // 移除最后一条用户消息（发送失败时）
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

// 渲染聊天消息
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

// 格式化聊天内容（简单的换行和代码块处理）
function formatChatContent(content) {
    if (!content) return '';
    let formatted = escapeHtml(content);
    formatted = formatted.replace(/\n/g, '<br>');
    return formatted;
}

// 显示正在输入指示器
function showTypingIndicator() {
    const indicator = document.createElement('div');
    indicator.className = 'chat-message chat-message-assistant chat-typing';
    indicator.id = 'typing-indicator';
    indicator.innerHTML = `
        <div class="chat-avatar"><i class="fas fa-robot"></i></div>
        <div class="chat-bubble">
            <div class="chat-role-name">AI</div>
            <div class="typing-dots">
                <span></span><span></span><span></span>
            </div>
        </div>`;
    elements.chatMessagesContainer.appendChild(indicator);
    elements.chatMessagesContainer.scrollTop = elements.chatMessagesContainer.scrollHeight;
}

// 移除正在输入指示器
function removeTypingIndicator() {
    const indicator = document.getElementById('typing-indicator');
    if (indicator) indicator.remove();
}

// 清空聊天
function clearChat() {
    chatMessages = [];
    renderChatMessages();
    showToast('对话已清空', 'info');
}

// 页面切换函数
function switchToConnection() {
    elements.pageResults.classList.add('hidden');
    elements.pageChat.classList.add('hidden');
    elements.pageConnection.classList.remove('hidden');
}

// 初始化
document.addEventListener('DOMContentLoaded', initEventListeners);
