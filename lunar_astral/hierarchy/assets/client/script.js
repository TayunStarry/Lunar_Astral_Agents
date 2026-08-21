// ============================================================
//  星月智能 · 消息终端 — 前端交互脚本
//  职责：WebSocket 实时收发、消息渲染、TTS 播放、乐谱播放、
//        标签页过滤、全文搜索、文件拖放、消息 JSON 持久化
// ============================================================

// ---------- 常量配置 ----------
const WS_PROTOCOL = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const WS_URL = `${WS_PROTOCOL}//${window.location.hostname}:36789/ws`;
const MAX_RECONNECT_ATTEMPTS = 3;
const RECONNECT_BASE_DELAY = 1500;
const USER_NAME = '你';
const ASSISTANT_NAME = '月华';
const MESSAGES_FILE_PATH = 'lunar_messages.json';
const MAX_PERSISTED_MESSAGES = 200;

// ---------- DOM 引用 ----------
const messageArea = document.getElementById('messageArea');
const emptyState = document.getElementById('emptyState');
const messageInput = document.getElementById('messageInput');
const sendButton = document.getElementById('sendButton');
const toastContainer = document.getElementById('toastContainer');
const dragOverlay = document.getElementById('dragOverlay');
const themeToggle = document.getElementById('themeToggle');
const clearBtn = document.getElementById('clearBtn');
const attachBtn = document.getElementById('attachBtn');
const fileInput = document.getElementById('fileInput');
const tabBar = document.getElementById('tabBar');
const searchInput = document.getElementById('searchInput');
const searchCount = document.getElementById('searchCount');
const searchPrev = document.getElementById('searchPrev');
const searchNext = document.getElementById('searchNext');
const searchClear = document.getElementById('searchClear');
const pendingAttachments = document.getElementById('pendingAttachments');
const voiceToggleBtn = document.getElementById('voiceToggleBtn');

// ---------- 截图 DOM 引用 ----------
const captureBtn = document.getElementById('captureBtn');
const captureModal = document.getElementById('captureModal');
const capturePreviewImg = document.getElementById('capturePreviewImg');
const captureToSendBtn = document.getElementById('captureToSendBtn');
const captureToDrawboardBtn = document.getElementById('captureToDrawboardBtn');
const captureCloseBtn = document.getElementById('captureCloseBtn');

// ---------- 画板 DOM 引用 ----------
const openDrawboardBtn = document.getElementById('openDrawboardBtn');
const drawboardOverlay = document.getElementById('drawboardOverlay');
const importBgBtn = document.getElementById('importBgBtn');
const bgFileInput = document.getElementById('bgFileInput');
const clearDrawBtn = document.getElementById('clearDrawBtn');
const closeDrawboardBtn = document.getElementById('closeDrawboardBtn');
const undoDrawBtn = document.getElementById('undoDrawBtn');
const drawboardCanvasWrap = document.getElementById('drawboardCanvasWrap');
const drawboardBg = document.getElementById('drawboardBg');
const drawboardLayer = document.getElementById('drawboardLayer');
const drawboardPreview = document.getElementById('drawboardPreview');
const drawboardInput = document.getElementById('drawboardInput');
const drawboardSendBtn = document.getElementById('drawboardSendBtn');

// ---------- 滚动控制 DOM 引用 ----------
const scrollTopBtn = document.getElementById('scrollTopBtn');
const scrollBottomBtn = document.getElementById('scrollBottomBtn');
const jumpUserBtn = document.getElementById('jumpUserBtn');
const userJumpPanel = document.getElementById('userJumpPanel');
const userJumpList = document.getElementById('userJumpList');

// ---------- 状态变量 ----------
let ws = null;
let reconnectAttempts = 0;
let reconnectTimer = null;
let manualClose = false;
let backendConnected = false;
let isDarkMode = false;
let currentTab = 'all';
let messages = [];            // 持久化的消息对象列表
let dragCounter = 0;
let searchQuery = '';
let searchMatches = [];
let currentMatchIndex = -1;
let saveTimer = null;
let mermaidInitialized = false;
let pendingFiles = [];        // 待发送附件（悬浮气泡）
let isSending = false;        // 是否正在发送
let autoPlayVoice = true;     // 收到语音消息时是否自动播放
let captureFile = null;       // 当前截图的 File 对象
let capturePreviewUrl = null; // 当前截图预览 blob URL

// ---------- 音频播放队列（TTS） ----------
class AudioQueueManager {
    constructor() {
        this.audioContext = null;
        this.currentSource = null;
        this.queue = [];
        this.playing = false;
    }

    enqueue(audioBase64) {
        if (!audioBase64) return;
        this.queue.push(audioBase64);
        if (!this.playing) this.playNext();
    }

    playNext() {
        if (this.queue.length === 0) {
            this.playing = false;
            this.currentSource = null;
            return;
        }
        this.playing = true;
        const base64 = this.queue.shift();
        try {
            const arrayBuffer = this.base64ToArrayBuffer(base64);
            this.decodeAndPlay(arrayBuffer);
        } catch (err) {
            console.warn('音频处理失败', err);
            this.playNext();
        }
    }

    decodeAndPlay(arrayBuffer) {
        if (!this.audioContext) {
            this.audioContext = new (window.AudioContext || window.webkitAudioContext)();
        }
        if (this.audioContext.state === 'suspended') {
            this.audioContext.resume();
        }
        this.audioContext.decodeAudioData(
            arrayBuffer,
            (buffer) => this.playAudioBuffer(buffer),
            (err) => {
                console.warn('音频解码失败', err);
                this.playNext();
            }
        );
    }

    playAudioBuffer(audioBuffer) {
        const source = this.audioContext.createBufferSource();
        source.buffer = audioBuffer;
        source.connect(this.audioContext.destination);
        this.currentSource = source;
        source.onended = () => {
            this.currentSource = null;
            this.playNext();
        };
        source.start();
    }

    stop() {
        if (this.currentSource) {
            try {
                this.currentSource.onended = null;
                this.currentSource.stop();
            } catch (e) { /* 已停止 */ }
            this.currentSource = null;
        }
        this.queue = [];
        this.playing = false;
    }

    base64ToArrayBuffer(base64) {
        const binary = atob(base64);
        const bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) {
            bytes[i] = binary.charCodeAt(i);
        }
        return bytes.buffer;
    }
}
const AudioQueue = new AudioQueueManager();

// ---------- 音乐播放器 iframe 桥接（BroadcastChannel） ----------
const musicChannel = new BroadcastChannel('lunar-astral-music');
let musicIframe = null;
let musicReady = false;
const musicPendingQueue = [];

function initMusicRenderer() {
    if (document.getElementById('music-renderer-frame')) return;
    musicIframe = document.createElement('iframe');
    musicIframe.id = 'music-renderer-frame';
    musicIframe.src = '/file/read/package/music_libs/music_renderer.html';
    musicIframe.allow = 'autoplay';
    musicIframe.style.cssText = 'position:fixed;inset:0;width:100%;height:100%;border:none;z-index:1000;pointer-events:none;background:transparent;display:none;';
    document.body.appendChild(musicIframe);

    musicChannel.onmessage = (event) => {
        const msg = event.data;
        if (!msg || !msg.type) return;
        switch (msg.type) {
            case 'ready':
                musicReady = true;
                musicChannel.postMessage({ type: 'theme', darkMode: document.body.classList.contains('dark-mode') });
                while (musicPendingQueue.length) musicChannel.postMessage(musicPendingQueue.shift());
                break;
            case 'closed':
                hideMusicIframe();
                break;
            case 'state':
                if (msg.playing || msg.paused) showMusicIframe();
                break;
        }
    };
}

function postMusicMessage(msg) {
    if (!musicReady) {
        musicPendingQueue.push(msg);
        return;
    }
    musicChannel.postMessage(msg);
}

function showMusicIframe() {
    if (musicIframe) {
        musicIframe.style.display = 'block';
        musicIframe.style.pointerEvents = 'auto';
    }
}

function hideMusicIframe() {
    if (musicIframe) {
        musicIframe.style.display = 'none';
        musicIframe.style.pointerEvents = 'none';
    }
}

function renderMusicScore(abcNotation) {
    if (!abcNotation) return;
    showMusicIframe();
    postMusicMessage({ type: 'render', abcNotation });
}

