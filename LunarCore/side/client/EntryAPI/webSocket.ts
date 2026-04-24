import * as EntryAPI from './code';

// TypeScript类型定义
type MessageType = 'new_request' | 'connection_established' | 'error' | 'response_sent' | 'ai_response';

type WebSocketMessage = {
	type: MessageType;
	data?: any;
	request_id?: string;
};

type MessageContent = {
	type: 'text' | 'image_url';
	text?: string;
	image_url?: {
		url: string;
	};
};

type MessageItem = {
	role: string;
	content: string | MessageContent[];
};

type RequestData = {
	messages: MessageItem[];
	tools?: EntryAPI.ToolCall[];
};

/** 外部通讯消息处理器 */
class MessageHandler {
	/** 聊天消息索引，用于去重 */
	private static chatMessageIndex = new Set<string>();
	/** 处理新请求 */
	static async handleNewRequest(ws: WebSocket, requestData: RequestData, requestId: string): Promise<void> {
		try {
			/** 从请求数据中提取消息 */
			const messages = requestData.messages || [];
			/** 从请求数据中提取工具调用 */
			const tools = requestData.tools || [];
			// 检查消息是否为空
			if (!Array.isArray(messages) || messages.length === 0) return;
			/** 构建消息对象 */
			const messageObjects = await this.buildMessageObjects(messages);
			// 更新历史消息
			EntryAPI.OnlyData.historyMessage = messageObjects;
			// 更新工具调用
			EntryAPI.OnlyData.toolCall = tools;
			// 运行连续记忆模块
			await EntryAPI.controlContinuousMemory.run();
			// 处理AI响应
			await this.handleAIResponse(ws, requestId);
		}
		catch (error) {
			EntryAPI.showSystemMessage(`外部通讯请求 ${requestId} 失败: ${error}`, 'error');
		}
	};
	/** 构建消息对象 */
	private static async buildMessageObjects(messages: MessageItem[]): Promise<any[]> {
		/** 转换后的历史消息 */
		const historyMessage: EntryAPI.HistoryMessage[] = await this.batchConversion(messages);
		// 合并历史消息和转换后的消息
		return [...EntryAPI.OnlyData.historyMessage, ...historyMessage.filter(Boolean)];
	}
	/** 批量转换外部消息 */
	private static async batchConversion(messages: MessageItem[]): Promise<EntryAPI.HistoryMessage[]> {
		/** 转换后的历史消息 */
		const historyMessage: EntryAPI.HistoryMessage[] = [];
		// 遍历消息数组
		for (const message of messages) {
			try {
				// 检查消息是否为空
				if (!message.role || !message.content) continue;
				/** 构建消息索引 */
				const messageIndex = `${message.role}-${JSON.stringify(message.content)}`;
				// 检查消息是否重复
				if (this.chatMessageIndex.has(messageIndex)) continue;
				/** 提取消息内容 */
				let content = this.extractContent(message.content);
				/** 提取图片URL */
				let imageUrl = this.extractImageUrl(message.content);
				/** 确保role是有效的类型 */
				const role = typeof message.role === 'string' ? message.role : 'user';
				// 添加到历史消息索引
				this.chatMessageIndex.add(messageIndex);
				// 返回构建的消息对象
				historyMessage.push(await EntryAPI.createMessageObject(role as any, content, false, false, false, imageUrl));
			}
			catch (error) {
				EntryAPI.showSystemMessage(`构建消息对象时出错: ${error}`, 'error');
				// 返回默认消息对象
				historyMessage.push(await EntryAPI.createMessageObject('user', 'Error processing message', false, false, false));
			}
		}
		return historyMessage;
	}
	/** 提取消息内容 */
	private static extractContent(content: string | MessageContent[]): string {
		if (Array.isArray(content)) {
			return content
				.filter(item => item.type === 'text')
				.map(item => item.text)
				.filter(Boolean)
				.join('\n');
		}
		return content || '';
	}
	/** 提取图片URL */
	private static extractImageUrl(content: string | MessageContent[]): string {
		if (Array.isArray(content)) {
			const imageUrlItem = content.find(item => item.type === 'image_url');
			return imageUrlItem?.image_url?.url || '';
		}
		return '';
	}
	/** 处理AI响应 */
	private static async handleAIResponse(ws: WebSocket, requestId: string): Promise<void> {
		try {
			/** 调用多模态API获取回答 */
			const chatAnswer = await (await new EntryAPI.MultimodalRequest(await EntryAPI.createMessages(), false, false, true).response).json();
			// 发送回答
			if (chatAnswer && ws.readyState === WebSocket.OPEN) {
				/** 构建WebSocket消息对象 */
				const responseMessage: WebSocketMessage = { type: 'ai_response', data: chatAnswer, request_id: requestId };
				// 发送消息
				ws.send(JSON.stringify(responseMessage));
			}
			else EntryAPI.showSystemMessage(`外部通讯请求 ${requestId} 没有获取到回答或WebSocket连接未打开`, 'error');
		}
		catch (error) {
			EntryAPI.showSystemMessage(`外部通讯请求 ${requestId} 调用多模态API时出错: ${error}`, 'error');
		}
	}
}

