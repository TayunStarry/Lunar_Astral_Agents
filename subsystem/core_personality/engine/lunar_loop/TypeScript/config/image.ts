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
	/** 截图实际采用的模式（仅截图结果携带） */
	mode?: string;
	/** 截图实际捕获的窗口标题（仅截图结果携带） */
	window_title?: string;
	/** 截图实际捕获的窗口句柄（仅截图结果携带） */
	window_handle?: number;
	/** 截图原始宽度（缩放前，仅截图结果携带） */
	source_width?: number;
	/** 截图原始高度（缩放前，仅截图结果携带） */
	source_height?: number;
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
	/** 图片宽度（0/未指定时由扩散模块按配置默认值归一化） */
	width?: number;
	/** 图片高度（0/未指定时由扩散模块按配置默认值归一化） */
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
	/** 生成完成后追加 RealESRGAN 超分辨率重建（4x，耗时增加） */
	superResolution?: boolean;
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
	/** 画面比例预设（如 '1:1'、'3:4'、'16:9'），缺省为 1:1 */
	ratio?: string;
	/** 迭代步数（30~60，省略则用配置 sd_steps）：越大越精细、耗时线性增加 */
	steps?: number;
	/** 提示词引导系数 */
	cfg_scale?: number;
	/** 生成完成后追加超分辨率重建 */
	super_resolution?: boolean;
}

/**
 * 立绘工具 - 参数接口（月华 / 琉璃 / 通用角色，全尺寸与Q版共用同一入口）
 *
 * 为什么把「谁 + 是否Q版」做成参数而不是四个并列工具：四份工具定义只有角色与比例
 * 不同，schema 重复约 200 行，既占上下文又容易让模型挑错入口。合成一个工具后，
 * 角色是四值枚举、Q版是布尔，新增角色只需加一份底板提示词。
 *
 * 为什么性别折进角色枚举（`通用-女` / `通用-男`）而不是单独的 `gender` 参数：
 * 性别只对通用角色有意义，独立参数就成了「仅当 character=通用 时生效」的条件参数；
 * 函数调用 schema 表达不了这种依赖，只能靠描述提醒，漏填/乱填都无从校验。
 * 折叠后没有默认值可错、没有条件规则要记。
 */
export interface PortraitParams {
	/** 角色：月华 / 琉璃 / 通用-女 / 通用-男（后两者需 appearance 描述外观） */
	character: string;
	/** 是否 Q 版（二头身）；false 或不填为全尺寸立绘 */
	chibi?: boolean;
	/** 通用角色的外观：性别、年龄感、发型发色、瞳色、服装、配饰、身份 */
	appearance?: string;
	/** 表情描述 */
	expression: string;
	/** 姿势描述 */
	posture: string;
	/** 服装描述，默认为角色默认服装 */
	outfit?: string;
	/** 环境描述 */
	environment: string;
	/** 画面比例预设（如 '1:1'、'2:3'、'9:16'）；缺省：全尺寸 2:3、Q版 1:1 */
	ratio?: string;
	/** 迭代步数（30~60，省略则用配置 sd_steps） */
	steps?: number;
	/** 负面提示词 */
	negative_prompt?: string;
	/** 提示词引导系数 */
	cfg_scale?: number;
	/** 生成完成后追加超分辨率重建 */
	super_resolution?: boolean;
}

/** 图生图工具 - 参数接口 */
export interface ImageToImageParams {
	/** 提示词 */
	prompt: string;
	/** 负面提示词 */
	negative_prompt?: string;
	/** 图生图强度（0~1，越大越偏离参考图）：由绘制者按创作意图指定 */
	strength: number;
	/** 迭代步数（30~60，省略则用配置 sd_steps） */
	steps?: number;
	/** 提示词引导系数 */
	cfg_scale?: number;
	/** 生成完成后追加超分辨率重建 */
	super_resolution?: boolean;
}

/** 合照工具 - 参数接口 */
export interface GroupPhotoParams {
	/** 参与者列表：内置/自定义角色直接写名字（继承外观），临时角色写完整外观描述 */
	roles: string[];
	/** 是否Q版：true=所有参与者使用二头身比例（省略或 false=全尺寸合照） */
	chibi?: boolean;
	/** 统一的角色规划：逐人写清表情、动作与服饰（省略则使用默认） */
	planning?: string;
	/** 环境描述（省略时随机使用预设背景） */
	environment?: string;
	/** 画面比例预设（未选定参考图时生效；已选定参考图时尺寸随参考图），缺省 1:1 */
	ratio?: string;
	/** 图生图强度（0~1，已选定参考图时生效）：由绘制者按创作意图指定 */
	strength?: number;
	/** 迭代步数（30~60，省略则用配置 sd_steps） */
	steps?: number;
	/** 负面提示词 */
	negative_prompt?: string;
	/** 提示词引导系数 */
	cfg_scale?: number;
	/** 生成完成后追加超分辨率重建 */
	super_resolution?: boolean;
}

/** 绘图模型方案切换 - 参数接口 */
export interface SchemeSwitchParams {
	/** 目标方案：turbo=速度优先 / quality=质量优先 */
	scheme: string;
}

/** 自定义角色创建/更新 - 参数接口 */
export interface CreateCharacterParams {
	/** 角色名（唯一键，后续绘制时引用） */
	name: string;
	/** 性别：女 / 男 */
	gender: string;
	/** 基础外观：年龄感、发型发色、瞳色、面部特征、配饰、体型 */
	appearance: string;
	/** 默认全尺寸服装（可空） */
	outfit_full?: string;
	/** 默认Q版服装（可空，空则复用全尺寸） */
	outfit_chibi?: string;
}

/** 输出尺寸计划（画面比例 → 实际像素尺寸） */
export interface DimensionPlan {
	/** 目标宽度（32 的倍数） */
	width: number;
	/** 目标高度（32 的倍数） */
	height: number;
	/** 该尺寸的来源说明（日志用） */
	origin: string;
}

/** 图片句柄工具 - 参数接口（选定参考图 / 超分共用） */
export interface ImageTagParams {
	/** 图片句柄（统一句柄工法，如 @I.003） */
	image_tag?: string;
}