function playRenderedAudio(audioUrl, fileName) {
    if (!audioUrl) return;
    showMusicIframe();
    postMusicMessage({ type: 'play_audio', audioUrl, fileName });
}

// ---------- 通用工具 ----------
function generateId() {
    return `msg-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

function getTimeString() {
    return new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str == null ? '' : String(str);
    return div.innerHTML;
}

function showToast(msg, type = 'info') {
    const toast = document.createElement('div');
    toast.className = 'toast';
    const icons = { info: 'fa-circle-info', success: 'fa-check-circle', error: 'fa-exclamation-triangle' };
    toast.innerHTML = `<i class="fas ${icons[type] || icons.info}" style="margin-right:8px;"></i>${escapeHtml(msg)}`;
    toastContainer.appendChild(toast);
    setTimeout(() => toast.remove(), 2800);
}

function updateEmptyState() {
    emptyState.classList.toggle('hidden', messages.length > 0);
}

function scrollToBottom(smooth = true) {
    messageArea.scrollTo({ top: messageArea.scrollHeight, behavior: smooth ? 'smooth' : 'auto' });
}

function scrollToTop(smooth = true) {
    messageArea.scrollTo({ top: 0, behavior: smooth ? 'smooth' : 'auto' });
}

// ---------- 用户发言跳转 ----------
function summarizeUserMessage(msg) {
    if (msg.content && msg.content.trim()) {
        const text = msg.content.replace(/\s+/g, ' ').trim();
        return text.length > 30 ? text.slice(0, 30) + '…' : text;
    }
    if (msg.attachments && msg.attachments.length) {
        const imgCount = msg.attachments.filter(a => a.type === 'image').length;
        if (imgCount) return `[图片 ×${imgCount}]`;
        return '[附件]';
    }
    if (msg.imageSrc) return '[图片]';
    return '[无文本]';
}

function renderUserJumpList() {
    userJumpList.innerHTML = '';
    const userMsgs = messages.filter(m => m.role === 'user');
    if (!userMsgs.length) {
        userJumpList.innerHTML = '<div class="user-jump-empty">暂无用户发言</div>';
        return;
    }
    userMsgs.forEach(m => {
        const item = document.createElement('button');
        item.className = 'user-jump-item';
        const time = m.timestamp
            ? new Date(m.timestamp).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
            : '';
        item.innerHTML = `<span class="user-jump-time">${escapeHtml(time)}</span><span class="user-jump-summary">${escapeHtml(summarizeUserMessage(m))}</span>`;
        item.addEventListener('click', () => {
            jumpToMessage(m.id);
            closeUserJumpPanel();
        });
        userJumpList.appendChild(item);
    });
}

function jumpToMessage(id) {
    const el = messageArea.querySelector(`.message[data-id="${id}"]`);
    if (!el) return;
    el.style.display = ''; // 即使被标签/搜索过滤隐藏，也临时显示目标消息
    el.scrollIntoView({ block: 'start', behavior: 'smooth' });
    flashMessageBorder(el);
}

// 边框闪烁（直接操作 box-shadow，不触发消息入场动画，避免气泡跳动）
function flashMessageBorder(el) {
    if (el._flashTimer) {
        clearInterval(el._flashTimer);
        el._flashTimer = null;
    }
    let on = true;
    let count = 0;
    el.style.boxShadow = '0 0 0 3px rgba(157, 107, 255, 0.85)';
    el._flashTimer = setInterval(() => {
        on = !on;
        count++;
        el.style.boxShadow = on ? '0 0 0 3px rgba(157, 107, 255, 0.85)' : '0 0 0 0 rgba(157, 107, 255, 0)';
        if (count >= 6) {
            clearInterval(el._flashTimer);
            el._flashTimer = null;
            el.style.boxShadow = '';
        }
    }, 220);
}

function openUserJumpPanel() {
    renderUserJumpList();
    userJumpPanel.hidden = false;
    jumpUserBtn.classList.add('active');
}

function closeUserJumpPanel() {
    userJumpPanel.hidden = true;
    jumpUserBtn.classList.remove('active');
}

function toggleUserJumpPanel() {
    if (userJumpPanel.hidden) openUserJumpPanel();
    else closeUserJumpPanel();
}

function setupScrollControls() {
    scrollTopBtn.addEventListener('click', () => scrollToTop(true));
    scrollBottomBtn.addEventListener('click', () => scrollToBottom(true));
    jumpUserBtn.addEventListener('click', toggleUserJumpPanel);
    document.addEventListener('click', (e) => {
        if (!userJumpPanel.hidden && !userJumpPanel.contains(e.target) && e.target !== jumpUserBtn && !jumpUserBtn.contains(e.target)) {
            closeUserJumpPanel();
        }
    });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && !userJumpPanel.hidden) closeUserJumpPanel();
    });
}

async function copyToClipboard(text) {
    try {
        await navigator.clipboard.writeText(text || '');
        return true;
    } catch {
        return false;
    }
}

function encodeFilePath(path) {
    // 路径为 ASCII，直接 base64 编码
    return btoa(unescape(encodeURIComponent(path)));
}

// ---------- 文件上传辅助 ----------
function getFileCategory(file) {
    if (file.type.startsWith('image/')) return 'image';
    if (file.type.startsWith('video/')) return 'video';
    if (file.type.startsWith('audio/')) return 'audio';
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    const textExts = ['txt', 'md', 'json', 'xml', 'yaml', 'yml', 'toml', 'csv', 'html', 'htm', 'css', 'js', 'ts', 'jsx', 'tsx', 'go', 'rs', 'java', 'c', 'cpp', 'h', 'py', 'rb', 'sh', 'ps1', 'bat', 'log'];
    if (textExts.includes(ext)) return 'text';
    return 'other';
}

async function calculateFileHash(file) {
    return new Promise((resolve) => {
        const reader = new FileReader();
        reader.onload = async (e) => {
            try {
                const arrayBuffer = e.target.result;
                const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
                const hashArray = Array.from(new Uint8Array(hashBuffer));
                const fullHash = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
                resolve(fullHash.substring(0, 16));
            } catch {
                resolve(encodeFilePath(file.name).slice(-16));
            }
        };
        reader.onerror = () => resolve(encodeFilePath(file.name).slice(-16));
        reader.readAsArrayBuffer(file);
    });
}

// 保存文件到服务器，返回可访问的 fileUrl
async function saveFile(file) {
    const category = getFileCategory(file);
    const prefix = (category === 'image' || category === 'video' || category === 'audio') ? 'images/' : 'documents/';
    const fileHash = await calculateFileHash(file);
    const ext = file.name.slice(file.name.lastIndexOf('.')).toLowerCase() || '.bin';
    const newFileName = `${fileHash}${ext}`;
    const res = await fetch('/file/write', {
        method: 'POST',
        headers: {
            'X-File-Name': encodeFilePath(prefix + newFileName),
            'X-Overwrite': 'true'
        },
        body: file
    });
    if (!res.ok) throw new Error('文件上传失败');
    const result = await res.json();
    return `${window.location.origin}/file/read/${result.filename}`;
}

function fileToRawBase64(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
            const result = reader.result;
            if (typeof result !== 'string' || !result.startsWith('data:')) {
                reject(new Error('读取文件失败'));
                return;
            }
            resolve(result.slice(result.indexOf(',') + 1));
        };
        reader.onerror = () => reject(reader.error || new Error('FileReader error'));
        reader.readAsDataURL(file);
    });
}

function getAudioFormat(file) {
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    if (ext === 'wav') return 'wav';
    if (ext === 'mp3') return 'mp3';
    return null;
}

function updateConnectionStatusUI(state) {
    if (state === 'connected') {
        sendButton.classList.add('connected');
    } else {
        sendButton.classList.remove('connected');
    }
}

// ---------- Markdown / 图表 / 数学渲染 ----------
async function ensureMarked() {
    if (window.marked) return true;
    for (let i = 0; i < 50; i++) {
        await new Promise(r => setTimeout(r, 100));
        if (window.marked) return true;
    }
    return false;
}

function initMermaid() {
    if (mermaidInitialized || !window.mermaid) return;
    window.mermaid.initialize({
        startOnLoad: false,
        theme: isDarkMode ? 'dark' : 'default',
        securityLevel: 'loose',
        fontFamily: 'inherit'
    });
    mermaidInitialized = true;
}

function processThinkTags(html) {
    return html.replace(/<think>([\s\S]*?)<\/think>/gi, (match, content) => {
        return `<div class="think-block"><div class="think-summary"><i class="fas fa-chevron-right toggle-icon"></i> 思考过程</div><div class="think-content">${content}</div></div>`;
    });
}

async function renderMarkdown(rawContent) {
    let html = processThinkTags(rawContent);
    if (window.marked) {
        html = await window.marked.parse(html);
    } else {
        html = '<p>' + escapeHtml(rawContent).replace(/\n/g, '<br>') + '</p>';
    }
    return html;
}

function highlightCode(container) {
    if (!window.hljs) return;
    container.querySelectorAll('pre code').forEach((block) => {
        if (block.parentElement && block.parentElement.classList.contains('hljs')) return;
        const langClass = Array.from(block.classList).find(c => c.startsWith('language-'));
        if (langClass && (langClass === 'language-echarts' || langClass === 'language-mermaid')) return;
        try {
            window.hljs.highlightElement(block);
        } catch (e) {
            console.warn('代码高亮失败', e);
        }
    });
}

function renderECharts(container) {
    if (!window.echarts) return;
    container.querySelectorAll('pre code.language-echarts').forEach((block) => {
        try {
            const clean = block.textContent.trim();
            const config = JSON.parse(clean);
            const chartDiv = document.createElement('div');
            chartDiv.className = 'echarts-container';
            const inner = document.createElement('div');
            inner.style.width = '100%';
            inner.style.height = '100%';
            chartDiv.appendChild(inner);
            const pre = block.parentElement;
            if (pre && pre.tagName === 'PRE') {
                pre.replaceWith(chartDiv);
            } else {
                block.replaceWith(chartDiv);
            }
            const chart = window.echarts.init(inner);
            chart.setOption(config);
            chartDiv._echartsInstance = chart;
            setTimeout(() => chart.resize(), 100);
            window.addEventListener('resize', () => chart.resize());
        } catch (e) {
            console.warn('ECharts 渲染失败', e);
        }
    });
}

async function renderMermaid(container) {
    if (!window.mermaid || !mermaidInitialized) return;
    const blocks = Array.from(container.querySelectorAll('pre code.language-mermaid'));
    for (const block of blocks) {
        const code = (block.textContent || '').trim();
        if (!code) continue;
        try {
            await window.mermaid.parse(code);
            const id = `mermaid-${Date.now()}-${Math.floor(Math.random() * 10000)}`;
            const { svg } = await window.mermaid.render(id, code);
            const wrapper = document.createElement('div');
            wrapper.className = 'mermaid-container';
            wrapper.innerHTML = svg;
            const pre = block.parentElement;
            if (pre && pre.tagName === 'PRE') {
                pre.replaceWith(wrapper);
            } else {
                block.replaceWith(wrapper);
            }
        } catch (e) {
            console.error('Mermaid 渲染失败', e);
            const errDiv = document.createElement('div');
            errDiv.className = 'mermaid-error';
            errDiv.textContent = `Mermaid 渲染失败：${e.message || String(e)}`;
            const pre = block.parentElement;
            if (pre && pre.tagName === 'PRE') {
                pre.replaceWith(errDiv);
            } else {
                block.replaceWith(errDiv);
            }
        }
    }
}

function renderMath(container) {
    if (typeof window.renderMathInElement === 'function') {
        try {
            window.renderMathInElement(container, {
                delimiters: [
                    { left: '$$', right: '$$', display: true },
                    { left: '$', right: '$', display: false },
                    { left: '\\(', right: '\\)', display: false },
                    { left: '\\[', right: '\\]', display: true }
                ],
                throwOnError: false
            });
        } catch (e) {
            console.warn('KaTeX 渲染失败', e);
        }
    }
}

async function fillMarkdownContent(el, content) {
    const contentDiv = el.querySelector('.markdown-content');
    if (!contentDiv || !content) return;
    contentDiv.innerHTML = '<i class="fas fa-spinner fa-pulse"></i> 加载中...';
    const html = await renderMarkdown(content);
    contentDiv.innerHTML = html;
    contentDiv.querySelectorAll('table').forEach(t => t.classList.add('markdown-table'));
    highlightCode(contentDiv);
    renderECharts(contentDiv);
    await renderMermaid(contentDiv);
    renderMath(contentDiv);
}

// ---------- 消息渲染 ----------
const CATEGORY_META = {
    text: { label: '文本', icon: 'fa-align-left', cls: 'badge-text' },
    image: { label: '图片', icon: 'fa-image', cls: 'badge-image' },
    voice: { label: '语音', icon: 'fa-volume-high', cls: 'badge-voice' },
    music: { label: '乐谱', icon: 'fa-music', cls: 'badge-music' },
    action: { label: '行动', icon: 'fa-person-running', cls: 'badge-action' }
};

function buildCategoryBadges(categories) {
    return (categories || []).map(cat => {
        const meta = CATEGORY_META[cat];
        if (!meta) return '';
        return `<span class="message-category-badge ${meta.cls}"><i class="fas ${meta.icon}"></i>${meta.label}</span>`;
    }).join('');
}

function buildImageBlock(msg) {
    const grid = document.createElement('div');
    grid.className = 'image-grid';
    const container = document.createElement('div');
    container.className = 'labeled-image-container';
    container.style.setProperty('--image-label', `'${msg.imageLabel || '图片'}'`);
    const img = document.createElement('img');
    img.src = msg.imageSrc;
    img.alt = msg.imageLabel || '图片';
    img.loading = 'lazy';
    img.addEventListener('click', () => {
        if (typeof window.previewImage === 'function') window.previewImage(msg.imageSrc, msg.imageLabel || '图片');
    });
    container.appendChild(img);
    grid.appendChild(container);
    return grid;
}

function buildVideoBlock(msg) {
    const container = document.createElement('div');
    container.className = 'video-container';
    const video = document.createElement('video');
    video.src = msg.videoSrc;
    video.controls = true;
    video.playsInline = true;
    container.appendChild(video);
    return container;
}

function buildAudioFileBlock(msg) {
    const audio = document.createElement('audio');
    audio.className = 'message-audio-player';
    audio.controls = true;
    audio.src = msg.audioSrc;
    return audio;
}

function buildAttachmentBlock(att) {
    if (!att) return null;
    if (att.type === 'image') {
        const grid = document.createElement('div');
        grid.className = 'image-grid';
        const container = document.createElement('div');
        container.className = 'labeled-image-container';
        container.style.setProperty('--image-label', `'${att.label || '图片'}'`);
        const img = document.createElement('img');
        img.src = att.src;
        img.alt = att.label || '图片';
        img.loading = 'lazy';
        img.addEventListener('click', () => {
            if (typeof window.previewImage === 'function') window.previewImage(att.src, att.label || '图片');
        });
        container.appendChild(img);
        grid.appendChild(container);
        return grid;
    }
    if (att.type === 'video') {
        const container = document.createElement('div');
        container.className = 'video-container';
        const video = document.createElement('video');
        video.src = att.src;
        video.controls = true;
        video.playsInline = true;
        container.appendChild(video);
        return container;
    }
    if (att.type === 'audio') {
        const audio = document.createElement('audio');
        audio.className = 'message-audio-player';
        audio.controls = true;
        audio.src = att.src;
        return audio;
    }
    return null;
}

function buildMusicCard(msg) {
    const card = document.createElement('div');
    card.className = 'music-card';
    const header = document.createElement('div');
    header.className = 'music-card-header';
    const title = document.createElement('div');
    title.className = 'music-card-title';
    title.innerHTML = '<i class="fas fa-music"></i> 乐谱';
    const playBtn = document.createElement('button');
    playBtn.className = 'music-play-btn';
    playBtn.innerHTML = '<i class="fas fa-play"></i> 播放';
    playBtn.addEventListener('click', () => renderMusicScore(msg.abcNotation));
    header.appendChild(title);
    header.appendChild(playBtn);
    const preview = document.createElement('div');
    preview.className = 'music-abc-preview';
    preview.textContent = msg.abcNotation || '';
    card.appendChild(header);
    card.appendChild(preview);
    return card;
}

function buildAudioReplay(msg) {
    const btn = document.createElement('button');
    btn.className = 'audio-replay-btn';
    btn.title = '重播语音';
    btn.innerHTML = '<i class="fas fa-volume-up"></i> 重播语音';
    btn.addEventListener('click', () => {
        AudioQueue.enqueue(msg.audio);
        btn.classList.add('replaying');
        setTimeout(() => btn.classList.remove('replaying'), 600);
    });
    return btn;
}

function buildActionsPanel(msg) {
    const panel = document.createElement('div');
    panel.className = 'message-actions-panel';
    const copyBtn = document.createElement('button');
    copyBtn.className = 'chat-action-button copy_message_button';
    copyBtn.title = '复制';
    copyBtn.innerHTML = '<i class="fas fa-copy"></i>';
    copyBtn.addEventListener('click', () => copyMessage(msg));
    const delBtn = document.createElement('button');
    delBtn.className = 'chat-action-button delete_message_button';
    delBtn.title = '删除';
    delBtn.innerHTML = '<i class="fas fa-trash"></i>';
    delBtn.addEventListener('click', () => deleteMessage(msg.id));
    panel.appendChild(copyBtn);
    panel.appendChild(delBtn);
    return panel;
}

function computeSearchText(msg) {
    const parts = [];
    if (msg.content) parts.push(msg.content);
    if (msg.imageLabel) parts.push(msg.imageLabel);
    if (msg.abcNotation) parts.push(msg.abcNotation);
    if (msg.attachments && msg.attachments.length) {
        msg.attachments.forEach(att => { if (att.label) parts.push(att.label); });
    }
    return parts.join(' ').toLowerCase();
}

function renderMessageElement(msg) {
    const el = document.createElement('div');
    el.className = `message ${msg.role === 'user' ? 'user-message' : 'assistant-message'}`;
    el.dataset.id = msg.id;
    el.dataset.categories = (msg.categories || ['text']).join(',');
    el.dataset.searchText = computeSearchText(msg);
    if (msg.categories && msg.categories.includes('action')) el.classList.add('action-message');

    const displayName = msg.role === 'user' ? USER_NAME : ASSISTANT_NAME;
    const header = document.createElement('div');
    header.className = 'message-header';
    header.innerHTML = `
        <span class="header-name">${escapeHtml(displayName)}</span>
        ${buildCategoryBadges(msg.categories)}
        <span class="header-time">${msg.timestamp ? new Date(msg.timestamp).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : getTimeString()}</span>
    `;
    el.appendChild(header);

    // 图片块
    if (msg.imageSrc) el.appendChild(buildImageBlock(msg));

    // 视频块
    if (msg.videoSrc) el.appendChild(buildVideoBlock(msg));

    // 音频文件块
    if (msg.audioSrc) el.appendChild(buildAudioFileBlock(msg));

    // 文本内容容器
    const contentDiv = document.createElement('div');
    contentDiv.className = 'markdown-content';
    el.appendChild(contentDiv);

    // 多附件（用户拖入的图片/视频/音频）
    if (msg.attachments && msg.attachments.length) {
        msg.attachments.forEach(att => {
            const block = buildAttachmentBlock(att);
            if (block) el.appendChild(block);
        });
    }

    // 乐谱卡片
    if (msg.abcNotation) el.appendChild(buildMusicCard(msg));

    // 语音重播按钮
    if (msg.audio) el.appendChild(buildAudioReplay(msg));

    // 操作按钮
    el.appendChild(buildActionsPanel(msg));

    messageArea.appendChild(el);

    if (msg.content) return fillMarkdownContent(el, msg.content);
    return Promise.resolve(el);
}

// ---------- 消息增删与持久化 ----------
async function copyMessage(msg) {
    let text = msg.content || '';
    if (!text && msg.imageSrc) text = msg.imageSrc;
    if (!text && msg.abcNotation) text = msg.abcNotation;
    const ok = await copyToClipboard(text);
    showToast(ok ? '已复制' : '复制失败', ok ? 'success' : 'error');
}

function deleteMessage(id) {
    const el = messageArea.querySelector(`.message[data-id="${id}"]`);
    if (el) el.remove();
    messages = messages.filter(m => m.id !== id);
    updateEmptyState();
    schedulePersist();
    showToast('消息已删除', 'info');
}

function addMessage(msg) {
    messages.push(msg);
    if (messages.length > MAX_PERSISTED_MESSAGES) {
        const removed = messages.shift();
        const oldEl = messageArea.querySelector(`.message[data-id="${removed.id}"]`);
        if (oldEl) oldEl.remove();
    }
    renderMessageElement(msg);
    updateEmptyState();
    scrollToBottom(true);
    applyFilters();
    schedulePersist();
}

async function persistMessages() {
    const data = JSON.stringify(messages, null, 2);
    try {
        await fetch('/file/write', {
            method: 'POST',
            headers: {
                'X-File-Name': encodeFilePath(MESSAGES_FILE_PATH),
                'X-Overwrite': 'true'
            },
            body: data
        });
    } catch (e) {
        console.warn('消息持久化失败', e);
    }
}

function schedulePersist() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(persistMessages, 800);
}

async function loadPersistedMessages() {
    try {
        const res = await fetch('/file/read/lunar_messages.json');
        if (!res.ok) return;
        const list = await res.json();
        if (!Array.isArray(list)) return;
        messages = list;
        const renders = list.map(msg => renderMessageElement(msg));
        updateEmptyState();
        // 等待所有消息的 markdown/mermaid 异步渲染完成后再滚动到底部
        await Promise.all(renders);
        scrollToBottom(false);
        // 懒加载图片进入视口后异步加载会改变高度，延迟补滚确保到达真实底部
        setTimeout(() => scrollToBottom(false), 250);
        applyFilters();
    } catch (e) {
        // 无持久化文件或读取失败，从空状态开始
    }
}

// ---------- WebSocket 消息处理 ----------
function handleWebSocketMessage(msg) {
    const type = msg.type || '';
    const data = msg.data || {};

    if (type === 'context') {
        const subType = data.type || 'response';
        const content = data.content || '';
        const audio = data.audio || '';

        if (subType === 'music') {
            addMessage({ id: generateId(), role: 'assistant', categories: ['music'], content: '', abcNotation: content, timestamp: Date.now() });
            return;
        }
        if (subType === 'music_audio') {
            try {
                const audioData = JSON.parse(content || '{}');
                if (audioData.type === 'audio_ready' && audioData.audio_url) {
                    playRenderedAudio(audioData.audio_url, audioData.file_name);
                }
            } catch (e) {
                console.warn('乐谱音频数据解析失败', e);
            }
            return;
        }
        if (subType === 'action') {
            addMessage({ id: generateId(), role: 'assistant', categories: ['action'], content, actionType: subType, timestamp: Date.now() });
            return;
        }

        // 文本 / 思考 / 代码等上下文消息（可能携带 TTS 音频）
        const categories = ['text'];
        if (audio) categories.push('voice');
        addMessage({ id: generateId(), role: 'assistant', categories, content, audio: audio || '', timestamp: Date.now() });
        if (audio && autoPlayVoice) AudioQueue.enqueue(audio);
        return;
    }

    if (type === 'image') {
        const images = data.images || [];
        const isSticker = !!data.sticker;
        images.forEach(img => {
            const src = (img.startsWith('data:') || img.startsWith('http')) ? img : ('data:image/jpeg;base64,' + img);
            addMessage({ id: generateId(), role: 'assistant', categories: ['image'], content: '', imageSrc: src, imageLabel: isSticker ? '表情包' : '图片', timestamp: Date.now() });
        });
        return;
    }

    // 未知格式：以 JSON 文本兜底展示
    addMessage({ id: generateId(), role: 'assistant', categories: ['text'], content: '```json\n' + JSON.stringify(msg, null, 2) + '\n```', timestamp: Date.now() });
}

// ---------- WebSocket 连接管理 ----------
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
        backendConnected = true;
        updateConnectionStatusUI('connected');
    };

    ws.onmessage = (event) => {
        try {
            const parsed = JSON.parse(event.data);
            handleWebSocketMessage(parsed);
        } catch {
            addMessage({ id: generateId(), role: 'assistant', categories: ['text'], content: event.data, timestamp: Date.now() });
        }
    };

    ws.onerror = () => {
        updateConnectionStatusUI('disconnected');
    };

    ws.onclose = () => {
        backendConnected = false;
        updateConnectionStatusUI('disconnected');
        if (!manualClose) scheduleReconnect();
    };
}

function scheduleReconnect() {
    if (manualClose) return;
    if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
        backendConnected = false;
        updateConnectionStatusUI('failed');
        showToast('后端连接失败，已进入本地模式', 'error');
        return;
    }
    const delay = RECONNECT_BASE_DELAY * Math.pow(1.5, reconnectAttempts);
    reconnectTimer = setTimeout(() => {
        reconnectAttempts++;
        connectWebSocket();
    }, delay);
}

// ---------- 标签页过滤 ----------
function setupTabEvents() {
    tabBar.addEventListener('click', (e) => {
        const btn = e.target.closest('.tab');
        if (!btn) return;
        tabBar.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
        btn.classList.add('active');
        currentTab = btn.dataset.tab;
        applyFilters();
    });
}

// ---------- 搜索 ----------
function clearHighlights() {
    document.querySelectorAll('.search-highlight').forEach(mark => {
        const parent = mark.parentNode;
        if (parent) {
            parent.replaceChild(document.createTextNode(mark.textContent), mark);
            parent.normalize();
        }
    });
}

function highlightTextInNode(root, query) {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
        acceptNode(node) {
            if (!node.nodeValue || !node.nodeValue.toLowerCase().includes(query)) return NodeFilter.FILTER_REJECT;
            const parent = node.parentElement;
            if (!parent || parent.tagName === 'SCRIPT' || parent.tagName === 'STYLE' || parent.classList.contains('search-highlight')) return NodeFilter.FILTER_REJECT;
            return NodeFilter.FILTER_ACCEPT;
        }
    });
    const textNodes = [];
    while (walker.nextNode()) textNodes.push(walker.currentNode);

    textNodes.forEach(node => {
        const text = node.nodeValue;
        const lower = text.toLowerCase();
        const qlen = query.length;
        const frag = document.createDocumentFragment();
        let i = 0;
        let idx;
        while ((idx = lower.indexOf(query, i)) !== -1) {
            if (idx > i) frag.appendChild(document.createTextNode(text.slice(i, idx)));
            const mark = document.createElement('span');
            mark.className = 'search-highlight';
            mark.textContent = text.slice(idx, idx + qlen);
            frag.appendChild(mark);
            i = idx + qlen;
        }
        if (i < text.length) frag.appendChild(document.createTextNode(text.slice(i)));
        node.parentNode.replaceChild(frag, node);
    });
}

function applyFilters() {
    clearHighlights();
    searchMatches = [];
    currentMatchIndex = -1;

    const q = searchQuery;
    messageArea.querySelectorAll('.message').forEach(el => {
        const cats = (el.dataset.categories || '').split(',');
        const tabOk = currentTab === 'all' || cats.includes(currentTab);
        const searchOk = !q || (el.dataset.searchText || '').includes(q);
        el.style.display = (tabOk && searchOk) ? '' : 'none';
    });

    if (q) {
        messageArea.querySelectorAll('.message').forEach(el => {
            if (el.style.display === 'none') return;
            const targets = el.querySelectorAll('.markdown-content, .music-abc-preview, .labeled-image-container');
            targets.forEach(t => highlightTextInNode(t, q));
        });
        searchMatches = Array.from(document.querySelectorAll('.search-highlight'));
    }

    updateSearchUI();
}

function updateSearchUI() {
    const q = searchQuery;
    searchClear.hidden = !q;
    searchPrev.hidden = !q || searchMatches.length === 0;
    searchNext.hidden = !q || searchMatches.length === 0;
    if (q) {
        searchCount.hidden = false;
        searchCount.textContent = searchMatches.length ? `${currentMatchIndex + 1}/${searchMatches.length}` : '0/0';
    } else {
        searchCount.hidden = true;
    }
}

function goToMatch(delta) {
    if (!searchMatches.length) return;
    currentMatchIndex = (currentMatchIndex + delta + searchMatches.length) % searchMatches.length;
    searchMatches.forEach(m => m.classList.remove('current'));
    const current = searchMatches[currentMatchIndex];
    current.classList.add('current');
    current.scrollIntoView({ block: 'center', behavior: 'smooth' });
    updateSearchUI();
}

function setupSearchEvents() {
    searchInput.addEventListener('input', () => {
        searchQuery = searchInput.value.trim().toLowerCase();
        currentMatchIndex = -1;
        applyFilters();
    });
    searchClear.addEventListener('click', () => {
        searchInput.value = '';
        searchQuery = '';
        applyFilters();
        searchInput.focus();
    });
    searchPrev.addEventListener('click', () => goToMatch(-1));
    searchNext.addEventListener('click', () => goToMatch(1));
}

// ---------- 消息发送 ----------
async function sendMessages(payload) {
    const res = await fetch('/write/message', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ messages: payload })
    });
    if (!res.ok) throw new Error('发送失败');
    return res.json();
}

function setSendingState(sending) {
    isSending = sending;
    if (sendButton) {
        sendButton.disabled = sending;
        sendButton.innerHTML = sending ? '<i class="fas fa-spinner fa-pulse"></i>' : '<i class="fas fa-paper-plane"></i>';
    }
}

async function handleSend() {
    if (isSending) return;
    const text = messageInput.value.trim();
    const hasPending = pendingFiles.length > 0;
    if (!text && !hasPending) return;

    setSendingState(true);

    try {
        const contentBlocks = [];
        const attachments = [];
        const historyTextParts = [];
        const categories = new Set();

        if (text) {
            categories.add('text');
            historyTextParts.push(text);
        }

        for (const pf of pendingFiles) {
            const category = pf.category;
            if (category === 'image' || category === 'video') {
                try {
                    const fileUrl = await saveFile(pf.file);
                    contentBlocks.push({ type: 'image_url', image_url: { url: fileUrl } });
                    attachments.push({ type: category, src: fileUrl.replace(window.location.origin, ''), label: pf.name });
                    categories.add('image');
                } catch (err) {
                    showToast(`无法上传 ${pf.name}`, 'error');
                }
            } else if (category === 'audio') {
                // 音频：wav/mp3 以 input_audio 形式推送，其余仅本地展示
                try {
                    const base64Data = await fileToRawBase64(pf.file);
                    const format = getAudioFormat(pf.file);
                    if (format) {
                        contentBlocks.push({ type: 'input_audio', input_audio: { data: base64Data, format } });
                    } else {
                        showToast(`音频 ${pf.name} 仅支持 wav/mp3，已跳过发送`, 'error');
                    }
                } catch (err) {
                    showToast(`无法读取音频 ${pf.name}`, 'error');
                }
                // 历史记录使用独立 blob URL，避免被清理撤销
                attachments.push({ type: 'audio', src: URL.createObjectURL(pf.file), label: pf.name });
                categories.add('voice');
            } else if (category === 'text') {
                try {
                    const fileUrl = await saveFile(pf.file);
                    const rawText = await readFileAsText(pf.file);
                    const preview = rawText.slice(0, 50000);
                    const block = `【文件 ${pf.name}】\n内容：\n\`\`\`\n${preview}\n\`\`\`\n访问链接：${fileUrl}`;
                    contentBlocks.push({ type: 'text', text: block });
                    historyTextParts.push(block);
                } catch (err) {
                    showToast(`无法读取文件 ${pf.name}`, 'error');
                }
                categories.add('text');
            } else {
                try {
                    const fileUrl = await saveFile(pf.file);
                    const block = `【文件 ${pf.name}】访问链接：${fileUrl}`;
                    contentBlocks.push({ type: 'text', text: block });
                    historyTextParts.push(block);
                } catch (err) {
                    showToast(`无法上传文件 ${pf.name}`, 'error');
                }
                categories.add('text');
            }
        }

        // 组装并显示到历史记录
        const userMsg = {
            id: generateId(),
            role: 'user',
            categories: categories.size ? Array.from(categories) : ['text'],
            content: historyTextParts.join('\n\n'),
            attachments: attachments.length ? attachments : undefined,
            timestamp: Date.now()
        };
        addMessage(userMsg);

        // 推送到后端
        if (backendConnected) {
            if (contentBlocks.length) {
                const openAIContent = (contentBlocks.length === 1 && contentBlocks[0].type === 'text')
                    ? contentBlocks[0].text
                    : contentBlocks;
                await sendMessages([{ role: 'user', content: openAIContent }]);
            } else if (text) {
                await sendMessages([{ role: 'user', content: text }]);
            }
        } else {
            showToast('离线模式：内容仅本地渲染', 'info');
        }
    } catch (err) {
        showToast('发送失败：' + (err.message || err), 'error');
    } finally {
        // 无论成功或失败都清理输入与待发送附件（消息已进入历史记录）
        messageInput.value = '';
        autoResizeTextarea();
        clearPendingFiles();
        messageInput.focus();
        setSendingState(false);
    }
}

