import { OnlyData, PostMessage, ModelProtocol, AuthHeaders, modelResponse, PostMessageRole, ToolCall } from '../index';

/** 当前的真实地址位置 */
let currentAddress: string[] = [];

/** 基础配置 */
export class BaseConfig {
	/** 是否启用流式响应 */
	protected stream: boolean = false;
	/** 是否启用工具调用 */
	protected enableTools: boolean = true;
	/** 消息列表 */
	public messages: PostMessage[] = [];
	/** RAG消息列表 */
	protected ragMessages: PostMessage[] = [];
	/** 运行时消息列表 */
	protected runtimeMessages: PostMessage[] = [];
	/** 系统提示 */
	protected systemPrompt: string = "你的名字叫做月华, 是一个女孩子";
	/** 记忆库是否已初始化 */
	protected static memoryReady: boolean = false;
	/** 私有化构造函数，防止外部实例化 */
	protected constructor() { }
	/** 初始化记忆库 */
	protected static initMemory(): void {
		if (BaseConfig.memoryReady) return;
		const [_, err] = memoryInit(OnlyData.systemUrl, OnlyData.SystemKey, OnlyData.EmbeddingName, 'lunar_messages');
		if (err) console.error('记忆库初始化失败:', err);
		else BaseConfig.memoryReady = true;
	}
}

/** 提示词处理器 */
class PromptProcessor extends BaseConfig {
	/** 生成提示词 */
	protected promptCompletion(prompt: string): string {
		/** 当前地址文本 */
		let addressText = "";
		// 若当前地址为空，查询真实地址
		if (currentAddress.length === 0) {
			/** 查询真实地址 */
			const addressResult = address();
			// 设置当前地址
			currentAddress = addressResult[0];
			addressText = currentAddress.join(' ');
		}
		// 否则使用缓存地址
		else addressText = currentAddress.join(' ');
		// 返回替换后的系统提示词
		return prompt
			// 转换用户名称
			.replace(/{name}/g, OnlyData.userName)
			// 转换当前地址
			.replace(/{current-address}/g, addressText);
	}
	/** 从消息中提取文本 */
	protected extractTextFromMessages(messages: PostMessage[]): string[] {
		return messages.map(message => {
			// 处理纯文本消息和工具响应消息
			if (typeof message.content === 'string') {
				return message.content;
			}
			// 处理多模态消息和连续多模态消息
			else if (Array.isArray(message.content)) {
				// 提取所有文本内容并拼接
				const textContents = message.content
					.filter(item => item.type === 'text')
					.map(item => item.text);
				return textContents.join(' ');
			}
			// 默认为空字符串
			return '';
		}).filter(text => text.trim() !== ''); // 过滤空字符串
	}
}

/** 配置修改器 */
class ConfigModifier extends PromptProcessor {
	/** 设置流式响应 */
	public setStream(stream: boolean = false): this {
		this.stream = stream;
		return this;
	}
	/** 设置工具调用 */
	public setEnableTools(enable: boolean = true): this {
		this.enableTools = enable;
		return this;
	}
	/** 写入上下文（自动剥离 reasoning_content，避免回传触发模型无限推理） */
	public writeContext(context: PostMessage): this {
		const cleaned = this.stripReasoningContent(context);
		if (this.messages.length >= 40) {
			const discarded = this.messages.slice(0, this.messages.length - 39);
			this.messages = this.messages.slice(-39).concat(cleaned);
			OnlyData.unreadRecords.push(...discarded);
		}
		else this.messages.push(cleaned);
		return this;
	}
	/** 剥离消息中的 reasoning_content 字段，防止回传给模型触发无限推理 */
		protected stripReasoningContent(message: PostMessage): PostMessage {
		if ('reasoning_content' in message) {
			const { reasoning_content, ...rest } = message as any;
			return rest as PostMessage;
		}
		return message;
	}
	/** 覆写上下文 */
	public coverContext(context: PostMessage[] | PostMessage): this {
		this.messages = Array.isArray(context) ? context : [context];
		return this;
	}
}

