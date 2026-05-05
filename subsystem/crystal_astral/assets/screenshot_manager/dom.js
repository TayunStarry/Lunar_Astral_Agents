/* ============================================================
   dom.js - DOM 元素引用与初始化
   ============================================================ */

export const elements = {
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
export function initCanvasContexts() {
    elements.canvasCtx = elements.canvas.getContext('2d', { willReadFrequently: true });
    elements.drawCtx = elements.drawCanvas.getContext('2d', { willReadFrequently: true });
    elements.previewCtx = elements.previewCanvas.getContext('2d', { willReadFrequently: true });
}