function autoResizeTextarea() {
    messageInput.style.height = 'auto';
    messageInput.style.height = Math.min(messageInput.scrollHeight, 200) + 'px';
}

function setupInputEvents() {
    messageInput.addEventListener('input', autoResizeTextarea);
    messageInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            handleSend();
        }
    });
    sendButton.addEventListener('click', handleSend);
    attachBtn.addEventListener('click', () => fileInput.click());
    fileInput.addEventListener('change', (e) => {
        const files = e.target.files;
        if (files && files.length) addPendingFiles(Array.from(files));
        fileInput.value = '';
    });
    clearBtn.addEventListener('click', () => {
        if (messages.length === 0) return;
        messageArea.querySelectorAll('.message').forEach(el => el.remove());
        messages = [];
        updateEmptyState();
        schedulePersist();
        showToast('已清空消息', 'info');
    });
}

// ---------- 文件拖放 / 待发送附件（悬浮气泡） ----------
async function addPendingFiles(files) {
    for (const file of files) {
        const category = getFileCategory(file);
        const entry = { file, category, name: file.name, previewUrl: null };
        if (category === 'image') {
            entry.previewUrl = await readFileAsDataUrl(file);
        } else if (category === 'video' || category === 'audio') {
            entry.previewUrl = URL.createObjectURL(file);
        }
        pendingFiles.push(entry);
    }
    renderPendingAttachments();
}