/** 模型构建器 */
export class ModelBuilder extends ConfigModifier {
	/** 运行模型，可输入额外的上下文补充 */
	public run(appendContext: PostMessage[], toolCall: ToolCall[]): modelResponse {
		/** 模型请求体消息列表（拼接所有来源） */
		const rawMessages: PostMessage[] = [
			// 系统提示词
			{ role: 'system', content: this.systemPrompt },
			// 用户上下文占位符
			{ role: 'user', content: '[上下文]' },
			// 追加的上下文(rag消息)
			...appendContext,
			// 早期历史消息
			...this.messages.slice(0, -1),
			// 运行时消息
			...this.runtimeMessages,
			// 最新消息
			...this.messages.slice(-1)
		];
		/** 构建发给推理模型的请求体 */
		const requestBody = {
			model: OnlyData.MultimodalName,
			messages: rawMessages,
			stream: this.stream,
			tools: toolCall,
			tool_choice: 'auto',
		};
		// 如果禁用工具调用或没有工具调用，删除 tool_choice 和 tools 字段
		if (!this.enableTools || toolCall.length === 0) {
			delete requestBody.tool_choice;
			delete requestBody.tools;
		};
		/** 构建请求头 */
		const headers: AuthHeaders = {
			Authorization: `Bearer ${encodeURIComponent(OnlyData.SystemKey)}`,
			"Content-Type": "application/json",
		};
		/** 构建模型请求 */
		const modelRequest: ModelProtocol = {
			method: "POST",
			crossDomain: true,
			headers,
			body: JSON.stringify(requestBody)
		};
		/** 定义API端点 */
		const endpoint = "/chat/completions";
		/** 直接调用Go函数处理请求 */
		const [result, error] = syncFetch({ url: OnlyData.systemUrl + endpoint, execute: modelRequest });
		// 抛出请求级错误
		if (error) throw error;
		// 检查云端错误响应（网关返回 {error: {...}} 而非正常 choices）
		if (result?.body?.error) {
			const errMsg = typeof result.body.error === 'string'
				? result.body.error
				: result.body.error.message || JSON.stringify(result.body.error);
			throw new Error(`模型服务错误 [${result.status}]: ${errMsg}`);
		}
		// 检查响应体结构完整性
		if (!result?.body?.choices) {
			throw new Error(`模型响应异常: status=${result?.status}, body=${JSON.stringify(result?.body)?.substring(0, 200)}`);
		}
		// 返回模型响应
		return result;
	}
	/** 从 记忆库 查询相关消息并填充 ragMessages */
	public queryRagMessages(): this {
		/** 获取最新的5条用户消息作为查询条件 */
		const userMessages = this.getLatestUserMessages();
		// 如果没有用户消息，直接返回
		if (userMessages.length === 0) return this;
		// 初始化 记忆库
		if (!BaseConfig.memoryReady) BaseConfig.initMemory();
		// 如果初始化失败，直接返回
		if (!BaseConfig.memoryReady) return this;
		/** 所有查询结果汇总（含相似度分数） */
		const allResults: { id: string, role: string, content: string, similarity: number }[] = [];
		// 对每条用户消息分别查询 记忆库
		for (const userMessage of userMessages) {
			const [results, error] = memoryQuery('lunar_messages', userMessage, 5);
			// 单条查询失败则跳过，继续处理下一条
			if (error) {
				console.error('记忆库查询失败:', error);
				continue;
			}
			if (results && results.length > 0) {
				// 记忆库 已按相似度降序返回结果
				allResults.push(...results);
			}
		}
		// 如果没有任何结果，直接返回
		if (allResults.length === 0) return this;
		/** 基于内容去重，保留相似度最高的记录 */
		const seen = new Map<string, { id: string, role: string, content: string, similarity: number }>();
		// 遍历所有结果，对相同内容只保留相似度最高的
		for (const r of allResults) {
			const existing = seen.get(r.content);
			if (!existing || r.similarity > existing.similarity) {
				seen.set(r.content, r);
			}
		}
		// 按相似度降序排列，确保相关度最高的结果在最前面
		const uniqueResults = Array.from(seen.values()).sort((a, b) => b.similarity - a.similarity);
		// 输出排序验证信息
		console.log(`[RAG] 查询到 ${uniqueResults.length} 条相关消息，相似度范围: ${uniqueResults[0]?.similarity?.toFixed(4) ?? 'N/A'} ~ ${uniqueResults[uniqueResults.length - 1]?.similarity?.toFixed(4) ?? 'N/A'}`);
		// 写入 ragMessages
		this.ragMessages = uniqueResults.map(r => ({ role: r.role as PostMessageRole, content: r.content }));
		return this;
	}
	/** 获取最新的5条用户消息内容 */
	private getLatestUserMessages(): string[] {
		/** 收集到的用户消息文本 */
		const userTexts: string[] = [];
		// 从消息列表的末尾开始遍历，收集最新的5条用户消息
		for (let i = this.messages.length - 1; i >= 0 && userTexts.length < 5; i--) {
			/** 检查当前消息是否为用户消息 */
			const message = this.messages[i];
			if (message.role === 'user') {
				// 提取文本内容
				if (typeof message.content === 'string') {
					userTexts.unshift(message.content);
				} else if (Array.isArray(message.content)) {
					const textContent = message.content
						.filter(item => item.type === 'text')
						.map(item => item.text)
						.join(' ');
					if (textContent.trim()) userTexts.unshift(textContent);
				}
			}
		}
		return userTexts;
	}
	/** 构建模型响应实例 */
	public constructor(prompt: string) {
		super();
		// 补全系统提示词
		this.systemPrompt = this.promptCompletion(prompt);
	}

