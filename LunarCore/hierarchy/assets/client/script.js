class WebSocketClient {
    url;
    ws = null;
    messageHandlers = [];
    connectionHandlers = [];
    errorHandlers = [];
    reconnectAttempts = 0;
    maxReconnectAttempts = 5;
    reconnectDelay = 3000;
    reconnectTimer = null;
    constructor(url) {
        this.url = url;
    }
    connect() {
        if (this.ws?.readyState === WebSocket.OPEN) {
            return;
        }
        try {
            this.ws = new WebSocket(this.url);
            this.setupEventListeners();
        }
        catch (error) {
            console.error('WebSocket connection error:', error);
            this.scheduleReconnect();
        }
    }
    setupEventListeners() {
        if (!this.ws)
            return;
        this.ws.onopen = () => {
            console.log('WebSocket connected');
            this.reconnectAttempts = 0;
            this.connectionHandlers.forEach(handler => handler());
        };
        this.ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                this.messageHandlers.forEach(handler => handler(message));
            }
            catch (error) {
                console.error('Failed to parse WebSocket message:', error);
            }
        };
        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            this.errorHandlers.forEach(handler => handler(error));
        };
        this.ws.onclose = () => {
            console.log('WebSocket closed');
            this.scheduleReconnect();
        };
    }
    scheduleReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('Max reconnection attempts reached');
            return;
        }
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
        }
        this.reconnectTimer = window.setTimeout(() => {
            this.reconnectAttempts++;
            console.log(`Reconnecting... attempt ${this.reconnectAttempts}`);
            this.connect();
        }, this.reconnectDelay);
    }
    disconnect() {
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
    }
    send(data) {
        if (this.ws?.readyState === WebSocket.OPEN) {
            const message = typeof data === 'string' ? data : JSON.stringify(data);
            this.ws.send(message);
        }
        else {
            console.warn('WebSocket is not connected');
        }
    }
    onMessage(handler) {
        this.messageHandlers.push(handler);
    }
    onConnect(handler) {
        this.connectionHandlers.push(handler);
    }
    onError(handler) {
        this.errorHandlers.push(handler);
    }
    isConnected() {
        return this.ws?.readyState === WebSocket.OPEN;
    }
}

