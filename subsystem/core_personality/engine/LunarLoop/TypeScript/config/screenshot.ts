/** 截图模式 */
export type ScreenshotMode = 'auto' | 'window' | 'fullscreen' | 'display' | 'region';

/** 截图请求参数接口（对齐 Go 层 CaptureRequest） */
export interface ScreenshotParams {
	/** 截图模式，默认 auto（焦点窗口优先，失败降级全屏） */
	mode?: ScreenshotMode;
	/** 显示器索引（mode=display 时生效，-1 表示全部） */
	display_index?: number;
	/** 窗口相对 X 偏移（mode=auto/window，配合 width/height 使用） */
	offset_x?: number;
	/** 窗口相对 Y 偏移（mode=auto/window，配合 width/height 使用） */
	offset_y?: number;
	/** 窗口相对区域宽度（>0 且 height>0 时启用精准区域覆盖） */
	width?: number;
	/** 窗口相对区域高度 */
	height?: number;
	/** 绝对屏幕区域 X（mode=region） */
	region_x?: number;
	/** 绝对屏幕区域 Y（mode=region） */
	region_y?: number;
	/** 绝对屏幕区域宽度（mode=region） */
	region_w?: number;
	/** 绝对屏幕区域高度（mode=region） */
	region_h?: number;
	/** 图片格式，"png" 或 "jpg" */
	format?: string;
	/** JPEG 质量 1-100 */
	quality?: number;
}
