/* ============================================================
   dom.js - DOM 元素引用与初始化
   ============================================================ */

const elements = {
    // 画布元素
    canvas: document.getElementById('canvas'),
    canvasCtx: null,
    canvasWrapper: document.getElementById('canvasWrapper'),
    drawCanvas: document.getElementById('drawCanvas'),
    drawCtx: null,
    previewCanvas: document.getElementById('previewCanvas'),
    previewCtx: null,
    
    // 按钮元素
    captureSceneButton: document.getElementById('captureSceneButton'),
    continuousCaptureButton: document.getElementById('continuousCaptureButton'),
    downloadSceneButton: document.getElementById('downloadSceneButton'),
    undoDrawButton: document.getElementById('undoDrawButton'),
    
    // 滑块元素
    regionXSlider: document.getElementById('regionX'),
    regionYSlider: document.getElementById('regionY'),
    regionWidthSlider: document.getElementById('regionWidth'),
    regionHeightSlider: document.getElementById('regionHeight'),
    intervalSlider: document.getElementById('captureInterval'),
    intervalValueDisplay: document.getElementById('intervalValue'),
    scaleSlider: document.getElementById('scaleSlider'),
    scaleValueDisplay: document.getElementById('scaleValue'),
    
    // 消息区域
    messageInput: document.getElementById('messageInput'),
    sendMessageBtn: document.getElementById('sendMessageBtn'),
    connectionStatus: document.getElementById('connectionStatus'),
    toastContainer: document.getElementById('toastContainer'),
    
    // 绘图工具按钮
    rectTool: document.getElementById('rectTool'),
    drawTool: document.getElementById('drawTool'),
    lineTool: document.getElementById('lineTool'),
    circleTool: document.getElementById('circleTool'),
    textTool: document.getElementById('textTool'),
    arrowTool: document.getElementById('arrowTool')
};

// 初始化画布上下文
function initCanvasContexts() {
    elements.canvasCtx = elements.canvas.getContext('2d', { willReadFrequently: true });
    elements.drawCtx = elements.drawCanvas.getContext('2d', { willReadFrequently: true });
    elements.previewCtx = elements.previewCanvas.getContext('2d', { willReadFrequently: true });
}

/* ============================================================
   ui.js - Toast 提示与状态管理
   ============================================================ */

let toastContainer;
let connectionStatus;

function initUI(container, status) {
    toastContainer = container;
    connectionStatus = status;
}

// ==================== Toast 提示 ====================
function showToast(message, type = 'info', duration = 3000) {
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    toastContainer.appendChild(toast);
    setTimeout(() => toast.remove(), duration + 400);
}

// ==================== 状态指示 ====================
function setStatus(status) {
    connectionStatus.className = 'status-badge';
    if (status === 'sending') {
        connectionStatus.classList.add('sending');
        connectionStatus.innerHTML = '<i class="fas fa-circle"></i> 发送中...';
    } else if (status === 'error') {
        connectionStatus.classList.add('error');
        connectionStatus.innerHTML = '<i class="fas fa-circle"></i> 连接失败';
    } else {
        connectionStatus.innerHTML = '<i class="fas fa-circle"></i> 就绪';
    }
}

/* ============================================================
   screenshot-core.js - 截图核心功能
   ============================================================ */


class ScreenshotCore {
    constructor(domElements) {
        this.dom = domElements;
        this.continuousConfig = { enabled: false, interval: 1000, intervalId: null };
        this.screenshotConfig = { x: 0, y: 0, width: 3840, height: 2160 };
        this.scale = 1.0;
        this.historyManager = null;
    }

    set updateScale(v) { this.scale = v; }
    get updateScale() { return this.scale; }

    get maximumScreenshot() {
        const { width, height } = this.screenshotConfig;
        return width === 3840 && height === 2160;
    }

