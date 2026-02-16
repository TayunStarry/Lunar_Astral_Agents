import * as EntryAPI from '../EntryAPI/code';

// 绘制工具功能
export class DrawingTools {
    /**
     * 是否正在绘制
     */
    private isDrawing: boolean = false;
    /**
     * 上一次鼠标X坐标（自由绘制用）
     */
    private lastX: number = 0;
    /**
     * 上一次鼠标Y坐标（自由绘制用）
     */
    private lastY: number = 0;
    /**
     * 起始点X坐标（矩形、直线、圆形、箭头用）
     */
    private startX: number = 0;
    /**
     * 起始点Y坐标（矩形、直线、圆形、箭头用）
     */
    private startY: number = 0;
    /**
     * 当前选中的绘图工具
     */
    public currentTool: EntryAPI.DrawingTool = 'draw';
    /**
     * 当前选中的线条颜色
     */
    public currentColor: EntryAPI.ColorHex = '#e74c3c';
    /**
     * 当前选中的线条粗细（像素）
     */
    public currentSize: number = 16;
    /**
     * 构造函数
     *
     * @param {HTMLCanvasElement} drawCanvas 主绘制画布
     *
     * @param {HTMLCanvasElement} previewCanvas 预览画布（实时显示矩形、直线等预览）
     *
     * @param {ScreenshotCore} screenshotCore 截图核心实例，用于计算坐标缩放比例
     */
    constructor(public drawCanvas: HTMLCanvasElement, public previewCanvas: HTMLCanvasElement, public screenshotCore: EntryAPI.ScreenshotCore) {
    }
    /**
     * 设置当前选中的绘图工具
     *
     * @param {EntryAPI.DrawingTool} tool 要设置的绘图工具（'draw', 'rect', 'line', 'circle', 'arrow'）
     */
    public setCurrentTool(tool: EntryAPI.DrawingTool): void {
        this.currentTool = tool;
    }
    /**
     * 设置当前选中的线条颜色
     *
     * @param {EntryAPI.ColorHex} color 要设置的线条颜色（十六进制字符串）
     */
    public setCurrentColor(color: EntryAPI.ColorHex): void {
        this.currentColor = color;
    }
    /**
     * 设置当前选中的线条粗细
     *
     * @param {string} size 要设置的线条粗细（像素）
     */
    public setCurrentSize(size: string): void {
        this.currentSize = parseInt(size, 10);
    }
    /**
     * 将鼠标事件中的视口坐标转换为画布上的实际像素坐标
     *
     * 先获取画布在视口中的位置与尺寸，再结合缩放比例换算出精确坐标
     *
     * @param event 鼠标事件对象，包含 clientX/clientY 等视口坐标
     *
     * @returns 转换后的画布坐标对象 {x, y}
     */
    public getMousePos(event: MouseEvent): EntryAPI.MousePosition {
        /** 获取画布在视口中的位置与尺寸 */
        const rect = this.drawCanvas.getBoundingClientRect();
        /** 计算当前缩放比例 */
        const scale = this.screenshotCore.calculateScale();
        // 计算实际画布坐标（考虑缩放比例）
        return {
            x: (event.clientX - rect.left) * scale.scaleX,
            y: (event.clientY - rect.top) * scale.scaleY
        };
    }
    /**
     * 绘制箭头
     *
     * 根据起始点和终点坐标，在当前颜色和线宽设置下绘制带箭头的线段
     *
     * @param {number} fromX 起始点X坐标
     *
     * @param {number} fromY 起始点Y坐标
     *
     * @param {number} toX 终点X坐标
     *
     * @param {number} toY 终点Y坐标
     *
     * @param {CanvasRenderingContext2D} context 画布2D绘图上下文
     */
    public drawArrow(fromX: number, fromY: number, toX: number, toY: number, context: CanvasRenderingContext2D): void {
        // 计算箭头方向向量
        const dx = toX - fromX;
        const dy = toY - fromY;
        // 计算箭头旋转角度
        const angle = Math.atan2(dy, dx);
        // 计算线段长度
        const length = Math.sqrt(dx * dx + dy * dy);
        // 根据线宽动态计算箭头头部大小，最大25像素，最小为长度的25%，并随线宽缩放
        const headLength = Math.min(25, length * 0.25) * (this.currentSize / 10);
        // 计算箭头线段终点（留出头部空间）
        const lineEndX = toX - headLength * Math.cos(angle);
        const lineEndY = toY - headLength * Math.sin(angle);
        // 保存当前绘图状态
        context.save();
        // 设置颜色和线宽
        context.strokeStyle = this.currentColor;
        context.fillStyle = this.currentColor;
        context.lineWidth = this.currentSize;
        context.lineCap = 'round';
        context.lineJoin = 'round';
        // 绘制箭头线段
        context.beginPath();
        context.moveTo(fromX, fromY);
        context.lineTo(lineEndX, lineEndY);
        context.stroke();
        // 绘制箭头头部（等边三角形）
        context.beginPath();
        context.moveTo(toX, toY);
        // 左侧箭头边终点
        const leftX = toX - headLength * Math.cos(angle - Math.PI / 6);
        const leftY = toY - headLength * Math.sin(angle - Math.PI / 6);
        // 右侧箭头边终点
        const rightX = toX - headLength * Math.cos(angle + Math.PI / 6);
        const rightY = toY - headLength * Math.sin(angle + Math.PI / 6);
        // 绘制三角形
        context.lineTo(leftX, leftY);
        context.lineTo(rightX, rightY);
        context.closePath();
        context.fill();
        // 恢复绘图状态
        context.restore();
    }