function renderPendingAttachments() {
    pendingAttachments.innerHTML = '';
    if (!pendingFiles.length) {
        pendingAttachments.hidden = true;
        return;
    }
    pendingAttachments.hidden = false;

    const icons = { audio: 'fa-music', text: 'fa-file-alt', other: 'fa-file' };

    pendingFiles.forEach((pf, index) => {
        const item = document.createElement('div');
        item.className = 'pending-attachment-item';

        const preview = document.createElement('div');
        preview.className = 'pending-attachment-preview';
        if (pf.category === 'image') {
            const img = document.createElement('img');
            img.src = pf.previewUrl;
            img.alt = pf.name;
            img.style.cssText = 'width:100%;height:100%;object-fit:cover;';
            preview.appendChild(img);
        } else if (pf.category === 'video') {
            const video = document.createElement('video');
            video.src = pf.previewUrl;
            video.muted = true;
            video.playsInline = true;
            video.style.cssText = 'width:100%;height:100%;object-fit:cover;';
            preview.appendChild(video);
        } else {
            preview.innerHTML = `<i class="fas ${icons[pf.category] || icons.other}"></i>`;
        }

        const name = document.createElement('div');
        name.className = 'pending-attachment-name';
        name.textContent = pf.name;
        name.title = pf.name;

        const removeBtn = document.createElement('button');
        removeBtn.className = 'pending-attachment-remove';
        removeBtn.title = '移除';
        removeBtn.innerHTML = '<i class="fas fa-times"></i>';
        removeBtn.addEventListener('click', () => removePendingFile(index));

        item.appendChild(preview);
        item.appendChild(name);
        item.appendChild(removeBtn);
        pendingAttachments.appendChild(item);
    });
}