    async getScreenshot() {
        try {
            let response;
            if (this.maximumScreenshot) {
                response = await fetch('/capture', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ display_index: -1, scale: String(this.scale), format: 'png' })
                });
            } else {
                const { x, y, width, height } = this.screenshotConfig;
                const url = `/capture/region?region=${x},${y},${width},${height}&scale=${this.scale}&format=png`;
                response = await fetch(url);
            }

            if (!response.ok) throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            const blob = await response.blob();
            const img = await new Promise((resolve, reject) => {
                const img = new Image();
                img.onload = () => resolve(img);
                img.onerror = reject;
                img.src = URL.createObjectURL(blob);
            });

            const tempCanvas = document.createElement('canvas');
            tempCanvas.width = img.width;
            tempCanvas.height = img.height;
            const tempCtx = tempCanvas.getContext('2d');
            tempCtx.drawImage(img, 0, 0);
            return tempCtx.getImageData(0, 0, img.width, img.height);
        } catch (err) {
            console.error('截图失败:', err);
            showToast('截图失败: ' + err.message, 'error');
            return null;
        }
    }

    async captureScreen() {
        this.dom.captureSceneButton.disabled = true;
        this.dom.captureSceneButton.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 截取中...';
        const imageData = await this.getScreenshot();
        if (!imageData) {
            this.dom.captureSceneButton.disabled = false;
            this.dom.captureSceneButton.innerHTML = '<i class="fas fa-camera"></i> 捕捉画面';
            return;
        }

        this.dom.canvas.width = imageData.width;
        this.dom.canvas.height = imageData.height;
        this.dom.drawCanvas.width = imageData.width;
        this.dom.drawCanvas.height = imageData.height;
        this.dom.previewCanvas.width = imageData.width;
        this.dom.previewCanvas.height = imageData.height;

        this.dom.canvasCtx.putImageData(imageData, 0, 0);
        this.dom.drawCtx.clearRect(0, 0, this.dom.drawCanvas.width, this.dom.drawCanvas.height);
        this.dom.previewCtx.clearRect(0, 0, this.dom.previewCanvas.width, this.dom.previewCanvas.height);

        this.dom.canvasWrapper.classList.add('has-image');
        if (this.historyManager) this.historyManager.clear();
        this.dom.downloadSceneButton.disabled = false;
        this.dom.continuousCaptureButton.disabled = false;
        this.dom.captureSceneButton.disabled = false;
        this.dom.captureSceneButton.innerHTML = '<i class="fas fa-camera"></i> 捕捉画面';
        showToast('画面截取成功', 'success');
    }

    toggleContinuousCapture() {
        if (this.continuousConfig.enabled) {
            this.stopContinuousCapture();
            this.dom.continuousCaptureButton.innerHTML = '<i class="fas fa-play"></i> 持续捕捉';
        } else {
            this.startContinuousCapture();
            this.dom.continuousCaptureButton.innerHTML = '<i class="fas fa-pause"></i> 停止';
        }
    }

    async startContinuousCapture() {
        this.continuousConfig.enabled = true;
        await this.captureScreen();
        this.continuousConfig.intervalId = setInterval(() => this.captureScreen(), this.continuousConfig.interval);
    }

    stopContinuousCapture() {
        this.continuousConfig.enabled = false;
        if (this.continuousConfig.intervalId) {
            clearInterval(this.continuousConfig.intervalId);
            this.continuousConfig.intervalId = null;
        }
    }

    setCaptureInterval(ms) {
        this.continuousConfig.interval = ms;
        if (this.continuousConfig.enabled) {
            this.stopContinuousCapture();
            this.startContinuousCapture();
        }
    }

    set accessScreenshotConfig(cfg) { Object.assign(this.screenshotConfig, cfg); }
    get accessScreenshotConfig() { return { ...this.screenshotConfig }; }

    calculateScale() {
        const rect = this.dom.drawCanvas.getBoundingClientRect();
        return {
            scaleX: this.dom.drawCanvas.width / (rect.width || 1),
            scaleY: this.dom.drawCanvas.height / (rect.height || 1)
        };
    }

    async downloadImage() {
        if (!this.dom.canvas.width) return showToast('请先截取画面', 'warning');
        const tempCanvas = document.createElement('canvas');
        tempCanvas.width = this.dom.canvas.width;
        tempCanvas.height = this.dom.canvas.height;
        const ctx = tempCanvas.getContext('2d');
        ctx.drawImage(this.dom.canvas, 0, 0);
        ctx.drawImage(this.dom.drawCanvas, 0, 0);
        const blob = await new Promise(r => tempCanvas.toBlob(r, 'image/png'));
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `screenshot-${Date.now()}.png`;
        a.click();
        URL.revokeObjectURL(url);
        showToast('下载完成', 'success');
    }

    getMergedImageData() {
        if (!this.dom.canvas.width) return '';
        const tempCanvas = document.createElement('canvas');
        tempCanvas.width = this.dom.canvas.width;
        tempCanvas.height = this.dom.canvas.height;
        const ctx = tempCanvas.getContext('2d');
        ctx.drawImage(this.dom.canvas, 0, 0);
        ctx.drawImage(this.dom.drawCanvas, 0, 0);
        return tempCanvas.toDataURL('image/png');
    }

    reset() {
        this.stopContinuousCapture();
        this.dom.canvasWrapper.classList.remove('has-image');
        this.dom.canvas.width = this.dom.canvas.height = 0;
        this.dom.drawCanvas.width = this.dom.drawCanvas.height = 0;
        this.dom.previewCanvas.width = this.dom.previewCanvas.height = 0;
        this.dom.downloadSceneButton.disabled = true;
        this.dom.continuousCaptureButton.disabled = true;
        this.dom.captureSceneButton.disabled = false;
        this.dom.captureSceneButton.innerHTML = '<i class="fas fa-camera"></i> 捕捉画面';
        this.dom.continuousCaptureButton.innerHTML = '<i class="fas fa-play"></i> 持续捕捉';
        if (this.historyManager) this.historyManager.clear();
    }

    setHistoryManager(hm) {
        this.historyManager = hm;
    }
}