const BORDER_COLORS = [
    'var(--status-218838)',
    'var(--status-3a5a8a)',
    'var(--status-4a6fa5)',
    'var(--status-6c9bcf)',
    'var(--status-8a2be2)',
    'var(--status-9d6bff)',
    'var(--status-dc3545)',
    'var(--status-fbbf24)',
    'var(--status-ffc107)',
    'var(--status-20c997)',
    'var(--status-ff6b9c)',
];
function randomBorderColor() {
    return BORDER_COLORS[Math.floor(Math.random() * BORDER_COLORS.length)];
}
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}
function processThinkTags(content) {
    return content
        .replace(/<think>/gi, '<details class="think-block"><summary class="toggle_think_button">思考过程</summary>')
        .replace(/<\/think>/gi, '</details>');
}
async function renderMarkdown(content) {
    if (window.marked) {
        let html = await window.marked.parse(content);
        html = processThinkTags(html);
        return html;
    }
    return escapeHtml(content);
}
function highlightCode(container) {
    container.querySelectorAll('pre code').forEach((block) => {
        if (window.hljs) {
            window.hljs.highlightElement(block);
        }
    });
}
async function renderMermaid(container) {
    const mermaidBlocks = container.querySelectorAll('code.language-mermaid');
    for (const block of Array.from(mermaidBlocks)) {
        const textContent = block.textContent || '';
        if (textContent.length <= 20)
            continue;
        try {
            const graphDefinition = textContent;
            await window.mermaid.parse(graphDefinition);
            const id = `mermaid-${Date.now()}-${Math.floor(Math.random() * 10000)}`;
            const { svg } = await window.mermaid.render(id, graphDefinition);
            const mermaidContainer = document.createElement('div');
            mermaidContainer.className = 'mermaid-container';
            const parser = new DOMParser();
            const doc = parser.parseFromString(svg, 'image/svg+xml');
            const svgElement = doc.documentElement;
            const chartType = svgElement.getAttribute('aria-roledescription');
            if (chartType === 'flowchart' || chartType === 'classDiagram') {
                const viewBox = svgElement.getAttribute('viewBox');
                if (viewBox) {
                    const values = viewBox.split(/\s+/).map(parseFloat);
                    if (values.length === 4 && values.every(v => !isNaN(v))) {
                        if (chartType === 'flowchart') {
                            values[0] *= 0.45;
                            values[1] *= 0.45;
                            values[2] *= 1.05;
                            values[3] *= 1.05;
                        }
                        else {
                            values[0] *= 0;
                            values[1] *= 0.35;
                            values[2] *= 1.05;
                            values[3] *= 1.25;
                        }
                        svgElement.setAttribute('viewBox', values.join(' '));
                    }
                }
            }
            const modifiedSVG = new XMLSerializer().serializeToString(svgElement);
            mermaidContainer.innerHTML = `<div style="width: 100%; border: 10px dashed #eee; padding: 0px">${modifiedSVG}</div>`;
            const parent = block.parentElement;
            if (parent) {
                parent.insertBefore(mermaidContainer, block);
                parent.removeChild(block);
            }
        }
        catch (error) {
            console.error('Mermaid rendering error:', error);
            const errorContainer = document.createElement('div');
            errorContainer.className = 'mermaid-error';
            errorContainer.textContent = error instanceof Error ? error.message : 'Mermaid rendering failed';
            const parent = block.parentElement;
            if (parent) {
                parent.insertBefore(errorContainer, block);
            }
        }
    }
}
function renderECharts(container) {
    container.querySelectorAll('.echarts-placeholder').forEach(async (placeholder) => {
        try {
            const chartData = placeholder.getAttribute('data-chart');
            if (!chartData)
                return;
            const config = JSON.parse(chartData);
            const chartContainer = document.createElement('div');
            chartContainer.style.width = '100%';
            chartContainer.style.height = '400px';
            placeholder.appendChild(chartContainer);
            const chart = window.echarts.init(chartContainer);
            chart.setOption(config);
        }
        catch (error) {
            console.error('ECharts rendering error:', error);
        }
    });
}
async function renderMath(container) {
    if (window.katex && window.renderMathInElement) {
        try {
            window.renderMathInElement(container, {
                delimiters: [
                    { left: '$$', right: '$$', display: true },
                    { left: '$', right: '$', display: false },
                    { left: '\\[', right: '\\]', display: true },
                    { left: '\\(', right: '\\)', display: false },
                ],
                throwOnError: false,
            });
        }
        catch (error) {
            console.error('KaTeX rendering error:', error);
        }
    }
}
function encodeFileName(filename) {
    return btoa(unescape(encodeURIComponent(filename)));
}
function clearContainer(container) {
    container.innerHTML = '';
}
function previewImage(url, alt) {
    const overlay = document.createElement('div');
    overlay.style.cssText = `
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background: rgba(0, 0, 0, 0.9);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 9999;
    cursor: pointer;
  `;
    const img = document.createElement('img');
    img.src = url;
    img.style.cssText = 'max-width: 90%; max-height: 90%; object-fit: contain;';
    overlay.appendChild(img);
    overlay.onclick = () => overlay.remove();
    document.body.appendChild(overlay);
}

