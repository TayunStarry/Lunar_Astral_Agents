// ---------- WebSocket 配置 ----------
const WS_URL = (() => {
    if (window.location.hostname && window.location.hostname !== '127.0.0.1' && window.location.hostname !== 'localhost') {
        return `ws://${window.location.host}/ws`;
    }
    return 'ws://localhost:36797/ws';
})();
const MAX_RECONNECT_ATTEMPTS = 10;
const RECONNECT_BASE_DELAY = 1500;
const USER_NAME = '你';
const ASSISTANT_NAME = '月华';

// DOM 元素
const messageArea = document.getElementById('messageArea');
const emptyState = document.getElementById('emptyState');
const messageInput = document.getElementById('messageInput');
const sendButton = document.getElementById('sendButton');
const toastContainer = document.getElementById('toastContainer');
const dragOverlay = document.getElementById('dragOverlay');
const appContainer = document.getElementById('appContainer');
const statusDot = document.getElementById('statusDot');
const statusTextSpan = document.getElementById('statusText');

// 状态变量
let messageIdCounter = 0;
let ws = null;
let reconnectAttempts = 0;
let reconnectTimer = null;
let manualClose = false;
let mermaidInitialized = false;
let dragCounter = 0;

// ---------- 辅助函数 ----------
function generateMessageId() {
    messageIdCounter++;
    return `msg-${Date.now()}-${messageIdCounter}-${Math.random().toString(36).slice(2, 8)}`;
}

