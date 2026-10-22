/** 一张已完成的截图（截图者内部产物） */
export interface ScreenshotShot {
	/** 本次截图的任务内说明（模型的 note） */
	note: string;
	/** 实际采用的截图模式 */
	mode: string;
	/** 实际捕获的窗口标题（屏幕/显示器截图时为空） */
	windowTitle: string;
	/** 缩放后宽度 */
	width: number;
	/** 缩放后高度 */
	height: number;
	/** 图片 data URI（供对话者与前端展示） */
	base64: string;
	/** 内容仓库可访问地址（落盘失败时为空） */
	url: string;
}
