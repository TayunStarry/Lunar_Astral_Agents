import { AuthHeaders, ToolCall } from '../index';

/** 全局系统配置项 */
export interface Config {
	/** 核心智能体配置（月华 Agent） */
	agent: {
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
	};
	server: {
		/** 用户名 */
		user_name?: string;
		/** 调试模式开关：开启时导出子智能体上下文日志 */
		developer?: boolean;
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