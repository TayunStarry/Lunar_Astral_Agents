import { Config } from './config';
import { PostMessage } from './model';
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
	/** 消息缓冲池（对话者淘汰的历史消息，待信息记忆流程写入记忆库） */
	public static unreadRecords: PostMessage[] = [];
	/** 未读上下文（宿主统一时序队列：文本与视频/音频URL内容项按到达顺序混排） */
	public static unreadContext: PostMessage[] = [];
	/** 是否正在思考中 */
	public static reasoningInProgress: boolean = false;
	/** 最终响应 */
	public static finalResponse: string = "";
	/**
	 * 本轮应答的失败原因（空串表示这一轮没有失败）
	 *
	 * 为什么单独记在全局：`generateDialogue` 内部把异常收敛成"空应答"，调用方只看到
	 * finalResponse 为空，分不清"模型说没话说"与"模型根本没接通"。有了这一格，
	 * 外层循环才能把真正的失败上报给宿主（触发自动重载引擎），而不是当成空应答忽略。
	 */
	public static lastFailure: string = "";
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
	/** 获取 语音识别模型名称（从 agent 配置读取） */
	public static get AsrName(): string {
		return GlobalConfig.customConfig?.agent?.asr_model || "system-asr";
	};
	/** 获取 调试模式开关 */
	public static get debugMode(): boolean {
		return GlobalConfig.customConfig?.server?.developer ?? false;
	};
	/** 获取 视频解读输入模式（agent.video_input，兼容旧 multimodal 节，默认 file） */
	public static get VideoInputMode(): string {
		return GlobalConfig.customConfig?.multimodal?.video_input === 'frames' ? 'frames' : 'file';
	};
	/** 获取 静态图思维链形态（agent.image_input，默认 url 视觉读取端点） */
	public static get ImageInputMode(): string {
		return GlobalConfig.customConfig?.agent?.image_input === 'base64' ? 'base64' : 'url';
	};
	/** 获取 视觉读取副本最大边长（agent.vision_max_side，默认 1080） */
	public static get VisionMaxSide(): number {
		const side = Number(GlobalConfig.customConfig?.agent?.vision_max_side);
		return Number.isFinite(side) && side > 0 ? side : 1080;
	};
};