/* ============================================================
   drawing-tools.js - 绘图工具类
   ============================================================ */

class DrawingTools {
    constructor(domElements, core) {
        this.dom = domElements;
        this.core = core;
        this.isDrawing = false;
        this.lastX = this.lastY = this.startX = this.startY = 0;
        this.currentTool = 'draw';
        this.currentColor = '#e74c3c';
        this.currentSize = 16;
    }

    setTool(t) { this.currentTool = t; }
    setColor(c) { this.currentColor = c; }
    setSize(s) { this.currentSize = parseInt(s, 10); }

    getMousePos(e) {
        const rect = this.dom.drawCanvas.getBoundingClientRect();
        const { scaleX, scaleY } = this.core.calculateScale();
        return {
            x: (e.clientX - rect.left) * scaleX,
            y: (e.clientY - rect.top) * scaleY
        };
    }

    drawArrow(fromX, fromY, toX, toY, ctx) {
        const dx = toX - fromX, dy = toY - fromY;
        const angle = Math.atan2(dy, dx);
        const len = Math.sqrt(dx*dx+dy*dy);
        const head = Math.min(25, len*0.25) * (this.currentSize/10);
        ctx.save();
        ctx.strokeStyle = ctx.fillStyle = this.currentColor;
        ctx.lineWidth = this.currentSize;
        ctx.lineCap = 'round';
        ctx.beginPath(); ctx.moveTo(fromX, fromY); ctx.lineTo(toX - head*Math.cos(angle), toY - head*Math.sin(angle)); ctx.stroke();
        ctx.beginPath(); ctx.moveTo(toX, toY);
        ctx.lineTo(toX - head*Math.cos(angle - Math.PI/6), toY - head*Math.sin(angle - Math.PI/6));
        ctx.lineTo(toX - head*Math.cos(angle + Math.PI/6), toY - head*Math.sin(angle + Math.PI/6));
        ctx.closePath(); ctx.fill();
        ctx.restore();
    }

    start(e) {
        if (!this.dom.canvasWrapper.classList.contains('has-image')) return;
        this.isDrawing = true;
        const p = this.getMousePos(e);
        this.lastX = this.startX = p.x;
        this.lastY = this.startY = p.y;
        if (this.currentTool === 'draw') {
            this.dom.drawCtx.beginPath();
            this.dom.drawCtx.moveTo(p.x, p.y);
        }
    }

