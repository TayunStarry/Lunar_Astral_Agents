// 历史记录管理类
export class HistoryManager {
	private actionHistory: ImageData[] = [];

	constructor(public drawCanvas: HTMLCanvasElement, public drawCtx: CanvasRenderingContext2D, public undoDrawButton: HTMLButtonElement) {
	}

	// 保存当前操作到历史
	saveState(): void {
		// 保存当前绘图状态到历史记录
		this.actionHistory.push(
			this.drawCtx.getImageData(0, 0, this.drawCanvas.width, this.drawCanvas.height)
		);
		// 更新撤销按钮状态
		this.updateUndoButtonState();
	}

	// 撤销上一步操作
	undo(): boolean {
		if (this.actionHistory.length > 0) {
			// 恢复到上一个状态
			this.drawCtx.putImageData(this.actionHistory.pop()!, 0, 0);
			// 更新撤销按钮状态
			this.updateUndoButtonState();
			return true;
		}
		return false;
	}

	// 更新撤销按钮状态
	updateUndoButtonState(): void {
		this.undoDrawButton.disabled = this.actionHistory.length === 0;
	}

	// 清空历史记录
	clear(): void {
		this.actionHistory.length = 0;
		this.updateUndoButtonState();
	}

	// 获取历史记录长度
	getLength(): number {
		return this.actionHistory.length;
	}
}