function removePendingFile(index) {
    const removed = pendingFiles.splice(index, 1)[0];
    if (removed && removed.previewUrl && removed.previewUrl.startsWith('blob:')) {
        URL.revokeObjectURL(removed.previewUrl);
    }
    renderPendingAttachments();
}

function clearPendingFiles() {
    pendingFiles.forEach(pf => {
        if (pf.previewUrl && pf.previewUrl.startsWith('blob:')) URL.revokeObjectURL(pf.previewUrl);
    });
    pendingFiles = [];
    renderPendingAttachments();
}

function readFileAsDataUrl(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result);
        reader.onerror = reject;
        reader.readAsDataURL(file);
    });
}

function readFileAsText(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result);
        reader.onerror = reject;
        reader.readAsText(file);
    });
}

function setupDragEvents() {
    document.addEventListener('dragenter', (e) => {
        e.preventDefault();
        dragCounter++;
        if (dragCounter === 1 && e.dataTransfer && Array.from(e.dataTransfer.types).includes('Files')) {
            dragOverlay.classList.add('active');
        }
    });
    document.addEventListener('dragover', (e) => e.preventDefault());
    document.addEventListener('dragleave', (e) => {
        e.preventDefault();
        dragCounter--;
        if (dragCounter === 0) dragOverlay.classList.remove('active');
    });
    document.addEventListener('drop', (e) => {
        e.preventDefault();
        dragCounter = 0;
        dragOverlay.classList.remove('active');
        const files = e.dataTransfer && e.dataTransfer.files;
        if (files && files.length) addPendingFiles(Array.from(files));
    });
}

