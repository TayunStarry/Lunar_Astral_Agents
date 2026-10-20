import { AuthHeaders } from './model';
import { ToolCall } from './tool';

/** 全局系统配置项 */
export interface Config {
	/** 核心智能体配置（月华 Agent）：用户显示名与模型路由 */
	agent: {
		/** 用户显示名（前端气泡名与发送前缀共用） */
		user_name?: string;
		/** 嵌入模型名称 */
		embedding_model?: string;
		/** 嵌入服务 API 地址 */
		embedding_url?: string;
		/** 嵌入服务 API 密钥 */
		embedding_key?: string;
		/** 多模态模型名称 */
		multimodal_model?: string;
		/** 多模态服务 API 地址 */
		multimodal_url?: string;
		/** 多模态服务 API 密钥 */
		multimodal_key?: string;
		/** 语音识别模型名称 */
		asr_model?: string;
		/** 静态图思维链形态：url=视觉读取端点（默认）/ base64=视觉读取管线直出的副本（云端模型） */
		image_input?: string;
		/** 视觉读取副本最大边长（像素） */
		vision_max_side?: number;
		/**
		 * 追加设定：逐条附加到「月华 · 对话者」系统提示词末尾的文本行。
		 *
		 * 每一项就是一行，按数组顺序拼接；未配置或空数组时不追加任何内容
		 * （也不会残留标题或空行）。适合在不改动内置提示词的前提下补充私人设定。
		 */
		append_settings?: string[];
	};
	/** 系统级设置：运行/安全、控制台日志、聊天显存守卫、前端显示统一收拢在 system 下 */
	system?: {
		/** 调试模式开关：开启时导出子智能体上下文日志 */
		developer?: boolean;
		/** 是否允许局域网访问引擎 HTTP/WS 服务（未配置默认仅绑定回环地址） */
		allow_lan?: boolean;
		/** 插件沙箱网络是否跳过 TLS 证书校验（未配置默认开启校验） */
		insecure_tls?: boolean;
		/** 是否把控制台输出镜像到 local_data/logs/console.log（默认关闭） */
		console_log?: boolean;
		/** 显存守卫开关 */
		chat_vram_guard?: boolean;
		/** 显存守卫阈值（MiB），可用显存低于该值时触发卸载 */
		chat_vram_guard_mib?: number;
		/** 显存守卫应答间隔：每完成 N 次应答检查一次 */
		chat_vram_guard_interval?: number;
		/** 角色渲染器：bedrock / live2d（由前端读取） */
		character_renderer?: string;
		/** Live2D 模型基址（空则用引擎层默认） */
		live2d_model?: string;
	};
	/** 多模态解读配置（视频/动态图送入模型的方式） */
	multimodal?: {
		/**
		 * 视频解读输入模式：
		 * - `file`（默认）写媒体目录后以 `file://` 引用交 llama-server 抽帧；
		 * - `frames` 本地抽帧为图片序列帧（base64）后以多个 image_url 提交，供不支持 file:// 的架构使用
		 */
		video_input?: string;
	};
	/** 绘图配置（画面比例与尺寸策略） */
	diffusion?: {
		/**
		 * 图生图/合照按参考图比例推导尺寸时的**总像素预算上限**（百万像素）。
		 * 缺省 1.6（≈1024×1536）：常规比例不受影响，超宽/超长画幅在此预算内尽量保持比例忠实。
		 * 调高更清晰但更吃显存与耗时；调低更省资源。
		 */
		pixel_budget_mp?: number;
		/**
		 * 图生图的提示词引导系数（CFG），覆盖绘制者给出的值。
		 * 缺省不设（按 1.0）：当前基线模型 Qwen-Image 2.1 是引导蒸馏模型，CFG 必须是 1.0；
		 * 换用非蒸馏模型（如 SDXL）后才需要 2~3。
		 */
		img2img_cfg?: number;
	};
}

/** 网络代理请求配置项 */
export interface ProxyFetchConfig {
	/** 请求 URL */
	url: string;
	/** 执行选项 */
	execute: {
		/** HTTP 方法, 默认为GET */
		method?: string;
		/** 是否允许跨域请求 */
		crossDomain?: boolean;
		/** 请求头 */
		headers?: Record<string, string> | AuthHeaders;
		/** 请求体 */
		body?: any;
	};
}

/** 聊天缓存接口 */
export interface ChatCache {
	/** 累积所有工具调用的数组 */
	toolCalls: ToolCall[];
	/** 当前正在累积的工具调用对象，可能为 null */
	currentToolCall: ToolCall | null;
	/** 当前工具调用在流中的索引，用于识别是否属于同一次调用 */
	currentToolCallIndex: number;
	/** 累积当前工具调用的参数字符串 */
	currentFunctionArgs: string;
	/** 累积当前工具调用的函数名 */
	currentFunctionName: string;
	/** 提取思考内容的字符串 */
	thinkingContent: string;
	/** 提取描述内容的字符串 */
	descriptionContent: string;
}

/** 文件列表项属性 */
export interface FileListItem {
	/** 文件或目录名称 */
	name: string;
	/** 文件大小（字节） */
	size: number;
	/** 是否为目录 */
	isDir: boolean;
	/** 最后修改时间，格式：YYYY-MM-DD HH:mm:ss */
	lastModified: string;
	/** 文件的完整路径 */
	path: string;
}