    move(e) {
        if (!this.isDrawing) return;
        const p = this.getMousePos(e);
        if (this.currentTool === 'draw') {
            this.dom.drawCtx.lineTo(p.x, p.y);
            this.dom.drawCtx.strokeStyle = this.currentColor;
            this.dom.drawCtx.lineWidth = this.currentSize;
            this.dom.drawCtx.lineCap = this.dom.drawCtx.lineJoin = 'round';
            this.dom.drawCtx.stroke();
        } else {
            this.dom.previewCtx.clearRect(0, 0, this.dom.previewCanvas.width, this.dom.previewCanvas.height);
            switch (this.currentTool) {
                case 'rect': this.dom.previewCtx.strokeStyle = this.currentColor; this.dom.previewCtx.lineWidth = this.currentSize; this.dom.previewCtx.strokeRect(this.startX, this.startY, p.x-this.startX, p.y-this.startY); break;
                case 'line': this.dom.previewCtx.beginPath(); this.dom.previewCtx.moveTo(this.startX, this.startY); this.dom.previewCtx.lineTo(p.x, p.y); this.dom.previewCtx.strokeStyle = this.currentColor; this.dom.previewCtx.lineWidth = this.currentSize; this.dom.previewCtx.stroke(); break;
                case 'circle': {
                    const rx = Math.abs(p.x-this.startX)/2, ry = Math.abs(p.y-this.startY)/2;
                    this.dom.previewCtx.beginPath(); this.dom.previewCtx.ellipse(this.startX+(p.x-this.startX)/2, this.startY+(p.y-this.startY)/2, rx, ry, 0, 0, 2*Math.PI);
                    this.dom.previewCtx.strokeStyle = this.currentColor; this.dom.previewCtx.lineWidth = this.currentSize; this.dom.previewCtx.stroke();
                    break;
                }
                case 'arrow': this.drawArrow(this.startX, this.startY, p.x, p.y, this.dom.previewCtx); break;
            }
        }
    }

    stop(e) {
        if (!this.isDrawing) return false;
        const p = this.getMousePos(e);
        let didDraw = false;
        if (this.currentTool === 'text') {
            const text = prompt('输入文本:', '标注');
            if (text) {
                this.dom.drawCtx.font = `bold ${20+this.currentSize*4}px sans-serif`;
                this.dom.drawCtx.fillStyle = this.currentColor;
                this.dom.drawCtx.fillText(text, p.x, p.y);
                didDraw = true;
            }
        } else if (this.currentTool === 'draw') {
            didDraw = true;
        } else {
            this.dom.drawCtx.drawImage(this.dom.previewCanvas, 0, 0);
            this.dom.previewCtx.clearRect(0, 0, this.dom.previewCanvas.width, this.dom.previewCanvas.height);
            didDraw = true;
        }
        this.isDrawing = false;
        return didDraw;
    }
}

/* ============================================================
   history-manager.js - 历史管理功能
   ============================================================ */

class HistoryManager {
    constructor(domElements) {
        this.dom = domElements;
        this.stack = [];
    }

    save() {
        if (this.dom.drawCanvas.width > 0) {
            this.stack.push(this.dom.drawCtx.getImageData(0, 0, this.dom.drawCanvas.width, this.dom.drawCanvas.height));
        }
        this.dom.undoDrawButton.disabled = this.stack.length === 0;
    }

    undo() {
        if (this.stack.length) {
            this.dom.drawCtx.putImageData(this.stack.pop(), 0, 0);
            this.dom.undoDrawButton.disabled = this.stack.length === 0;
        }
    }

    clear() {
        this.stack.length = 0;
        this.dom.undoDrawButton.disabled = true;
    }
}

/* ============================================================
   tool-selector.js - 工具选择器
   ============================================================ */

class ToolSelector {
    constructor(domElements) {
        this.dom = domElements;
        this.onTool = null;
        this.onColor = null;
        this.onSize = null;
    }

    setTool(t) {
        [this.dom.rectTool, this.dom.drawTool, this.dom.lineTool, this.dom.circleTool, this.dom.textTool, this.dom.arrowTool].forEach(b => b.classList.remove('clicking'));
        const map = { rect: this.dom.rectTool, draw: this.dom.drawTool, line: this.dom.lineTool, circle: this.dom.circleTool, text: this.dom.textTool, arrow: this.dom.arrowTool };
        if (map[t]) map[t].classList.add('clicking');
        this.dom.drawCanvas.style.cursor = t === 'text' ? 'text' : 'crosshair';
        if (this.onTool) this.onTool(t);
    }

    setColor(c, evt) {
        document.querySelectorAll('.line-color').forEach(b => b.classList.remove('clicking'));
        if (evt?.target) evt.target.classList.add('clicking');
        if (this.onColor) this.onColor(c);
    }