function getTimeString() {
    return new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

function showToast(msg, type = 'info') {
    const toast = document.createElement('div');
    toast.className = 'toast';
    const icons = { info: 'fa-circle-info', success: 'fa-check-circle', error: 'fa-exclamation-triangle' };
    toast.innerHTML = `<i class="fas ${icons[type]}" style="margin-right:8px;"></i>${escapeHtml(msg)}`;
    toastContainer.appendChild(toast);
    setTimeout(() => toast.remove(), 2800);
}

function updateEmptyState() {
    const messages = messageArea.querySelectorAll('.message');
    emptyState.classList.toggle('hidden', messages.length !== 0);
}

function scrollToBottom(smooth = true) {
    messageArea.scrollTo({ top: messageArea.scrollHeight, behavior: smooth ? 'smooth' : 'instant' });
}

async function copyToClipboard(text) {
    try {
        await navigator.clipboard.writeText(text);
        return true;
    } catch {
        return false;
    }
}

function updateConnectionStatusUI(connected) {
    if (statusDot && statusTextSpan) {
        if (connected) {
            statusDot.classList.add('connected');
            statusTextSpan.textContent = '已连接';
        } else {
            statusDot.classList.remove('connected');
            statusTextSpan.textContent = '未连接';
        }
    }
}

// ---------- Markdown 与渲染库初始化 ----------
if (typeof marked !== 'undefined') {
    marked.setOptions({
        breaks: true,
        gfm: true,
        highlight: function (code, lang) {
            return code;
        }
    });
}

function initMermaid() {
    if (mermaidInitialized) return;
    if (typeof mermaid !== 'undefined') {
        mermaid.initialize({ startOnLoad: false, theme: 'default', securityLevel: 'loose' });
        mermaidInitialized = true;
    }
}

function highlightCodeInContainer(container) {
    if (typeof hljs === 'undefined') return;
    const blocks = container.querySelectorAll('pre code');
    blocks.forEach(block => {
        if (block.parentElement.classList.contains('hljs')) return;
        const languageClass = Array.from(block.classList).find(c => c.startsWith('language-'));
        if (languageClass) {
            const lang = languageClass.replace('language-', '');
            if (lang === 'echarts' || lang === 'mermaid') return;
        }
        try {
            hljs.highlightElement(block);
        } catch (e) {
            console.warn('代码高亮失败', e);
        }
    });
}

function renderEChartsInContainer(container) {
    if (typeof echarts === 'undefined') return;
    const blocks = container.querySelectorAll('code.language-echarts');
    blocks.forEach(block => {
        try {
            const cleanCode = block.textContent.trim().replace(/\/\/.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '');
            let config = JSON.parse(cleanCode);
            if (!config.series) config.series = [{ type: 'line', data: [12, 28, 35, 42] }];
            const chartDiv = document.createElement('div');
            chartDiv.className = 'echarts-container';
            block.parentNode.replaceChild(chartDiv, block);
            const chart = echarts.init(chartDiv);
            chart.setOption(config);
            chartDiv._echartsInstance = chart;
            window.addEventListener('resize', () => chart.resize());
        } catch (err) {
            console.warn('echarts渲染出错', err);
        }
    });
}

async function renderMermaidInContainer(container) {
    if (typeof mermaid === 'undefined' || !mermaidInitialized) return;
    const blocks = container.querySelectorAll('code.language-mermaid');
    for (const block of Array.from(blocks)) {
        const textContent = block.textContent || '';
        if (textContent.length <= 20) continue;

        let svg;
        try {
            await mermaid.parse(textContent);
            const id = `mermaid-${Date.now()}-${Math.floor(Math.random() * 10000)}`;
            ({ svg } = await mermaid.render(id, textContent));
        } catch (e) {
            console.error('Mermaid parse/render error:', e, '\nSource:', textContent);
            continue; // 跳过此图表
        }

        // viewBox 调整，若出错则直接使用原始 svg
        let finalSvg = svg;
        try {
            const parser = new DOMParser();
            const doc = parser.parseFromString(svg, 'image/svg+xml');
            const svgEl = doc.documentElement;
            const chartType = svgEl.getAttribute('aria-roledescription');
            if (chartType === 'flowchart' || chartType === 'classDiagram') {
                const vb = svgEl.getAttribute('viewBox');
                if (vb) {
                    const v = vb.split(/\s+/).map(parseFloat);
                    if (v.length === 4 && v.every(n => !isNaN(n))) {
                        if (chartType === 'flowchart') {
                            v[0] *= 0.45; v[1] *= 0.45;
                            v[2] *= 1.05; v[3] *= 1.05;
                        } else {
                            v[0] *= 0; v[1] *= 0.35;
                            v[2] *= 1.05; v[3] *= 1.25;
                        }
                        svgEl.setAttribute('viewBox', v.join(' '));
                    }
                }
                finalSvg = new XMLSerializer().serializeToString(svgEl);
            }
        } catch (adjustErr) {
            console.warn('ViewBox adjust failed, using raw SVG:', adjustErr);
        }

        const wrapper = document.createElement('div');
        wrapper.className = 'mermaid-container';
        wrapper.innerHTML = `<div style="width:100%; border:none; padding:0">${finalSvg}</div>`;
        const parent = block.parentElement;
        if (parent) {
            parent.insertBefore(wrapper, block);
            parent.removeChild(block);
        }
    }
}

function renderKaTeX(container) {
    if (typeof window.renderMathInElement === 'function') {
        window.renderMathInElement(container, {
            delimiters: [
                { left: '$$', right: '$$', display: true },
                { left: '$', right: '$', display: false },
                { left: '\\(', right: '\\)', display: false },
                { left: '\\[', right: '\\]', display: true }
            ],
            throwOnError: false
        });
    }
}

function processThinkTags(html) {
    return html.replace(/<think>([\s\S]*?)<\/think>/gi, (match, content) => {
        return `<div class="think-block"><div class="think-summary" onclick="this.parentElement.classList.toggle('open')"><i class="fas fa-chevron-right toggle-icon"></i> 思考过程</div><div class="think-content">${content}</div></div>`;
    });
}

async function renderContentAsync(rawContent) {
    if (!rawContent) return '';
    let withThink = processThinkTags(rawContent);
    if (typeof marked !== 'undefined') {
        withThink = await marked.parse(withThink);
    } else {
        withThink = '<p>' + escapeHtml(withThink).replace(/\n/g, '<br>') + '</p>';
    }
    return withThink;
}

// ---------- 消息操作绑定（复制返回原始内容） ----------
function bindMessageActions(msgEl, rawContent) {
    const copyBtn = msgEl.querySelector('[data-action="copy"]');
    if (copyBtn) {
        copyBtn.addEventListener('click', async () => {
            // 优先使用保存的原始内容，次选文本内容
            let text = rawContent || '';
            if (!text) {
                const contentDiv = msgEl.querySelector('.markdown-content');
                if (contentDiv) text = contentDiv.textContent;
            }
            const success = await copyToClipboard(text);
            showToast(success ? '消息已复制' : '复制失败', success ? 'success' : 'error');
        });
    }
    const delBtn = msgEl.querySelector('[data-action="delete"]');
    if (delBtn) {
        delBtn.addEventListener('click', () => {
            msgEl.style.opacity = '0';
            msgEl.style.transform = 'scale(0.95)';
            msgEl.style.transition = '0.2s';
            msgEl.addEventListener('transitionend', () => {
                msgEl.remove();
                updateEmptyState();
            }, { once: true });
            showToast('消息已删除', 'info');
        });
    }
}

// 创建消息骨架，并存储原始内容
function createMessageElementSkeleton({ id, role, imageSrc, isFile = false, borderColor = null, content = '' }) {
    const el = document.createElement('div');
    el.className = `message ${role === 'user' ? 'user-message' : 'assistant-message'}`;
    if (isFile && borderColor) el.classList.add('file-message');
    const displayName = role === 'user' ? USER_NAME : ASSISTANT_NAME;
    const time = getTimeString();

    if (imageSrc) {
        const safeSrc = imageSrc.replace(/\\/g, '/');
        el.innerHTML = `
            <div class="message-header">
                <span>${escapeHtml(displayName)}</span>
                <span style="font-size:0.7rem;">${time}</span>
            </div>
            <div class="labeled-image-container" style="--image-label: '${escapeHtml('图片文件')}';">
                <img src="${safeSrc}" alt="image" style="border-color: ${borderColor || '#5b6cd4'};" onclick="if(window.previewImage) previewImage('${safeSrc}','')" loading="lazy">
            </div>
            <div class="message-actions-panel">
                <button class="chat-action-button copy_message_button" data-action="copy" title="复制"><i class="fas fa-copy"></i></button>
                <button class="chat-action-button delete_message_button" data-action="delete"><i class="fas fa-trash"></i></button>
            </div>
        `;
    } else {
        el.innerHTML = `
            <div class="message-header">
                <span>${escapeHtml(displayName)}</span>
                <span style="font-size:0.7rem;">${time}</span>
            </div>
            <div class="markdown-content"></div>
            <div class="message-actions-panel">
                <button class="chat-action-button copy_message_button" data-action="copy"><i class="fas fa-copy"></i></button>
                <button class="chat-action-button delete_message_button" data-action="delete"><i class="fas fa-trash"></i></button>
            </div>
        `;
    }
    // 关键：将原始内容存储到 dataset 中
    el.dataset.rawContent = content || '';
    bindMessageActions(el, content || '');
    return el;
}

// 填充消息内容（异步）
async function populateMessageContent(msgEl, content, role) {
    if (!content) return;
    const contentDiv = msgEl.querySelector('.markdown-content');
    if (contentDiv) {
        contentDiv.innerHTML = '<i class="fas fa-spinner fa-pulse"></i> 加载中...';
        const html = await renderContentAsync(content);
        contentDiv.innerHTML = html;

        highlightCodeInContainer(contentDiv);
        renderEChartsInContainer(contentDiv);
        await renderMermaidInContainer(contentDiv);
        renderKaTeX(contentDiv);
    }
}

// 添加消息到界面（统一入口）
async function addMessageToArea(opts) {
    const { id, role, content, imageSrc, isFile = false, borderColor = null } = opts;
    const msgEl = createMessageElementSkeleton({ id, role, imageSrc, isFile, borderColor, content });
    messageArea.appendChild(msgEl);
    updateEmptyState();
    scrollToBottom(true);

    if (!imageSrc && content) {
        await populateMessageContent(msgEl, content, role);
        scrollToBottom(true);
    }
    return msgEl;
}

function handleWebSocketMessage(data) {
    const msgType = data.type || '';
    if (msgType === 'context' && data.data?.content) {
        addMessageToArea({ id: generateMessageId(), role: 'assistant', content: String(data.data.content) });
        return;
    }
    if (msgType === 'image' && (data.data?.images || data.images)) {
        const images = data.data?.images || data.images;
        images.forEach(img => {
            let src = (img.startsWith('data:') || img.startsWith('http')) ? img : 'data:image/png;base64,' + img;
            addMessageToArea({ id: generateMessageId(), role: 'assistant', content: data.data?.prompt || '', imageSrc: src, borderColor: '#7b8cd6' });
        });
        return;
    }
    if (data.content) {
        addMessageToArea({ id: generateMessageId(), role: 'assistant', content: String(data.content) });
        return;
    }
    addMessageToArea({ id: generateMessageId(), role: 'assistant', content: '```json\n' + JSON.stringify(data, null, 2) + '\n```' });
}

// WebSocket 连接管理
function connectWebSocket() {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return;
    try {
        ws = new WebSocket(WS_URL);
    } catch (err) {
        scheduleReconnect();
        return;
    }
    ws.onopen = () => {
        reconnectAttempts = 0;
        showToast('WebSocket 已连接', 'success');
        updateConnectionStatusUI(true);
    };
    ws.onmessage = (event) => {
        try {
            const parsed = JSON.parse(event.data);
            handleWebSocketMessage(parsed);
        } catch {
            addMessageToArea({ id: generateMessageId(), role: 'assistant', content: event.data });
        }
    };
    ws.onerror = () => {
        updateConnectionStatusUI(false);
    };
    ws.onclose = () => {
        updateConnectionStatusUI(false);
        if (!manualClose) scheduleReconnect();
    };
}

function scheduleReconnect() {
    if (manualClose) return;
    if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
        showToast('重连失败，请刷新页面', 'error');
        return;
    }
    const delay = RECONNECT_BASE_DELAY * Math.pow(1.5, reconnectAttempts);
    reconnectTimer = setTimeout(() => {
        reconnectAttempts++;
        connectWebSocket();
    }, delay);
}