// ---------- 主题切换 ----------
function loadTheme() {
    const saved = localStorage.getItem('message_terminal_theme');
    if (saved === 'dark') {
        isDarkMode = true;
        document.body.classList.add('dark-mode');
        themeToggle.innerHTML = '<i class="fas fa-sun"></i>';
    }
}

function toggleTheme() {
    isDarkMode = !isDarkMode;
    document.body.classList.toggle('dark-mode', isDarkMode);
    themeToggle.innerHTML = isDarkMode ? '<i class="fas fa-sun"></i>' : '<i class="fas fa-moon"></i>';
    localStorage.setItem('message_terminal_theme', isDarkMode ? 'dark' : 'light');
    mermaidInitialized = false;
    initMermaid();
    if (musicReady) musicChannel.postMessage({ type: 'theme', darkMode: isDarkMode });
}

function setupThemeToggle() {
    themeToggle.addEventListener('click', toggleTheme);
}

// ---------- 语音自动播放开关 ----------
function loadVoiceAutoPlay() {
    autoPlayVoice = localStorage.getItem('message_terminal_autoplay') !== 'off';
    updateVoiceToggleUI();
}

function toggleVoiceAutoPlay() {
    autoPlayVoice = !autoPlayVoice;
    localStorage.setItem('message_terminal_autoplay', autoPlayVoice ? 'on' : 'off');
    updateVoiceToggleUI();
    if (!autoPlayVoice) AudioQueue.stop();
}

function updateVoiceToggleUI() {
    voiceToggleBtn.classList.toggle('active', autoPlayVoice);
    voiceToggleBtn.title = autoPlayVoice ? '自动播放语音：开' : '自动播放语音：关';
    voiceToggleBtn.innerHTML = autoPlayVoice ? '<i class="fas fa-volume-up"></i>' : '<i class="fas fa-volume-mute"></i>';
}

function setupVoiceToggle() {
    voiceToggleBtn.addEventListener('click', toggleVoiceAutoPlay);
}

// ---------- 截图功能 ----------
async function captureScreen() {
    showToast('正在截图…', 'info');
    try {
        const res = await fetch('/capture', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mode: 'fullscreen', format: 'png' })
        });
        if (!res.ok) {
            const errText = await res.text();
            throw new Error(errText || `截图失败 (${res.status})`);
        }
        const blob = await res.blob();
        const file = new File([blob], `screenshot-${Date.now()}.png`, { type: blob.type || 'image/png' });
        openCaptureModal(file);
    } catch (err) {
        showToast('截图失败：' + (err.message || err), 'error');
    }
}

function openCaptureModal(file) {
    captureFile = file;
    capturePreviewUrl = URL.createObjectURL(file);
    capturePreviewImg.src = capturePreviewUrl;
    captureModal.classList.add('active');
    captureModal.setAttribute('aria-hidden', 'false');
}

function closeCaptureModal() {
    captureModal.classList.remove('active');
    captureModal.setAttribute('aria-hidden', 'true');
    if (capturePreviewUrl) {
        URL.revokeObjectURL(capturePreviewUrl);
        capturePreviewUrl = null;
    }
    captureFile = null;
    capturePreviewImg.removeAttribute('src');
}

