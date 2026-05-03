/* ============================================================
   history-manager.js - 历史管理功能
   ============================================================ */

export class HistoryManager {
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
