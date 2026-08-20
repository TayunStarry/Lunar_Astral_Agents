import { Config, PostMessage, ToolCall } from '../index';
/** 全局配置 */
export class GlobalConfig {
	/** 自定义配置项 */
	public static customConfig: Config = { agent: {}, server: {} };
	/** 支持的图片文件扩展名 */
	public static readonly imageFormatsExtensions: string[] = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.webp'];
	/** 支持的视频文件扩展名 */
	public static readonly videoFormatsExtensions: string[] = ['.mp4', '.avi', '.mov', '.wmv', '.flv', '.mkv'];
	/** LTP协议工具包-函数映射表，工具函数返回 string[]：下标0=文本内容结果，下标1=图片base64数据（无则为空字符串） */
	public static LTPfunction = new Map<string, (args?: Record<string, any> | string) => Promise<string[]>>();
	/** LTP协议工具包-函数定义 */
	public static LTPdefinition: ToolCall[] = [];
	/** 消息缓冲池（对话者淘汰的历史消息，待信息记忆流程写入记忆库） */
	public static unreadRecords: PostMessage[] = [];
	/** 未读上下文 */
	public static unreadContext: PostMessage[] = [];
	/** 未读视频URL */
	public static unreadVideoUrl: string[] = [];
	/** 发言权重 */
	public static speakWeight: number = 1;
	/** 沉默计数（连续不允许发言的循环次数） */
	public static silenceCount: number = 0;
	/** 是否正在思考中 */
	public static reasoningInProgress: boolean = false;
	/** 最终响应 */
	public static finalResponse: string = "";
	/** 记忆库是否已初始化 */
	public static memoryReady: boolean = false;
	/** 当前的真实地址位置 */
	public static currentAddress: string[] = [];
	/** 多模态服务URL */
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
		return GlobalConfig.customConfig?.server?.developer ?? false;
	};
};