/** 外部通讯管理器 */
class ExternalDialogueManager {
	/** WebSocket连接实例 */
	private dialogueExample: WebSocket | null = null;
	/** WebSocket服务器URL */
	private readonly serverUrl = `ws://localhost:${Number(window.location.port) + 5}/ws`;
	/** 重新连接间隔（毫秒） */
	private reconnectInterval = 3000;
	/** 重新连接定时器 */
	private tickExample: NodeJS.Timeout | null = null;
	/** 启动WebSocket连接 */
	start(): void {
		try {
			this.dialogueExample = new WebSocket(this.serverUrl);
			this.setupEventListeners();
		}
		catch (error) {
			EntryAPI.showSystemMessage(`创建WebSocket连接失败: ${error}`, 'error');
			this.scheduleReconnect();
		}
	}
	/** 设置事件监听器 */
	private setupEventListeners(): void {
		// 检查是否已存在连接
		if (!this.dialogueExample) return;
		// 清除已存在的重连定时器
		clearTimeout(this.tickExample);
		// 连接打开事件
		this.dialogueExample.onopen = () => EntryAPI.disabledReleaseButton(true);
		// 接收消息事件
		this.dialogueExample.onmessage = async (event) => await this.handleMessage(event);
		// 连接关闭事件
		this.dialogueExample.onclose = () => EntryAPI.disabledReleaseButton(false);
		// 连接错误事件
		this.dialogueExample.onerror = (error) => EntryAPI.showSystemMessage(`外部通讯WebSocket连接错误: ${error}`, 'error');
	}
	/** 处理WebSocket消息 */
	private async handleMessage(event: MessageEvent): Promise<void> {
		try {
			const message: WebSocketMessage = JSON.parse(event.data);
			switch (message.type) {
				case 'new_request':
					if (message.request_id && this.dialogueExample) {
						await MessageHandler.handleNewRequest(this.dialogueExample, message.data, message.request_id);
					}
					break;

				case 'connection_established':
					EntryAPI.showSystemMessage(message.data, 'success');
					break;

				case 'error':
					EntryAPI.showSystemMessage(message.data, 'error');
					break;

				case 'response_sent':
					EntryAPI.showSystemMessage(message.data, 'success');
					break;

				default:
					EntryAPI.showSystemMessage(`外部通讯收到未知类型的消息: ${message.type}`, 'error');
					break;
			}
		}
		catch (error) {
			EntryAPI.showSystemMessage(`外部通讯处理WebSocket消息时出错: ${error}`, 'error');
		}
	}
	/** 安排重连 */
	private scheduleReconnect(): void {
		this.tickExample = setTimeout(() => this.start(), this.reconnectInterval);
	}
	/** 获取当前WebSocket连接 */
	getConnection(): WebSocket | null {
		return this.dialogueExample;
	}
	/** 关闭WebSocket服务 */
	close(): void {
		if (!this.dialogueExample) return;
		this.dialogueExample.close();
		this.dialogueExample = null;
	}
}

/** 实例化外部通讯管理器 */
export const managerExchanges = new ExternalDialogueManager();