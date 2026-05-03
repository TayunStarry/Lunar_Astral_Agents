/* ============================================================
   drawing-tools.js - 绘图工具类
   ============================================================ */

export class DrawingTools {
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
