import { ToolCall, PostMessage, ModelBuilder, modelResponse, ToolCallItem } from '../index';

/**
 * 创作型子智能体基座
 *
 * 绘画师与音乐家共享相同的工作流：
 *   构建上下文 → 关键词匹配 → 推理循环 → 收集创作详情 → 写入历史 → 供对话者消费
 *
 * 子类只需实现5个抽象钩子即可完成特化。
 *
 * @typeParam TDetail - 创作详情类型（如 PaintingDetail / MusicPieceDetail）
 */
export abstract class CreativeRoleBase<TDetail> extends ModelBuilder {
	/** 独立历史（跨周期持久化，供对话者消费后清空） */
	private _history: PostMessage[] = [];
	/** 对话历史读取条数 */
	protected readonly DIALOGUE_HISTORY_LIMIT = 15;
	/** 自身历史读取条数 */
	protected readonly OWN_HISTORY_LIMIT = 5;
	/** 最大推理迭代次数 */
	protected readonly MAX_ITERATIONS = 3;
	/** 未读消息检查条数 */
	protected readonly UNREAD_CHECK_COUNT = 10;

	/** 构造函数 */
	protected constructor(prompt: string) {
		super(prompt);
	}

	/** 角色名称（用于日志） */
	protected abstract get roleName(): string;

	/** 检查未读消息是否匹配创作关键词 */
	protected abstract matchKeywords(texts: string[]): boolean;

	/** 获取工具定义 */
	protected abstract getToolDefinitions(): ToolCall[];

	/** 执行工具调用，返回结果描述 */
	protected abstract executeTool(toolCall: ToolCallItem): string;

	/** 从工具调用中收集创作详情 */
	protected abstract collectDetail(toolCall: ToolCallItem, details: TDetail[]): void;

	/** 根据创作详情构建摘要，供对话者使用 */
	protected abstract buildSummary(details: TDetail[]): string;

	/** 获取历史摘要（对话者调用后清空） */
	public consumeHistory(): PostMessage[] {
		const result = [...this._history];
		this._history = [];
		return result;
	}

	/**
	 * 执行创作流程
	 *
	 * @param dialogueMessages 对话历史消息（来自 dialogueRole.messages）
	 * @param unreadContext 当前未读上下文快照
	 * @param count 检查的消息数量
	 *
	 * @returns true 表示未执行创作，false 表示已执行创作
	 */
	public createCreativeWork(dialogueMessages: PostMessage[], unreadContext: PostMessage[], count: number = this.UNREAD_CHECK_COUNT): boolean {
		// 构建上下文：对话历史 + 自身历史 + 当前未读
		const dialogueHistory = dialogueMessages.slice(-this.DIALOGUE_HISTORY_LIMIT);
		const ownHistory = this._history.slice(-this.OWN_HISTORY_LIMIT);
		this.coverContext([...dialogueHistory, ...ownHistory, ...unreadContext]);

		// 提取未读消息文本
		const unreadTexts = this.extractUnreadTexts(unreadContext, count);

		// 检查是否匹配创作关键词
		if (!this.matchKeywords(unreadTexts)) return true;

		// 创作记录
		const details: TDetail[] = [];

		// 推理循环
		for (let i = 0; i < this.MAX_ITERATIONS; i++) {
			console.log(`[${this.roleName}] 第 ${i + 1} 轮推理`);
			/** LLM 响应 */
			let response: modelResponse;
			try {
				response = this.run([], this.getToolDefinitions());
			}
			catch (error) {
				console.error(`[${this.roleName}] 第 ${i + 1} 轮推理失败:`, error);
				break;
			}
			/** 模型返回的选项 */
			const choice = response.body?.choices?.[0];
			if (!choice) {
				console.log(`[${this.roleName}] 模型返回空结果，结束循环`);
				break;
			}
			/** 工具调用列表 */
			const toolCalls = choice.message?.tool_calls;
			// 如果模型没有调用任何工具，直接结束循环
			if (!toolCalls || toolCalls.length === 0) break;
			// 将助手消息写入上下文（包含工具调用信息）
			this.writeContext(choice.message);
			// 遍历执行所有工具调用
			for (const toolCall of toolCalls) {
				console.log(`[${this.roleName}] 执行工具: ${toolCall.function.name}`);
				/** 工具执行结果 */
				const result = this.executeTool(toolCall);
				// 将工具执行结果写入上下文
				this.writeContext({ role: 'tool', content: result, tool_call_id: toolCall.id });
				// 收集创作详情
				this.collectDetail(toolCall, details);
			}
		}

		// 将创作详情写入历史，供对话者消费
		if (details.length > 0) {
			const summary = this.buildSummary(details);
			this._history.push({ role: 'user', content: summary });
			console.log(`[${this.roleName}] 已将 ${details.length} 件作品详情写入历史`);
		}

		return false;
	}

	/** 提取未读消息文本 */
	protected extractUnreadTexts(unreadContext: PostMessage[], count: number): string[] {
		const texts: string[] = [];
		for (const message of unreadContext.slice(-count)) {
			if (typeof message.content === 'string') texts.push(message.content);
			else message.content.forEach(item => { if (item.type === 'text') texts.push(item.text); });
		}
		return texts;
	}
}
