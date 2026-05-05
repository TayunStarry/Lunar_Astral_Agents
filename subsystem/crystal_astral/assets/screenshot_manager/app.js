/* ============================================================
   app.js - 主入口文件，整合所有模块
   ============================================================ */

import { elements, initCanvasContexts } from './dom.js';
import { initUI, showToast, setStatus } from './ui.js';
import { ScreenshotCore } from './screenshot-core.js';
import { DrawingTools } from './drawing-tools.js';
import { HistoryManager } from './history-manager.js';
import { ToolSelector } from './tool-selector.js';

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