    // 开始绘制
    public startDrawing(event: MouseEvent): void {
        if (!EntryAPI.canvasWrapper.style.display || EntryAPI.canvasWrapper.style.display === 'none') return;

        this.isDrawing = true;
        const pos = this.getMousePos(event);
        this.lastX = pos.x;
        this.lastY = pos.y;
        this.startX = pos.x;
        this.startY = pos.y;

        // 对于自由绘制，开始一条新路径
        if (this.currentTool === 'draw') {
            EntryAPI.drawCtx.beginPath();
            EntryAPI.drawCtx.moveTo(this.lastX, this.lastY);
        }
    }

    // 绘制中
    public draw(event: MouseEvent): void {
        if (!this.isDrawing) return;

        const pos = this.getMousePos(event);

        if (this.currentTool === 'draw') {
            // 自由绘制 - 连续绘制路径
            EntryAPI.drawCtx.lineTo(pos.x, pos.y);
            EntryAPI.drawCtx.strokeStyle = this.currentColor;
            EntryAPI.drawCtx.lineWidth = this.currentSize;
            EntryAPI.drawCtx.lineCap = 'round';
            EntryAPI.drawCtx.lineJoin = 'round';
            EntryAPI.drawCtx.stroke();
        }
        else {
            EntryAPI.previewCtx.clearRect(0, 0, this.previewCanvas.width, this.previewCanvas.height);

            switch (this.currentTool) {
                case 'rect':
                    const width = pos.x - this.startX;
                    const height = pos.y - this.startY;

                    EntryAPI.previewCtx.strokeStyle = this.currentColor;
                    EntryAPI.previewCtx.lineWidth = this.currentSize;
                    EntryAPI.previewCtx.strokeRect(this.startX, this.startY, width, height);
                    break;

                case 'line':
                    EntryAPI.previewCtx.beginPath();
                    EntryAPI.previewCtx.moveTo(this.startX, this.startY);
                    EntryAPI.previewCtx.lineTo(pos.x, pos.y);
                    EntryAPI.previewCtx.strokeStyle = this.currentColor;
                    EntryAPI.previewCtx.lineWidth = this.currentSize;
                    EntryAPI.previewCtx.stroke();
                    break;

                case 'circle':
                    const radiusX = Math.abs(pos.x - this.startX) / 2;
                    const radiusY = Math.abs(pos.y - this.startY) / 2;
                    const centerX = this.startX + (pos.x - this.startX) / 2;
                    const centerY = this.startY + (pos.y - this.startY) / 2;

                    EntryAPI.previewCtx.beginPath();
                    EntryAPI.previewCtx.ellipse(centerX, centerY, radiusX, radiusY, 0, 0, 2 * Math.PI);
                    EntryAPI.previewCtx.strokeStyle = this.currentColor;
                    EntryAPI.previewCtx.lineWidth = this.currentSize;
                    EntryAPI.previewCtx.stroke();
                    break;

                case 'arrow':
                    this.drawArrow(this.startX, this.startY, pos.x, pos.y, EntryAPI.previewCtx);
                    break;

                default:
                    break;
            }
        }
    }

    // 结束绘制
    public stopDrawing(e: MouseEvent): boolean {
        if (!this.isDrawing) return false;
        const pos = this.getMousePos(e);
        let didDraw = false;
        if (this.currentTool === 'text') {
            const text = prompt('请输入要添加的文本:', '示例文本');
            if (text) {
                const fontSize = 20 + (this.currentSize * 4);
                EntryAPI.drawCtx.font = `bold ${fontSize}px Arial`;
                EntryAPI.drawCtx.fillStyle = this.currentColor;
                EntryAPI.drawCtx.fillText(text, pos.x, pos.y);
                didDraw = true;
            }
        }
        else if (this.currentTool === 'draw') {
            didDraw = true;
        }
        else {
            EntryAPI.drawCtx.drawImage(this.previewCanvas, 0, 0);
            EntryAPI.previewCtx.clearRect(0, 0, this.previewCanvas.width, this.previewCanvas.height);
            didDraw = true;
        }

        this.isDrawing = false;
        return didDraw;
    }

    // 调整画布大小
    public resizeCanvases(width: number, height: number): void {
        this.drawCanvas.width = width;
        this.drawCanvas.height = height;
        this.previewCanvas.width = width;
        this.previewCanvas.height = height;
    }
}