	/**
	 * 导出当前子智能体的运行时上下文到本地 JSON 文件（覆写模式）
	 *
	 * 用于调试排查消息重复、上下文异常等问题。
	 * 所有继承 ModelBuilder 的子智能体均可使用此方法。
	 *
	 * @param roleName 角色名称（用于日志和文件命名）
	 * @param outputPath 输出文件路径（默认 agent_debug_{roleName}.json）
	 * @returns 导出文件路径，或空字符串表示失败
	 */
	public dumpContext(roleName: string, outputPath?: string): string {
		const timestamp = new Date().toLocaleString('zh-CN', {
			year: 'numeric', month: '2-digit', day: '2-digit',
			hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
		});

		const snapshot = {
			timestamp,
			role: roleName,
			systemPrompt: this.systemPrompt,
			messagesCount: this.messages.length,
			messages: this.messages.map((msg, idx) => {
				const content = typeof msg.content === 'string'
					? msg.content
					: JSON.stringify(msg.content);
				return {
					index: idx,
					role: msg.role,
					contentPreview: content.length > 500 ? content.slice(0, 500) + '...' : content,
					contentLength: content.length,
				};
			}),
			ragMessagesCount: this.ragMessages.length,
			ragMessages: this.ragMessages.map((msg, idx) => ({
				index: idx,
				role: msg.role,
				contentPreview: typeof msg.content === 'string'
					? (msg.content.length > 300 ? msg.content.slice(0, 300) + '...' : msg.content)
					: JSON.stringify(msg.content).slice(0, 300),
			})),
			runtimeMessagesCount: this.runtimeMessages.length,
			runtimeMessages: this.runtimeMessages.map((msg, idx) => ({
				index: idx,
				role: msg.role,
				contentPreview: typeof msg.content === 'string'
					? (msg.content.length > 300 ? msg.content.slice(0, 300) + '...' : msg.content)
					: JSON.stringify(msg.content).slice(0, 300),
			})),
			stream: this.stream,
			enableTools: this.enableTools,
		};

		const path = outputPath || `agent_debug_${roleName}.json`;
		const [, error] = saveDebugFile(path, JSON.stringify(snapshot, null, 2));
		if (error) {
			console.error(`[${roleName}] 导出上下文失败:`, error);
			return '';
		}

		console.log(`[${roleName}] 上下文快照已导出: ${path}`);
		return path;
	}
}