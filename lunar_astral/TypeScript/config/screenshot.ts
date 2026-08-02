/** 截图请求参数接口 */
export interface ScreenshotParams {
	/** 显示器索引（-1 表示所有显示器，0 表示主显示器） */
	display_index?: number;
	/** 截图区域，格式为 "x,y,width,height" */
	region?: string;
	/** 缩放参数，如 "0.5" 或 "800,600" */
	scale?: string;
	/** 图片格式，"png" 或 "jpg" */
	format?: string;
	/** JPEG 质量 1-100 */
	quality?: number;
}

/** 显示器信息接口 */
export interface DisplayInfo {
	/** 显示器索引 */
	index: number;
	/** 显示器 X 坐标 */
	x: number;
	/** 显示器 Y 坐标 */
	y: number;
	/** 显示器宽度 */
	width: number;
	/** 显示器高度 */
	height: number;
}