async function sendCaptureToPending() {
    const file = captureFile;
    if (!file) return;
    closeCaptureModal();
    await addPendingFiles([file]);
    showToast('截图已加入待发送列表', 'success');
}

async function sendCaptureToDrawboard() {
    const file = captureFile;
    if (!file) return;
    closeCaptureModal();
    openDrawboard();
    await importDrawboardBackground(file);
}

function setupCapture() {
    captureBtn.addEventListener('click', captureScreen);
    captureToSendBtn.addEventListener('click', sendCaptureToPending);
    captureToDrawboardBtn.addEventListener('click', sendCaptureToDrawboard);
    captureCloseBtn.addEventListener('click', closeCaptureModal);
    captureModal.addEventListener('click', (e) => {
        if (e.target === captureModal) closeCaptureModal();
    });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && captureModal.classList.contains('active')) {
            closeCaptureModal();
        }
    });
}

// ---------- 事件委托 ----------
function setupMessageAreaDelegation() {
    messageArea.addEventListener('click', (e) => {
        const summary = e.target.closest('.think-summary');
        if (summary) {
            summary.parentElement.classList.toggle('open');
        }
    });
}

// ============================================================
//  画板页 — 绘制图形 + 导入背景图 + 合并为图片发送给 AI
// ============================================================

const DRAWBOARD_DEFAULT_W = 800;
const DRAWBOARD_DEFAULT_H = 600;

const drawboard = {
    bgCtx: null,
    layerCtx: null,
    previewCtx: null,
    currentTool: 'draw',
    currentColor: '#e74c3c',
    currentSize: 8,
    isDrawing: false,
    hasImage: false,
    dirty: false,
    startX: 0,
    startY: 0,
    lastX: 0,
    lastY: 0,
    history: []
};

function initDrawboardCanvas() {
    drawboardBg.width = drawboardLayer.width = drawboardPreview.width = DRAWBOARD_DEFAULT_W;
    drawboardBg.height = drawboardLayer.height = drawboardPreview.height = DRAWBOARD_DEFAULT_H;
    drawboard.bgCtx = drawboardBg.getContext('2d', { willReadFrequently: true });
    drawboard.layerCtx = drawboardLayer.getContext('2d', { willReadFrequently: true });
    drawboard.previewCtx = drawboardPreview.getContext('2d', { willReadFrequently: true });
}

function resizeDrawboardCanvases(w, h) {
    drawboardBg.width = drawboardLayer.width = drawboardPreview.width = w;
    drawboardBg.height = drawboardLayer.height = drawboardPreview.height = h;
}

function openDrawboard() {
    drawboardOverlay.classList.add('active');
    drawboardOverlay.setAttribute('aria-hidden', 'false');
    setTimeout(() => drawboardInput.focus(), 50);
}

function closeDrawboard() {
    drawboardOverlay.classList.remove('active');
    drawboardOverlay.setAttribute('aria-hidden', 'true');
}

function getDrawboardPos(e) {
    const rect = drawboardLayer.getBoundingClientRect();
    return {
        x: (e.clientX - rect.left) * (drawboardLayer.width / (rect.width || 1)),
        y: (e.clientY - rect.top) * (drawboardLayer.height / (rect.height || 1))
    };
}

function updateDrawboardUndo() {
    undoDrawBtn.disabled = drawboard.history.length === 0;
}

function saveDrawboardSnapshot() {
    if (drawboardLayer.width > 0 && drawboardLayer.height > 0) {
        drawboard.history.push(drawboard.layerCtx.getImageData(0, 0, drawboardLayer.width, drawboardLayer.height));
        if (drawboard.history.length > 30) drawboard.history.shift();
    }
    updateDrawboardUndo();
}

function undoDrawboard() {
    if (!drawboard.history.length) return;
    drawboard.layerCtx.putImageData(drawboard.history.pop(), 0, 0);
    updateDrawboardUndo();
}

function setDrawboardTool(tool, btn) {
    drawboard.currentTool = tool;
    document.querySelectorAll('.drawboard-tool[data-tool]').forEach(b => b.classList.remove('active'));
    if (btn) btn.classList.add('active');
    drawboardLayer.style.cursor = tool === 'text' ? 'text' : 'crosshair';
}

function setDrawboardColor(color, el) {
    drawboard.currentColor = color;
    document.querySelectorAll('.drawboard-color').forEach(c => c.classList.remove('active'));
    if (el) el.classList.add('active');
}

function setDrawboardSize(size, el) {
    drawboard.currentSize = parseInt(size, 10);
    document.querySelectorAll('.drawboard-size').forEach(s => s.classList.remove('active'));
    if (el) el.classList.add('active');
}

async function importDrawboardBackground(file) {
    try {
        const dataUrl = await readFileAsDataUrl(file);
        const img = await new Promise((resolve, reject) => {
            const image = new Image();
            image.onload = () => resolve(image);
            image.onerror = () => reject(new Error('图片加载失败'));
            image.src = dataUrl;
        });
        const w = img.naturalWidth || img.width;
        const h = img.naturalHeight || img.height;
        resizeDrawboardCanvases(w, h);
        drawboard.bgCtx.clearRect(0, 0, w, h);
        drawboard.bgCtx.drawImage(img, 0, 0);
        drawboard.layerCtx.clearRect(0, 0, w, h);
        drawboard.previewCtx.clearRect(0, 0, w, h);
        drawboard.hasImage = true;
        drawboard.dirty = false;
        drawboard.history = [];
        drawboardCanvasWrap.classList.add('has-image');
        updateDrawboardUndo();
        showToast('背景图已导入', 'success');
    } catch (err) {
        showToast('背景图导入失败：' + (err.message || err), 'error');
    }
}

function clearDrawboard() {
    resizeDrawboardCanvases(DRAWBOARD_DEFAULT_W, DRAWBOARD_DEFAULT_H);
    drawboard.bgCtx.clearRect(0, 0, DRAWBOARD_DEFAULT_W, DRAWBOARD_DEFAULT_H);
    drawboard.layerCtx.clearRect(0, 0, DRAWBOARD_DEFAULT_W, DRAWBOARD_DEFAULT_H);
    drawboard.previewCtx.clearRect(0, 0, DRAWBOARD_DEFAULT_W, DRAWBOARD_DEFAULT_H);
    drawboard.hasImage = false;
    drawboard.dirty = false;
    drawboard.history = [];
    drawboardCanvasWrap.classList.remove('has-image');
    updateDrawboardUndo();
    showToast('画板已清空', 'info');
}

function drawboardStart(e) {
    if (e.button !== 0) return;
    drawboard.isDrawing = true;
    saveDrawboardSnapshot();
    const p = getDrawboardPos(e);
    drawboard.startX = drawboard.lastX = p.x;
    drawboard.startY = drawboard.lastY = p.y;
    if (drawboard.currentTool === 'draw') {
        drawboard.layerCtx.beginPath();
        drawboard.layerCtx.moveTo(p.x, p.y);
    }
}

function drawboardMove(e) {
    if (!drawboard.isDrawing) return;
    const p = getDrawboardPos(e);
    if (drawboard.currentTool === 'draw') {
        drawboard.layerCtx.lineTo(p.x, p.y);
        drawboard.layerCtx.strokeStyle = drawboard.currentColor;
        drawboard.layerCtx.lineWidth = drawboard.currentSize;
        drawboard.layerCtx.lineCap = drawboard.layerCtx.lineJoin = 'round';
        drawboard.layerCtx.stroke();
    } else if (drawboard.currentTool !== 'text') {
        drawboard.previewCtx.clearRect(0, 0, drawboardPreview.width, drawboardPreview.height);
        drawShapePreview(drawboard.previewCtx, p.x, p.y);
    }
    drawboard.lastX = p.x;
    drawboard.lastY = p.y;
}

