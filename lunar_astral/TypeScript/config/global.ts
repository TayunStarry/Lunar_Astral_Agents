import { Config, PostMessage, ToolCall } from '../index';
/** 全局配置 */
export class GlobalConfig {
	/** 自定义配置项 */
	public static customConfig: Config = { agent: {}, server: {} };
	/** 未读记录列表 */
	public static unreadRecords: PostMessage[] = [];
	/** LTP协议工具包-函数映射表，工具函数返回 string[]：下标0=文本内容结果，下标1=图片base64数据（无则为空字符串） */
	public static LTPfunction = new Map<string, (args?: Record<string, any> | string) => Promise<string[]>>();
	/** LTP协议工具包-函数定义 */
	public static LTPdefinition: ToolCall[] = [];
	/** 多模态服务URL（从 agent 配置读取多模态服务地址，未配置时回退到本地服务） */
	public static get MultimodalUrl(): string {
		return GlobalConfig.customConfig?.agent?.multimodal_url || url()[0] + '/v1';
	};
	/** 获取 多模态服务 API 密钥（从 agent 配置读取） */
	public static get MultimodalKey(): string {
		return GlobalConfig.customConfig?.agent?.multimodal_key || 'key-520-1314-2000-02-18';
	};
	/** 获取 多模态模型名称（从 agent 配置读取） */
	public static get MultimodalName(): string {
		return GlobalConfig.customConfig?.agent?.multimodal_model || "system-multimodal";
	};
	/** 获取 嵌入模型名称（从 agent 配置读取） */
	public static get EmbeddingName(): string {
		return GlobalConfig.customConfig?.agent?.embedding_model || "system-embedding";
	};
	/** 获取 用户名 */
	public static get userName(): string {
		return GlobalConfig.customConfig?.server?.user_name || "阁下";
	};
	/** 获取 调试模式开关 */
	public static get debugMode(): boolean {
		return GlobalConfig.customConfig?.server?.debug_mode ?? false;
	};
};