/* ============================================================
   tool-selector.js - 工具选择器
   ============================================================ */

export class ToolSelector {
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
