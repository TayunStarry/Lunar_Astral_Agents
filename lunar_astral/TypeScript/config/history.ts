/** 历史消息（单条） - 用于对话历史记录 */
export interface HistoryMessage {
	/** 消息角色 */
	role: string;
	/** 消息正文内容 */
	content: string;
	/** 是否为系统提示消息 */
	isPrompt: boolean;
	/** 是否在 UI 界面中跳过渲染 */
	noRender: boolean;
	/** 附带图片的 URL 地址，无则为 null */
	imageUrl: string | null;
	/** 消息是否可被用户删除 */
	deletable: boolean | null;
	/** 消息的唯一标识符 */
	uuid: string;
	/** 消息内容的嵌入向量，用于语义检索 */
	embedVector: number[];
}

/** 知识库消息（单条） - 用于知识库检索 */
export interface KnowledgeMessage {
	/** 消息角色 */
	role: string;
	/** 消息正文内容 */
	content: string;
	/** 附带图片的 URL 地址，无则为 null */
	imageUrl: string | null;
	/** 消息的唯一标识符 */
	uuid: string;
}

/** 混合消息类型 - 可用于统一处理历史消息和知识库消息 */
export type MixedMessage = KnowledgeMessage | HistoryMessage;

/** 加权历史消息 - 用于带有权重的历史消息检索 */
export interface WeightedHistoryMessage {
	/** 历史消息对象 */
	message: HistoryMessage;
	/** 消息的权重值，影响检索优先级 */
	weight: number;
}

/** 历史会话导出文档结构 */
export interface HistoryDocument {
	/** 文档元信息 */
	meta: {
		/** 导出时间，格式：YYYY.MM.DD-HH:mm:ss */
		exportedAt: string;
		/** 文档版本号 */
		version: string;
	};
	/** 历史消息数组 */
	history: HistoryMessage[];
}