function createFilePreview(file) {
    return {
        file,
        url: URL.createObjectURL(file),
        type: getFileType(file),
        name: file.name,
    };
}
function getFileType(file) {
    if (file.type.startsWith('image/'))
        return 'image';
    if (file.type.startsWith('video/'))
        return 'video';
    return 'text';
}
function isMediaFile(file) {
    return file.type.startsWith('image/') || file.type.startsWith('video/');
}
function revokeFilePreview(preview) {
    if (preview.url.startsWith('blob:')) {
        URL.revokeObjectURL(preview.url);
    }
}
function revokeAllFilePreviews(previews) {
    previews.forEach(revokeFilePreview);
}
async function getVideoThumbnail(file) {
    return new Promise((resolve, reject) => {
        const video = document.createElement('video');
        video.preload = 'metadata';
        video.muted = true;
        video.onloadeddata = () => {
            video.currentTime = 1;
        };
        video.onseeked = () => {
            const canvas = document.createElement('canvas');
            canvas.width = video.videoWidth;
            canvas.height = video.videoHeight;
            const ctx = canvas.getContext('2d');
            if (ctx) {
                ctx.drawImage(video, 0, 0);
                resolve(canvas.toDataURL('image/jpeg'));
            }
            else {
                reject(new Error('Failed to get video context'));
            }
        };
        video.onerror = () => {
            reject(new Error('Failed to load video'));
        };
        video.src = URL.createObjectURL(file);
    });
}
async function saveFile(file, overwrite = false) {
    const encodedFileName = encodeFileName(file.name);
    const response = await fetch('/save', {
        method: 'POST',
        headers: {
            'X-File-Name': encodedFileName,
            'X-Overwrite': overwrite.toString(),
            'Content-Length': file.size.toString(),
        },
        body: file,
    });
    if (!response.ok) {
        const errorData = await response.json().catch(() => ({ message: 'Upload failed' }));
        throw new Error(errorData.message || `HTTP error! status: ${response.status}`);
    }
    return response.json();
}
async function sendMessages(messages) {
    const response = await fetch('/write/message', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({ messages }),
    });
    if (!response.ok) {
        const errorData = await response.json().catch(() => ({ message: 'Request failed' }));
        throw new Error(errorData.message || `HTTP error! status: ${response.status}`);
    }
    return response.json();
}
async function fetchLive2DSetting() {
    try {
        const response = await fetch('/read/resources/live2d/setting.json');
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const rawText = await response.text();
        const jsonText = rawText
            .replace(/\/\/.*$/gm, '')
            .replace(/\/\*[\s\S]*?\*\//g, '')
            .replace(/'/g, '"');
        return JSON.parse(jsonText);
    }
    catch (error) {
        console.error('Failed to fetch Live2D setting:', error);
        return {};
    }
}

let pixiJSExample = null;
let Live2DModelInstance = null;
let currentLive2DModel = null;
let currentEmotionState = 'IDLE';
const EmotionalStateEnum = {
    IDLE: 'IDLE',
    THINKING: 'THINKING',
    AWAIT: 'AWAIT',
    SPEAKING: 'SPEAKING',
    HAPPY: 'HAPPY',
    SAD: 'SAD',
    ANGRY: 'ANGRY',
};
const Live2D = {
    async init() {
        const errorDiv = document.querySelector('.live2d-error-message');
        if (errorDiv)
            errorDiv.remove();
        try {
            await this.waitForPIXI();
            await this.loadLive2DPlugin();
            this.initApplication();
            currentLive2DModel = (await fetchLive2DSetting());
            await this.loadModel();
            window.addEventListener('resize', () => this.reloadContainer());
            this.setEmotionState(EmotionalStateEnum.IDLE);
            this.reloadContainer();
        }
        catch (error) {
            if (error instanceof Error) {
                this.showError(`初始化失败: ${error.message}`);
            }
            throw error;
        }
    },
    waitForPIXI() {
        return new Promise((resolve) => {
            const check = () => {
                if (window.PIXI) {
                    resolve();
                }
                else {
                    setTimeout(check, 100);
                }
            };
            check();
        });
    },
    loadLive2DPlugin() {
        return new Promise((resolve) => {
            const check = () => {
                if (window.PIXI?.live2d) {
                    resolve();
                }
                else {
                    setTimeout(check, 100);
                }
            };
            check();
        });
    },
    initApplication() {
        if (pixiJSExample) {
            pixiJSExample.destroy(true);
        }
        const container = document.getElementById('live2dContainer');
        const wasHidden = container?.parentElement?.style.display === 'none';
        if (wasHidden && container?.parentElement) {
            container.parentElement.style.display = 'block';
            container.parentElement.style.visibility = 'hidden';
        }
        const canvas = document.getElementById('live2dCanvas');
        const parameters = {
            transparent: true,
            width: container?.clientWidth || 0,
            height: container?.clientHeight || 0,
            view: canvas,
            antialias: true,
        };
        pixiJSExample = new window.PIXI.Application(parameters);
        const modelInfo = document.getElementById('modelIntel');
        if (modelInfo) {
            modelInfo.textContent = `加载模型: ${currentLive2DModel?.name || '未知'}...`;
        }
        if (wasHidden && container?.parentElement) {
            container.parentElement.style.display = 'none';
            container.parentElement.style.visibility = 'visible';
        }
    },
    async loadModel() {
        const modelInfo = document.getElementById('modelIntel');
        try {
            if (Live2DModelInstance) {
                pixiJSExample?.stage.removeChild(Live2DModelInstance);
                Live2DModelInstance.destroy();
                Live2DModelInstance = null;
            }
            if (modelInfo) {
                modelInfo.textContent = `加载模型: ${currentLive2DModel?.name || '未知'}...`;
            }
            if (!currentLive2DModel) {
                throw new Error('No Live2D model configured');
            }
            const model = await window.PIXI.live2d.Live2DModel.from(currentLive2DModel.url, { autoInteract: currentLive2DModel.autoInteract });
            Live2DModelInstance = model;
            model.scale.set(currentLive2DModel.scale);
            model.anchor.set(0.5, 0.5);
            const container = document.getElementById('live2dContainer');
            model.x = (container?.clientWidth || 0) * currentLive2DModel.x;
            model.y = (container?.clientHeight || 0) * currentLive2DModel.y;
            pixiJSExample?.stage.addChild(Live2DModelInstance);
            if (modelInfo) {
                modelInfo.textContent = currentLive2DModel?.name || '未知';
            }
        }
        catch (error) {
            if (error instanceof Error) {
                this.showError(`Live2D 加载失败: ${error.message}`);
                if (modelInfo) {
                    modelInfo.textContent = 'Live2D 加载失败';
                }
            }
            throw error;
        }
    },
    showError(message) {
        const container = document.getElementById('live2dContainer');
        if (!container)
            return;
        const errorDiv = document.createElement('div');
        errorDiv.className = 'live2d-error-message';
        errorDiv.innerHTML = `
      <h2><i class="fas fa-exclamation-triangle"></i> 出错了</h2>
      <p>${message}</p>
      <p>请检查控制台获取详细信息</p>
      <button id="reload-btn" style="margin-top: 20px; padding: 10px 20px; cursor: pointer;">重新加载</button>
    `;
        container.appendChild(errorDiv);
        document.getElementById('reload-btn')?.addEventListener('click', () => {
            errorDiv.remove();
            this.init();
        });
    },
    reloadContainer() {
        const container = document.getElementById('live2dContainer');
        if (!container)
            return;
        if (pixiJSExample) {
            pixiJSExample.renderer.resize(container.clientWidth, container.clientHeight);
        }
        if (Live2DModelInstance && currentLive2DModel) {
            const scale = container.clientHeight < 500
                ? currentLive2DModel.scale * 0.65
                : currentLive2DModel.scale;
            Live2DModelInstance.scale.x = scale;
            Live2DModelInstance.scale.y = scale;
            Live2DModelInstance.x = container.clientWidth * currentLive2DModel.x;
            Live2DModelInstance.y = container.clientHeight * currentLive2DModel.y;
        }
    },
    setEmotionState(state) {
        currentEmotionState = state;
        if (Live2DModelInstance && Live2DModelInstance.motion) {
            const motionMap = {
                IDLE: 'idle',
                THINKING: 'thinking',
                AWAIT: 'waiting',
                SPEAKING: 'speaking',
                HAPPY: 'happy',
                SAD: 'sad',
                ANGRY: 'angry',
            };
            const motion = motionMap[state];
            if (motion) {
                try {
                    Live2DModelInstance.motion(motion);
                }
                catch (e) {
                    console.warn(`Motion ${motion} not available:`, e);
                }
            }
        }
    },
    getCurrentEmotionState() {
        return currentEmotionState;
    },
    setStateWithTimeout(state, duration = 9000) {
        this.setEmotionState(state);
        if (state !== EmotionalStateEnum.IDLE && state !== EmotionalStateEnum.THINKING) {
            setTimeout(() => {
                if (currentEmotionState === state) {
                    this.setEmotionState(EmotionalStateEnum.IDLE);
                }
            }, duration);
        }
    },
    getModel() {
        return Live2DModelInstance;
    },
    isReady() {
        return Live2DModelInstance !== null;
    },
};

function createMessageElement(message) {
    const messageElement = document.createElement('div');
    messageElement.classList.add('message');
    if (message.role === 'user') {
        messageElement.classList.add('user-message');
    }
    else {
        messageElement.classList.add('assistant-message');
    }
    messageElement.style.borderColor = randomBorderColor();
    const header = document.createElement('div');
    header.className = 'message-header';
    header.textContent = message.role === 'user' ? '你' : '月华';
    const contentDiv = document.createElement('div');
    contentDiv.className = 'message-content';
    messageElement.appendChild(header);
    messageElement.appendChild(contentDiv);
    return messageElement;
}
async function renderMessage(message, container) {
    const messageElement = createMessageElement(message);
    const contentDiv = messageElement.querySelector('.message-content');
    if (message.imageUrls && message.imageUrls.length > 0) {
        const imagesContainer = document.createElement('div');
        imagesContainer.className = 'images-container';
        for (const imageUrl of message.imageUrls) {
            const imgContainer = document.createElement('div');
            imgContainer.className = 'labeled-image-container';
            const img = document.createElement('img');
            img.src = imageUrl;
            img.className = 'image-just-drawn';
            img.alt = typeof message.content === 'string' ? message.content : '图片';
            img.onerror = () => {
                img.src = `/read/resources/placeholder/blank-0${Math.floor(Math.random() * 3)}.png`;
            };
            img.onclick = () => previewImage(imageUrl, typeof message.content === 'string' ? message.content : '图片');
            imgContainer.appendChild(img);
            imagesContainer.appendChild(imgContainer);
        }
        contentDiv.appendChild(imagesContainer);
    }
    else if (message.imageUrl) {
        const imgContainer = document.createElement('div');
        imgContainer.className = 'labeled-image-container';
        const img = document.createElement('img');
        img.src = message.imageUrl;
        img.className = 'image-just-drawn';
        img.alt = typeof message.content === 'string' ? message.content : '图片';
        img.onerror = () => {
            img.src = `/read/resources/placeholder/blank-0${Math.floor(Math.random() * 3)}.png`;
        };
        img.onclick = () => previewImage(message.imageUrl, typeof message.content === 'string' ? message.content : '图片');
        imgContainer.appendChild(img);
        contentDiv.appendChild(imgContainer);
    }
    if (typeof message.content === 'string' && message.content) {
        const markdownContent = document.createElement('div');
        markdownContent.className = 'markdown-content';
        markdownContent.innerHTML = await renderMarkdown(message.content);
        contentDiv.appendChild(markdownContent);
        highlightCode(markdownContent);
        await renderMermaid(markdownContent);
        renderECharts(markdownContent);
        await renderMath(markdownContent);
    }
    container.appendChild(messageElement);
    container.scrollTo({ top: container.scrollHeight, behavior: 'smooth' });
    return messageElement;
}
async function renderAllMessages(container, messages, clearFirst = true) {
    if (clearFirst) {
        clearContainer(container);
    }
    for (const message of messages) {
        await renderMessage(message, container);
    }
}

class LunarCoreApp {
    wsClient = null;
    historyMessages = [];
    filePreviews = [];
    isModalOpen = false;
    isLoading = false;
    messageInput = null;
    sendButton = null;
    filePreviewArea = null;
    chatHistoryPanel = null;
    modalOverlay = null;
    modalBody = null;
    errorToast = null;
    constructor() {
        this.initElements();
        this.initEventListeners();
        this.initWebSocket();
        this.initLive2D();
    }
    initElements() {
        this.messageInput = document.getElementById('messageInput');
        this.sendButton = document.getElementById('sendButton');
        this.filePreviewArea = document.getElementById('filePreviewArea');
        this.chatHistoryPanel = document.getElementById('chatHistoryPanel');
        this.modalOverlay = document.getElementById('modalOverlay');
        this.modalBody = document.getElementById('modalBody');
        this.errorToast = document.getElementById('errorToast');
        document.getElementById('chatHistoryButton')?.addEventListener('click', () => this.toggleModal());
        document.getElementById('modalClose')?.addEventListener('click', () => this.toggleModal());
        this.modalOverlay?.addEventListener('click', (e) => {
            if (e.target === this.modalOverlay)
                this.toggleModal();
        });
    }
    initEventListeners() {
        this.sendButton?.addEventListener('click', () => this.handleSend());
        this.messageInput?.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.handleSend();
            }
        });
        this.messageInput?.addEventListener('input', () => {
            this.autoResizeTextarea();
        });
        this.setupDragAndDrop();
    }
    setupDragAndDrop() {
        const inputContainer = document.getElementById('inputContainer');
        ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
            inputContainer?.addEventListener(eventName, (e) => {
                e.preventDefault();
                e.stopPropagation();
            });
        });
        ['dragenter', 'dragover'].forEach(eventName => {
            inputContainer?.addEventListener(eventName, () => {
                inputContainer.classList.add('drag-over');
            });
        });
        ['dragleave', 'drop'].forEach(eventName => {
            inputContainer?.addEventListener(eventName, () => {
                inputContainer.classList.remove('drag-over');
            });
        });
        inputContainer?.addEventListener('drop', async (e) => {
            const files = e.dataTransfer?.files;
            if (files) {
                await this.handleFileSelect(Array.from(files));
            }
        });
    }
    async handleFileSelect(files) {
        for (const file of files) {
            const preview = createFilePreview(file);
            this.filePreviews.push(preview);
            this.renderFilePreview(preview);
        }
    }
    renderFilePreview(preview) {
        const item = document.createElement('div');
        item.className = 'file-preview-item';
        item.dataset.name = preview.name;
        if (preview.type === 'image') {
            const img = document.createElement('img');
            img.src = preview.url;
            item.appendChild(img);
        }
        else if (preview.type === 'video') {
            const video = document.createElement('video');
            video.src = preview.url;
            video.muted = true;
            item.appendChild(video);
            getVideoThumbnail(preview.file).then(thumbnail => {
                const img = document.createElement('img');
                img.src = thumbnail;
                item.insertBefore(img, video);
                video.style.display = 'none';
            }).catch(e => {
                console.warn('Failed to get video thumbnail:', e);
            });
        }
        else {
            const icon = document.createElement('i');
            icon.className = 'fas fa-file-alt';
            icon.style.cssText = 'font-size: 24px; color: white; display: flex; align-items: center; justify-content: center; height: 100%;';
            item.appendChild(icon);
        }
        const label = document.createElement('div');
        label.className = 'file-label';
        label.textContent = preview.name;
        item.appendChild(label);
        const removeBtn = document.createElement('button');
        removeBtn.className = 'remove-btn';
        removeBtn.innerHTML = '<i class="fas fa-times"></i>';
        removeBtn.onclick = () => this.removeFilePreview(preview, item);
        item.appendChild(removeBtn);
        this.filePreviewArea?.appendChild(item);
    }
    removeFilePreview(preview, item) {
        const index = this.filePreviews.indexOf(preview);
        if (index > -1) {
            this.filePreviews.splice(index, 1);
        }
        if (preview.url.startsWith('blob:')) {
            URL.revokeObjectURL(preview.url);
        }
        item.remove();
    }
    autoResizeTextarea() {
        if (this.messageInput) {
            this.messageInput.style.height = 'auto';
            this.messageInput.style.height = Math.min(this.messageInput.scrollHeight, 120) + 'px';
        }
    }
    initWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//localhost:36797/ws`;
        this.wsClient = new WebSocketClient(wsUrl);
        this.wsClient.onConnect(() => {
            console.log('WebSocket connected');
            this.setLoadingState(false);
        });
        this.wsClient.onMessage((message) => {
            this.handleWebSocketMessage(message);
        });
        this.wsClient.onError((error) => {
            console.error('WebSocket error:', error);
            this.showError('连接错误，请刷新页面');
        });
        this.wsClient.connect();
    }
    async handleWebSocketMessage(message) {
        switch (message.type) {
            case 'context':
                if (message.data.type === 'response' || message.data.type === 'active') {
                    await this.handleAssistantMessage(message.data.content || '');
                }
                break;
            case 'image':
                if (message.data.images) {
                    for (const imageBase64 of message.data.images) {
                        const byteString = atob(imageBase64);
                        const mimeType = 'image/jpeg';
                        const ab = new ArrayBuffer(byteString.length);
                        const ia = new Uint8Array(ab);
                        for (let i = 0; i < byteString.length; i++) {
                            ia[i] = byteString.charCodeAt(i);
                        }
                        const blob = new Blob([ab], { type: mimeType });
                        const file = new File([blob], `assistant_${Date.now()}.jpg`, { type: 'image/jpeg' });
                        const saveResult = await saveFile(file, true);
                        const fileUrl = `${window.location.origin}/read/${saveResult.filename}`;
                        await this.handleAssistantMessage('', fileUrl);
                    }
                }
                break;
            case 'error':
                this.showError(message.data.content || '发生错误');
                this.setLoadingState(false);
                Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
                break;
        }
    }
    async handleAssistantMessage(content, imageUrl) {
        const assistantMessage = {
            role: 'assistant',
            content: content,
            imageUrl: imageUrl || undefined,
            timestamp: Date.now(),
        };
        this.historyMessages.push(assistantMessage);
        await renderMessage(assistantMessage, this.chatHistoryPanel);
        if (this.isModalOpen) {
            await renderMessage(assistantMessage, this.modalBody);
        }
        this.setLoadingState(false);
        Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
    }
    async handleSend() {
        const text = this.messageInput?.value.trim();
        const hasFiles = this.filePreviews.length > 0;
        if (!text && !hasFiles) {
            return;
        }
        this.setLoadingState(true);
        Live2D.setEmotionState(EmotionalStateEnum.AWAIT);
        try {
            const openAIMessages = [];
            let userContent;
            const uploadedImageUrls = [];
            if (hasFiles) {
                const contentBlocks = [];
                for (const preview of this.filePreviews) {
                    if (isMediaFile(preview.file)) {
                        const saveResult = await saveFile(preview.file, false);
                        const fileUrl = `${window.location.origin}/read/${saveResult.filename}`;
                        if (preview.type === 'image') {
                            contentBlocks.push({
                                type: 'image_url',
                                image_url: { url: fileUrl },
                            });
                            uploadedImageUrls.push(fileUrl);
                        }
                        else if (preview.type === 'video') {
                            contentBlocks.push({
                                type: 'image_url',
                                image_url: { url: fileUrl },
                            });
                            uploadedImageUrls.push(fileUrl);
                        }
                    }
                }
                if (text) {
                    contentBlocks.push({ type: 'text', text });
                }
                userContent = contentBlocks;
            }
            else {
                userContent = text;
            }
            openAIMessages.push({
                role: 'user',
                content: userContent,
            });
            const userMessage = {
                role: 'user',
                content: text || (hasFiles ? '[文件]' : ''),
                timestamp: Date.now(),
            };
            if (uploadedImageUrls.length > 0) {
                userMessage.imageUrls = uploadedImageUrls;
            }
            this.historyMessages.push(userMessage);
            await renderMessage(userMessage, this.chatHistoryPanel);
            if (this.isModalOpen) {
                await renderMessage(userMessage, this.modalBody);
            }
            await sendMessages(openAIMessages);
            this.messageInput.value = '';
            this.clearFilePreviews();
            this.autoResizeTextarea();
        }
        catch (error) {
            console.error('Send error:', error);
            this.showError(error instanceof Error ? error.message : '发送失败');
            this.setLoadingState(false);
            Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
        }
    }
    clearFilePreviews() {
        revokeAllFilePreviews(this.filePreviews);
        this.filePreviews = [];
        if (this.filePreviewArea) {
            this.filePreviewArea.innerHTML = '';
        }
    }
    setLoadingState(loading) {
        this.isLoading = loading;
        if (this.sendButton) {
            this.sendButton.disabled = loading;
            if (loading) {
                this.sendButton.innerHTML = '<span class="loading-indicator"></span>';
            }
            else {
                this.sendButton.innerHTML = '<i class="fas fa-paper-plane"></i>';
            }
        }
    }
    toggleModal() {
        this.isModalOpen = !this.isModalOpen;
        if (this.isModalOpen) {
            this.modalOverlay?.classList.add('visible');
            if (this.modalBody) {
                this.modalBody.innerHTML = '';
            }
            renderAllMessages(this.modalBody, this.historyMessages, false);
        }
        else {
            this.modalOverlay?.classList.remove('visible');
        }
    }
    showError(message) {
        if (this.errorToast) {
            this.errorToast.textContent = message;
            this.errorToast.classList.add('visible');
            setTimeout(() => {
                this.errorToast?.classList.remove('visible');
            }, 3000);
        }
    }
    async initLive2D() {
        try {
            await Live2D.init();
        }
        catch (error) {
            console.error('Failed to initialize Live2D:', error);
            this.showError('Live2D 初始化失败');
        }
    }
}
document.addEventListener('DOMContentLoaded', () => {
    window.app = new LunarCoreApp();
});

