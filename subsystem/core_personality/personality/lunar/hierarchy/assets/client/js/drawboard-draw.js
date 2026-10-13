// ============================================================
//  星月智能 · 消息终端 — 画板：开关 / 背景导入 / 绘制交互
// ============================================================

function openDrawboard() {
    drawboardOverlay.classList.add('active');
    drawboardOverlay.setAttribute('aria-hidden', 'false');
    setTimeout(() => drawboardInput.focus(), 50);
}

function closeDrawboard() {
    drawboardOverlay.classList.remove('active');
    drawboardOverlay.setAttribute('aria-hidden', 'true');
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
