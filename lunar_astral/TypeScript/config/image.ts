/** 视频媒体片段（llama-server 媒体目录内的分段） */
export interface MediaSegment {
	/** 媒体目录内的文件名（通过 file:// 引用） */
	file: string;
	/** 片段起始时间（秒） */
	start: number;
	/** 片段结束时间（秒） */
	end: number;
}

/** 序列帧模式的单帧（图片序列帧解读模式，无需服务端解码视频） */
export interface MediaFrame {
	/** 帧图片的完整 data URI（data:image/jpeg;base64,...），可直接作为 image_url 提交 */
	image: string;
	/** 帧在源视频中的时间点（秒） */
	time: number;
}

/** 序列帧模式的时间分组，与 MediaSegment 同构 */
export interface MediaFrameGroup {
	/** 分组起始时间（秒） */
	start: number;
	/** 分组结束时间（秒） */
	end: number;
	/** 该时间分组内的采样帧（按时间顺序） */
	frames: MediaFrame[];
}

/** 缩放图片结果接口（单帧） */
export interface ResizeImageResult {
	/** 缩放后的图片数据 */
	image: Uint8Array;
	/** 缩放后的图片base64编码 */
	base64: string;
	/** 图片格式 */
	format: string;
	/** 缩放后的图片宽度 */
	width: number;
	/** 缩放后的图片高度 */
	height: number;
}

/** 缩放图片返回类型：静态图返回单元素数组，动态图(GIF/APNG/WebP帧数>2)返回多帧数组 */
export type ResizeImageResults = ResizeImageResult[];

/** 图片生成参数接口 */
export interface GenerateImageParams {
	/** 提示词 */
	prompt: string;
	/** 负面提示词 */
	negativePrompt?: string;
	/** 批处理数量 */
	batchSize?: number;
	/** 图片宽度 */
	width?: number;
	/** 图片高度 */
	height?: number;
	/** 生成步数 */
	steps?: number;
	/** 图生图强度 */
	strength?: number;
	/** 提示词引导系数 */
	cfgScale?: number;
	/** 随机数种子 */
	seed?: number;
	/** 初始图片路径 */
	initImg?: string;
}

/** 图片生成结果接口 */
export interface GenerateImageResult {
	/** 生成的图片路径 */
	path: string;
	/** 生成的图片base64编码 */
	base64: string;
	/** 图片宽度 */
	width: number;
	/** 图片高度 */
	height: number;
	/** 随机数种子 */
	seed: number;
}

/** 扩散生成工具 - 参数接口 */
export interface DiffusionGenerationParams {
	/** 提示词 */
	prompt: string;
	/** 负面提示词 */
	negative_prompt?: string;
	/** 提示词引导系数 */
	cfg_scale?: number;
}

/** 自画像工具 - 参数接口（全尺寸 / Q版共用） */
export interface SelfPortraitParams {
	/** 表情描述 */
	expression: string;
	/** 姿势描述 */
	posture: string;
	/** 服装描述，默认为角色默认服装 */
	outfit?: string;
	/** 环境描述 */
	environment: string;
	/** 负面提示词 */
	negative_prompt?: string;
	/** 提示词引导系数 */
	cfg_scale?: number;
}

/** 图生图工具 - 参数接口 */
export interface ImageToImageParams {
	/** 提示词 */
	prompt: string;
	/** 负面提示词 */
	negative_prompt?: string;
	/** 图生图强度（0~1，越大越偏离参考图），默认随机取值 */
	strength?: number;
	/** 提示词引导系数 */
	cfg_scale?: number;
}

/** 合照工具 - 参数接口 */
export interface GroupPhotoParams {
	/** 合照对象描述（外貌、服装、身份等） */
	companion: string;
	/** 月华的表情描述 */
	expression: string;
	/** 月华的姿势描述 */
	posture: string;
	/** 月华的服装描述，默认为角色默认服装 */
	outfit?: string;
	/** 环境描述 */
	environment: string;
	/** 负面提示词 */
	negative_prompt?: string;
	/** 提示词引导系数 */
	cfg_scale?: number;
}
