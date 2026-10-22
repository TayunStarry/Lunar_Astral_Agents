/** 截图模式 */
export type ScreenshotMode = 'auto' | 'window' | 'fullscreen' | 'display' | 'region';

/** 截图请求参数接口（对齐 Go 层 CaptureRequest） */
export interface ScreenshotParams {
	/** 截图模式，默认 auto（目标窗口优先，失败降级全屏） */
	mode?: ScreenshotMode;
	/** 显示器索引（mode=display 时生效，-1 表示全部显示器拼接） */
	display_index?: number;
	/** 目标窗口句柄（mode=auto/window，优先于 window_title） */
	window_handle?: number;
	/** 目标窗口标题关键字（mode=auto/window，包含匹配；与 window_handle 都为空时截焦点窗口） */
	window_title?: string;
	/** 是否只截客户区（去掉标题栏与边框），默认 false */
	client_area?: boolean;
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

/** 可截图的顶层窗口信息 */
export interface CaptureWindowInfo {
	/** 窗口句柄 */
	handle: number;
	/** 窗口标题 */
	title: string;
	/** 窗口类名 */
	class: string;
	/** 进程 ID */
	pid: number;
	/** 进程可执行文件名 */
	process: string;
	/** 窗口矩形左上角 X（屏幕坐标） */
	x: number;
	/** 窗口矩形左上角 Y（屏幕坐标） */
	y: number;
	/** 窗口矩形宽度 */
	width: number;
	/** 窗口矩形高度 */
	height: number;
	/** 客户区宽度 */
	client_w: number;
	/** 客户区高度 */
	client_h: number;
	/** 是否最小化（最小化窗口无法截图） */
	minimized: boolean;
	/** 是否当前焦点窗口 */
	foreground: boolean;
}
