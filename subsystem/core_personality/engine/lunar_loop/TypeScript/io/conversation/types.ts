// ===== 对话缓存数据结构（与前端 messages.js 记录结构一致） =====

/** 缓存附件 */
export interface CachedAttachment {
	type: string;
	src: string;
	label: string;
}

/** 缓存消息 */
export interface CachedMessage {
	id: string;
	role: 'user' | 'assistant';
	categories: string[];
	content: string;
	/** 用户消息的发送者名字（私聊/前端用户名），载入上下文时还原 `[用户: 名字]:` 前缀 */
	sender?: string;
	attachments?: CachedAttachment[];
	imageSrc?: string;
	imageLabel?: string;
	audio?: string;
	abcNotation?: string;
	actionType?: string;
	fileRef?: string;
	fileLabel?: string;
	timestamp: number;
}
