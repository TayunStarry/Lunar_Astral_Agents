// ============================================================
//  星月智能 · 消息终端 — 画板：画布初始化 / 状态 / 工具设置
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