function sendLocalMessage() {
    const text = messageInput.value.trim();
    if (!text) return;
    addMessageToArea({ id: generateMessageId(), role: 'user', content: text });
    messageInput.value = '';
    messageInput.style.height = 'auto';
    messageInput.focus();
    scrollToBottom(true);
}

async function handleDroppedFiles(files) {
    for (const file of files) {
        if (file.type.startsWith('image/')) {
            const dataUrl = await new Promise(resolve => {
                const reader = new FileReader();
                reader.onload = () => resolve(reader.result);
                reader.readAsDataURL(file);
            });
            addMessageToArea({ id: generateMessageId(), role: 'user', content: file.name, imageSrc: dataUrl, isFile: true, borderColor: '#5b6cd4' });
        } else {
            const text = await file.text();
            const preview = text.slice(0, 50000);
            addMessageToArea({ id: generateMessageId(), role: 'user', content: `**📄 ${escapeHtml(file.name)}**\n\`\`\`\n${preview}\n\`\`\``, isFile: true, borderColor: '#9080e0' });
        }
    }
}

function setupDragEvents() {
    document.addEventListener('dragenter', (e) => {
        dragCounter++;
        if (dragCounter === 1 && e.dataTransfer?.types?.includes('Files')) dragOverlay.classList.add('active');
    });
    document.addEventListener('dragleave', () => {
        dragCounter--;
        if (dragCounter === 0) dragOverlay.classList.remove('active');
    });
    document.addEventListener('dragover', (e) => e.preventDefault());
    document.addEventListener('drop', (e) => {
        e.preventDefault();
        dragCounter = 0;
        dragOverlay.classList.remove('active');
        const files = e.dataTransfer?.files;
        if (files && files.length) handleDroppedFiles(files);
    });
}

function setupInputEvents() {
    messageInput.addEventListener('input', function () {
        this.style.height = 'auto';
        this.style.height = Math.min(this.scrollHeight, 200) + 'px';
    });
    messageInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            sendLocalMessage();
        }
    });
    sendButton.addEventListener('click', sendLocalMessage);
}

function setupImagePreview() {
    messageArea.addEventListener('click', (e) => {
        const img = e.target.closest('img');
        if (img && typeof previewImage === 'function') {
            const src = img.getAttribute('src');
            const alt = img.getAttribute('alt') || '图片';
            previewImage(src, alt);
        }
    });
}

function cleanup() {
    manualClose = true;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    if (ws) {
        ws.onclose = null;
        ws.close();
        ws = null;
    }
    document.querySelectorAll('.echarts-container').forEach(c => {
        if (c._echartsInstance) c._echartsInstance.dispose();
    });
}

function init() {
    initMermaid();
    setupDragEvents();
    setupInputEvents();
    setupImagePreview();
    updateEmptyState();
    connectWebSocket();
    window.addEventListener('beforeunload', cleanup);
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}