function drawboardStop(e) {
    if (!drawboard.isDrawing) return;
    drawboard.isDrawing = false;
    const p = e ? getDrawboardPos(e) : { x: drawboard.lastX, y: drawboard.lastY };

    if (drawboard.currentTool === 'draw') {
        drawboard.dirty = true;
    } else if (drawboard.currentTool === 'text') {
        const text = prompt('输入文本:', '标注');
        if (text) {
            drawboard.layerCtx.font = `bold ${20 + drawboard.currentSize * 3}px sans-serif`;
            drawboard.layerCtx.fillStyle = drawboard.currentColor;
            drawboard.layerCtx.fillText(text, p.x, p.y);
            drawboard.dirty = true;
        }
    } else {
        drawboard.layerCtx.drawImage(drawboardPreview, 0, 0);
        drawboard.previewCtx.clearRect(0, 0, drawboardPreview.width, drawboardPreview.height);
        drawboard.dirty = true;
    }
}

function drawShapePreview(ctx, x, y) {
    const { startX, startY, currentColor, currentSize } = drawboard;
    ctx.strokeStyle = currentColor;
    ctx.lineWidth = currentSize;
    ctx.lineCap = 'round';
    switch (drawboard.currentTool) {
        case 'line':
            ctx.beginPath();
            ctx.moveTo(startX, startY);
            ctx.lineTo(x, y);
            ctx.stroke();
            break;
        case 'rect':
            ctx.strokeRect(startX, startY, x - startX, y - startY);
            break;
        case 'circle': {
            const rx = Math.abs(x - startX) / 2;
            const ry = Math.abs(y - startY) / 2;
            ctx.beginPath();
            ctx.ellipse(startX + (x - startX) / 2, startY + (y - startY) / 2, rx, ry, 0, 0, 2 * Math.PI);
            ctx.stroke();
            break;
        }
        case 'arrow':
            drawArrowShape(ctx, startX, startY, x, y);
            break;
    }
}

function drawArrowShape(ctx, fromX, fromY, toX, toY) {
    const dx = toX - fromX;
    const dy = toY - fromY;
    const angle = Math.atan2(dy, dx);
    const len = Math.sqrt(dx * dx + dy * dy);
    const head = Math.min(24, len * 0.25) * (drawboard.currentSize / 8);
    ctx.strokeStyle = ctx.fillStyle = drawboard.currentColor;
    ctx.lineWidth = drawboard.currentSize;
    ctx.lineCap = 'round';
    ctx.beginPath();
    ctx.moveTo(fromX, fromY);
    ctx.lineTo(toX - head * Math.cos(angle), toY - head * Math.sin(angle));
    ctx.stroke();
    ctx.beginPath();
    ctx.moveTo(toX, toY);
    ctx.lineTo(toX - head * Math.cos(angle - Math.PI / 6), toY - head * Math.sin(angle - Math.PI / 6));
    ctx.lineTo(toX - head * Math.cos(angle + Math.PI / 6), toY - head * Math.sin(angle + Math.PI / 6));
    ctx.closePath();
    ctx.fill();
}

function getDrawboardMergedBlob() {
    return new Promise((resolve) => {
        const temp = document.createElement('canvas');
        temp.width = drawboardLayer.width || DRAWBOARD_DEFAULT_W;
        temp.height = drawboardLayer.height || DRAWBOARD_DEFAULT_H;
        const ctx = temp.getContext('2d');
        ctx.fillStyle = '#ffffff';
        ctx.fillRect(0, 0, temp.width, temp.height);
        if (drawboard.hasImage) ctx.drawImage(drawboardBg, 0, 0);
        ctx.drawImage(drawboardLayer, 0, 0);
        temp.toBlob(resolve, 'image/png');
    });
}

function setDrawboardSending(sending) {
    drawboardSendBtn.disabled = sending;
    drawboardSendBtn.innerHTML = sending ? '<i class="fas fa-spinner fa-pulse"></i>' : '<i class="fas fa-paper-plane"></i>';
}

async function sendDrawboardMessage() {
    const text = drawboardInput.value.trim();
    if (!text) {
        showToast('请输入文字内容', 'warning');
        return;
    }
    if (!drawboard.hasImage && !drawboard.dirty) {
        showToast('请先导入背景图或进行绘制', 'warning');
        return;
    }

    setDrawboardSending(true);
    try {
        const blob = await getDrawboardMergedBlob();
        if (!blob) throw new Error('合并画板失败');
        const file = new File([blob], `drawboard-${Date.now()}.png`, { type: 'image/png' });
        const fileUrl = await saveFile(file);
        const relSrc = fileUrl.replace(window.location.origin, '');

        const userMsg = {
            id: generateId(),
            role: 'user',
            categories: ['text', 'image'],
            content: text,
            attachments: [{ type: 'image', src: relSrc, label: '画板' }],
            timestamp: Date.now()
        };
        addMessage(userMsg);

        if (backendConnected) {
            const content = [
                { type: 'text', text },
                { type: 'image_url', image_url: { url: fileUrl } }
            ];
            await sendMessages([{ role: 'user', content }]);
        } else {
            showToast('离线模式：画板消息仅本地展示', 'info');
        }

        drawboardInput.value = '';
    } catch (err) {
        showToast('发送失败：' + (err.message || err), 'error');
    } finally {
        setDrawboardSending(false);
    }
}

function setupDrawboard() {
    initDrawboardCanvas();

    openDrawboardBtn.addEventListener('click', openDrawboard);
    closeDrawboardBtn.addEventListener('click', closeDrawboard);
    drawboardOverlay.addEventListener('click', (e) => {
        if (e.target === drawboardOverlay) closeDrawboard();
    });

    importBgBtn.addEventListener('click', () => bgFileInput.click());
    bgFileInput.addEventListener('change', (e) => {
        const file = e.target.files && e.target.files[0];
        if (file) importDrawboardBackground(file);
        bgFileInput.value = '';
    });
    clearDrawBtn.addEventListener('click', clearDrawboard);
    undoDrawBtn.addEventListener('click', undoDrawboard);

    document.querySelectorAll('.drawboard-tool[data-tool]').forEach(btn => {
        btn.addEventListener('click', () => setDrawboardTool(btn.dataset.tool, btn));
    });
    document.querySelectorAll('.drawboard-color').forEach(el => {
        el.addEventListener('click', () => setDrawboardColor(el.dataset.color, el));
    });
    document.querySelectorAll('.drawboard-size').forEach(el => {
        el.addEventListener('click', () => setDrawboardSize(el.dataset.size, el));
    });

    setDrawboardTool('draw', document.querySelector('.drawboard-tool[data-tool="draw"]'));
    setDrawboardColor('#e74c3c', document.querySelector('.drawboard-color[data-color="#e74c3c"]'));
    setDrawboardSize('8', document.querySelector('.drawboard-size[data-size="8"]'));

    drawboardLayer.addEventListener('mousedown', drawboardStart);
    drawboardLayer.addEventListener('mousemove', drawboardMove);
    drawboardLayer.addEventListener('mouseup', drawboardStop);
    drawboardLayer.addEventListener('mouseleave', drawboardStop);

    drawboardSendBtn.addEventListener('click', sendDrawboardMessage);
    drawboardInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            sendDrawboardMessage();
        }
    });

    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && drawboardOverlay.classList.contains('active')) {
            closeDrawboard();
        }
    });
}

// ---------- 清理 ----------
function cleanup() {
    manualClose = true;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    AudioQueue.stop();
    if (ws) {
        ws.onclose = null;
        ws.close();
        ws = null;
    }
    document.querySelectorAll('.echarts-container').forEach(c => {
        if (c._echartsInstance) c._echartsInstance.dispose();
    });
    if (musicIframe) {
        musicIframe.remove();
        musicIframe = null;
    }
    musicChannel.close();
}

// ---------- 初始化 ----------
/** 渲染引擎 iframe：指向智能体所在后端（36789），与主 WebSocket 连接保持一致 */
function setupRendererFrame() {
    const frame = document.getElementById('rendererFrame');
    if (!frame || !frame.dataset.src) return;
    frame.src = `${frame.dataset.src}&ws=${window.location.hostname}:36789`;
}

async function init() {
    loadTheme();
    setupThemeToggle();
    loadVoiceAutoPlay();
    setupVoiceToggle();
    setupTabEvents();
    setupSearchEvents();
    setupDragEvents();
    setupInputEvents();
    setupMessageAreaDelegation();
    setupDrawboard();
    setupCapture();
    setupScrollControls();
    initMusicRenderer();
    setupRendererFrame();
    await ensureMarked();
    initMermaid();
    await loadPersistedMessages();
    updateEmptyState();
    connectWebSocket();
    window.addEventListener('beforeunload', cleanup);
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
