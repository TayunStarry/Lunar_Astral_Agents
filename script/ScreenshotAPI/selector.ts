import * as EntryAPI from '../EntryAPI/code';

// 工具选择器
export class ToolSelector {
	/**
	 * 工具切换回调：当用户点击不同绘图工具时触发
	 */
	public onToolChange: ((tool: EntryAPI.DrawingTool) => void) | null = null;
	/**
	 * 颜色切换回调：当用户点击不同颜色按钮时触发
	 */
	public onColorChange: ((color: EntryAPI.ColorHex) => void) | null = null;
	/**
	 * 线宽切换回调：当用户点击不同线宽按钮时触发
	 */
	public onSizeChange: ((size: EntryAPI.LineSize) => void) | null = null;
	/**
	 * 构造器：接收主绘图画布，用于后续动态修改光标样式
	 *
	 * @param {HTMLCanvasElement} drawCanvas 主绘图画布元素
	 */
	constructor(public drawCanvas: HTMLCanvasElement) { }
	/**
	 * 设置当前选中的绘图工具，并同步 UI 状态
	 * @param tool 要激活的绘图工具类型
	 */
	public setclickingTool(tool: EntryAPI.DrawingTool): void {
		/** 所有绘图工具按钮元素数组 */
		const tools = [EntryAPI.rectTool, EntryAPI.drawTool, EntryAPI.lineTool, EntryAPI.circleTool, EntryAPI.textTool, EntryAPI.arrowTool];
		// 确保所有工具按钮都没有激活样式
		tools.forEach(toolEl => toolEl.classList.remove('clicking'));
		// 根据工具类型添加激活样式并设置对应光标
		switch (tool) {
			case 'rect':
				EntryAPI.rectTool.classList.add('clicking');
				this.drawCanvas.style.cursor = 'crosshair';
				break;
			case 'draw':
				EntryAPI.drawTool.classList.add('clicking');
				this.drawCanvas.style.cursor = 'crosshair';
				break;
			case 'line':
				EntryAPI.lineTool.classList.add('clicking');
				this.drawCanvas.style.cursor = 'crosshair';
				break;
			case 'circle':
				EntryAPI.circleTool.classList.add('clicking');
				this.drawCanvas.style.cursor = 'crosshair';
				break;
			case 'text':
				EntryAPI.textTool.classList.add('clicking');
				this.drawCanvas.style.cursor = 'text';
				break;
			case 'arrow':
				EntryAPI.arrowTool.classList.add('clicking');
				this.drawCanvas.style.cursor = 'crosshair';
				break;
		}
		// 触发外部回调
		if (this.onToolChange) this.onToolChange(tool);
	}

	/**
	 * 设置当前选中的颜色，并同步 UI 状态
	 *
	 * @param {EntryAPI.ColorHex} color 要激活的颜色值（十六进制）
	 *
	 * @param {Event} event 点击事件，用于高亮当前按钮
	 */
	public setclickingColor(color: EntryAPI.ColorHex, event?: Event): void {
		// 清除所有颜色按钮的激活样式
		EntryAPI.colorOptions.forEach(option => option.classList.remove('clicking'));
		// 高亮被点击的按钮
		if (event && event.target instanceof HTMLElement) {
			event.target.classList.add('clicking');
		}
		// 触发外部回调
		if (this.onColorChange) this.onColorChange(color);
	}
	/**
	 * 设置当前选中的线宽，并同步 UI 状态
	 *
	 * @param {EntryAPI.LineSize} size 要激活的线宽值（字符串形式）
	 *
	 * @param {Event} event 点击事件，用于高亮当前按钮
	 */
	public setclickingSize(size: EntryAPI.LineSize, event?: Event): void {
		// 清除所有线宽按钮的激活样式
		EntryAPI.sizeOptions.forEach(option => option.classList.remove('clicking'));
		// 高亮被点击的按钮
		if (event && event.target instanceof HTMLElement) {
			event.target.classList.add('clicking');
		}
		// 触发外部回调
		if (this.onSizeChange) this.onSizeChange(size);
	}
	/**
	 * 初始化所有工具、颜色、线宽按钮的点击事件监听
	 */
	public initEventListeners(): void {
		// 工具按钮
		EntryAPI.rectTool.addEventListener('click', () => this.setclickingTool('rect'));
		EntryAPI.drawTool.addEventListener('click', () => this.setclickingTool('draw'));
		EntryAPI.lineTool.addEventListener('click', () => this.setclickingTool('line'));
		EntryAPI.circleTool.addEventListener('click', () => this.setclickingTool('circle'));
		EntryAPI.textTool.addEventListener('click', () => this.setclickingTool('text'));
		EntryAPI.arrowTool.addEventListener('click', () => this.setclickingTool('arrow'));
		// 颜色按钮
		EntryAPI.colorOptions.forEach(option => {
			option.addEventListener('click', (e: Event) => {
				const target = e.target as HTMLButtonElement;
				this.setclickingColor(target.dataset.color || '#e74c3c', e);
			});
		});
		// 线宽按钮
		EntryAPI.sizeOptions.forEach(option => {
			option.addEventListener('click', (e: Event) => {
				const target = e.target as HTMLButtonElement;
				this.setclickingSize(target.dataset.size || '10', e);
			});
		});
	}
	/**
	 * 初始化默认状态：默认选中画笔工具、线宽 10、红色
	 */
	public initDefaultState(): void {
		this.setclickingTool('draw');
		this.setclickingSize('10');
		const defaultColorOption = Array.from(EntryAPI.colorOptions).find(option =>
			option.dataset.color === '#e74c3c'
		);
		if (defaultColorOption) {
			this.setclickingColor('#e74c3c', { target: defaultColorOption } as unknown as Event);
		}
	}
}