    setSize(s, evt) {
        document.querySelectorAll('.line-size').forEach(b => b.classList.remove('clicking'));
        if (evt?.target) evt.target.classList.add('clicking');
        if (this.onSize) this.onSize(s);
    }

    init() {
        this.dom.rectTool.onclick = () => this.setTool('rect');
        this.dom.drawTool.onclick = () => this.setTool('draw');
        this.dom.lineTool.onclick = () => this.setTool('line');
        this.dom.circleTool.onclick = () => this.setTool('circle');
        this.dom.textTool.onclick = () => this.setTool('text');
        this.dom.arrowTool.onclick = () => this.setTool('arrow');
        document.querySelectorAll('.line-color').forEach(el => el.onclick = (e) => this.setColor(el.dataset.color, e));
        document.querySelectorAll('.line-size').forEach(el => el.onclick = (e) => this.setSize(el.dataset.size, e));
    }

    default() {
        this.setTool('draw');
        this.setSize('16');
        const red = document.querySelector('.line-color[data-color="#e74c3c"]');
        if (red) this.setColor('#e74c3c', { target: red });
    }
}

/* ============================================================
   app.js - 主入口文件，整合所有模块
   ============================================================ */


let screenshotCore;
let drawingTools;
let historyManager;
let toolSelector;

window.onload = () => {
    // 初始化 DOM 上下文
    initCanvasContexts();

    // 初始化 UI
    initUI(elements.toastContainer, elements.connectionStatus);

    // 实例化核心模块
    screenshotCore = new ScreenshotCore(elements);
    drawingTools = new DrawingTools(elements, screenshotCore);
    historyManager = new HistoryManager(elements);
    toolSelector = new ToolSelector(elements);

    // 设置模块间的引用
    screenshotCore.setHistoryManager(historyManager);

    // 设置工具选择器回调
    toolSelector.onTool = (t) => drawingTools.setTool(t);
    toolSelector.onColor = (c) => drawingTools.setColor(c);
    toolSelector.onSize = (s) => drawingTools.setSize(s);
    toolSelector.init();
    toolSelector.default();

    // 绑定滑块事件
    bindSliderEvents();

    // 绑定按钮事件
    bindButtonEvents();

    // 绑定绘图事件
    bindDrawingEvents();

    // 绑定粘贴事件
    bindPasteEvents();

    // 初始化滑块显示
    initSliderDisplay();
};

function updateSliderDisplay(slider, span) {
    span.textContent = slider.value;
}

function updateRegionConfig() {
    screenshotCore.stopContinuousCapture();
    screenshotCore.accessScreenshotConfig = {
        x: +elements.regionXSlider.value,
        y: +elements.regionYSlider.value,
        width: +elements.regionWidthSlider.value,
        height: +elements.regionHeightSlider.value
    };
}

function bindSliderEvents() {
    // 声明防抖函数 (放在 bindSliderEvents 内部或外部均可)
    let captureTimer = null;
    const autoCapture = () => {
        if (captureTimer) clearTimeout(captureTimer);
        captureTimer = setTimeout(() => {
            screenshotCore.captureScreen();
        }, 200);
    };
    elements.regionXSlider.oninput = () => {
        updateSliderDisplay(elements.regionXSlider, elements.regionXSlider.nextElementSibling);
        updateRegionConfig();
    };
    elements.regionYSlider.oninput = () => {
        updateSliderDisplay(elements.regionYSlider, elements.regionYSlider.nextElementSibling);
        updateRegionConfig();
    };
    elements.regionWidthSlider.oninput = () => {
        updateSliderDisplay(elements.regionWidthSlider, elements.regionWidthSlider.nextElementSibling);
        updateRegionConfig();
    };
    elements.regionHeightSlider.oninput = () => {
        updateSliderDisplay(elements.regionHeightSlider, elements.regionHeightSlider.nextElementSibling);
        updateRegionConfig();
    };
    elements.intervalSlider.oninput = () => {
        elements.intervalValueDisplay.textContent = elements.intervalSlider.value + 'ms';
        screenshotCore.setCaptureInterval(+elements.intervalSlider.value);
    };
    elements.scaleSlider.oninput = () => {
        elements.scaleValueDisplay.textContent = parseFloat(elements.scaleSlider.value).toFixed(1) + 'x';
        screenshotCore.updateScale = +elements.scaleSlider.value;
    };
    elements.regionXSlider.onchange = autoCapture;
    elements.regionYSlider.onchange = autoCapture;
    elements.regionWidthSlider.onchange = autoCapture;
    elements.regionHeightSlider.onchange = autoCapture;
    elements.scaleSlider.onchange = autoCapture;
}

