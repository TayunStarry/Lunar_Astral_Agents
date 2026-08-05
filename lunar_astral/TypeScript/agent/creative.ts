import { ToolCall, PostMessage, ModelBuilder, modelResponse, ToolCallItem } from '../index';

/**
 * 创作型子智能体基座
 *
 * 绘画师与音乐家共享相同的工作流：
 *   接收任务描述 → 追加到历史 → 推理循环 → 收集创作详情 → 返回作品描述
 *
 * 子类只需实现5个抽象钩子即可完成特化。
 *
 * @typeParam TDetail - 创作详情类型（如 PaintingDetail / MusicPieceDetail）
 */
export abstract class CreativeRoleBase<TDetail> extends ModelBuilder {
	/** 自身历史读取条数 */
	protected readonly OWN_HISTORY_LIMIT = 5;
	/** 最大推理迭代次数（子类可覆写） */
	protected MAX_ITERATIONS = 3;

	/** 构造函数 */
	protected constructor(prompt: string) {
		super(prompt);
	}

	/** 角色名称（用于日志） */
	protected abstract get roleName(): string;

	/** 获取工具定义 */
	protected abstract getToolDefinitions(): ToolCall[];

	/** 执行工具调用，返回结果描述 */
	protected abstract executeTool(toolCall: ToolCallItem): string;

	/** 从工具调用中收集创作详情 */
	protected abstract collectDetail(toolCall: ToolCallItem, details: TDetail[]): void;

	/** 根据创作详情构建摘要，供对话者使用 */
	protected abstract buildSummary(details: TDetail[]): string;

	/** 获取历史摘要（对话者调用后清空，新架构下不再使用，保留用于调试导出） */
	public consumeHistory(): PostMessage[] {
		const result = [...this.messages];
		this.messages = [];
		return result;
	}

	/**
	 * 覆写 writeContext：子智能体淘汰的消息不进入 OnlyData.unreadRecords
	 *
	 * 对话者淘汰的历史记录才能进入 unreadRecords 供编纂者归档。
	 */
	public writeContext(context: PostMessage): this {
		const cleaned = this.stripReasoningContent(context);
		if (this.messages.length >= 40) {
			this.messages = this.messages.slice(-39).concat(cleaned);
			// 子智能体淘汰的消息不进入 OnlyData.unreadRecords
		}
		else this.messages.push(cleaned);
		return this;
	}

	/**
	 * 执行创作流程
	 *
	 * 接收对话者通过工具调用传来的任务描述，追加到自身历史中，
	 * 运行 LLM 推理循环，调用专业工具链完成创作。
	 *
	 * @param taskDescription 对话者传来的创作任务描述
	 *
	 * @returns 作品描述文本（月华话术格式），或拒绝原因
	 */
	public async createCreativeWork(taskDescription: string): Promise<string> {
		// 将任务描述追加到自身历史中
		this.writeContext({ role: 'user', content: taskDescription });

		// 创作记录
		const details: TDetail[] = [];
		/** 拒绝原因（LLM 判定无需创作时的文本回复） */
		let rejectionReason = '';

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
			// 如果模型没有调用任何工具，捕获其文本回复作为拒绝原因
			if (!toolCalls || toolCalls.length === 0) {
				if (choice.message?.content && choice.message.content.trim()) {
					rejectionReason = choice.message.content;
				}
				// 将助手消息写入上下文（保留拒绝原因供后续推理参考）
				this.writeContext(choice.message);
				break;
			}
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

		// 如果没有创作产出，返回拒绝原因
		if (details.length === 0) {
			const reason = rejectionReason || '月华认为此次无需进行创作';
			console.log(`[${this.roleName}] 未产出作品，原因: ${reason}`);
			return reason;
		}

		// 构建作品摘要并返回
		const summary = this.buildSummary(details);
		console.log(`[${this.roleName}] 已完成 ${details.length} 件作品创作`);
		return summary;
	}
}