function bindButtonEvents() {
    elements.captureSceneButton.onclick = () => screenshotCore.captureScreen();
    elements.continuousCaptureButton.onclick = () => screenshotCore.toggleContinuousCapture();
    elements.downloadSceneButton.onclick = () => screenshotCore.downloadImage();
    elements.undoDrawButton.onclick = () => historyManager.undo();
    elements.sendMessageBtn.onclick = sendToAI;
    elements.messageInput.onkeydown = (e) => {
        if (e.key === 'Enter' && !e.ctrlKey && !e.shiftKey) {
            e.preventDefault();
            sendToAI();
        }
    };
}

function bindDrawingEvents() {
    elements.drawCanvas.onmousedown = (e) => {
        historyManager.save();
        drawingTools.start(e);
    };
    elements.drawCanvas.onmousemove = (e) => drawingTools.move(e);
    elements.drawCanvas.onmouseup = (e) => {
        if (drawingTools.stop(e) && drawingTools.currentTool === 'draw') {
            historyManager.save();
        }
    };
    elements.drawCanvas.onmouseleave = (e) => {
        if (drawingTools.stop(e) && drawingTools.currentTool === 'draw') {
            historyManager.save();
        }
    };
}

function bindPasteEvents() {
    document.onpaste = async (e) => {
        const items = e.clipboardData?.items;
        if (!items) return;
        for (const item of items) {
            if (item.type.startsWith('image/')) {
                e.preventDefault();
                const blob = item.getAsFile();
                const img = await new Promise((res, rej) => {
                    const i = new Image();
                    i.onload = () => res(i);
                    i.onerror = rej;
                    i.src = URL.createObjectURL(blob);
                });
                elements.canvas.width = elements.drawCanvas.width = elements.previewCanvas.width = img.width;
                elements.canvas.height = elements.drawCanvas.height = elements.previewCanvas.height = img.height;
                elements.canvasCtx.drawImage(img, 0, 0);
                elements.drawCtx.clearRect(0, 0, img.width, img.height);
                elements.previewCtx.clearRect(0, 0, img.width, img.height);
                elements.canvasWrapper.classList.add('has-image');
                historyManager.clear();
                elements.downloadSceneButton.disabled = false;
                elements.continuousCaptureButton.disabled = false;
                showToast('图片已粘贴', 'success');
                return;
            }
        }
    };
}

async function sendToAI() {
    const text = elements.messageInput.value.trim();
    const imageData = screenshotCore.getMergedImageData();
    if (!text && !imageData) return showToast('请输入消息或先截取画面', 'warning');

    const messages = [{ role: 'user', content: [] }];
    if (text) messages[0].content.push({ type: 'text', text });
    if (imageData) messages[0].content.push({ type: 'image_url', image_url: { url: imageData } });

    elements.sendMessageBtn.disabled = true;
    setStatus('sending');
    try {
        const res = await fetch('/write/message', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ messages })
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        showToast(`消息已发送 (队列长度: ${data.length})`, 'success');
        elements.messageInput.value = '';
        setStatus('ready');
    } catch (err) {
        showToast('发送失败: ' + err.message, 'error');
        setStatus('error');
    } finally {
        elements.sendMessageBtn.disabled = false;
    }
}

function initSliderDisplay() {
    updateSliderDisplay(elements.regionXSlider, elements.regionXSlider.nextElementSibling);
    updateSliderDisplay(elements.regionYSlider, elements.regionYSlider.nextElementSibling);
    updateSliderDisplay(elements.regionWidthSlider, elements.regionWidthSlider.nextElementSibling);
    updateSliderDisplay(elements.regionHeightSlider, elements.regionHeightSlider.nextElementSibling);
    elements.intervalValueDisplay.textContent = elements.intervalSlider.value + 'ms';
    elements.scaleValueDisplay.textContent = parseFloat(elements.scaleSlider.value).toFixed(1) + 'x';
    updateRegionConfig();
    elements.downloadSceneButton.disabled = true;
    elements.continuousCaptureButton.disabled = true;
}
