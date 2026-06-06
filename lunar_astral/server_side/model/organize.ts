import { OnlyData, modelResponse, ModelBuilder, PostMessage, BaseConfig, ToolCall, ToolCallItem } from '../index';

/** 编纂者提示词构建 */
class Prompt extends ModelBuilder {
	/** 构建整理提示词 */
	protected buildOrganizePrompt(records: PostMessage[]): string {
		const now = new Date();
		const recordTexts = records.map((msg, idx) => {
			const content = typeof msg.content === 'string'
				? msg.content
				: JSON.stringify(msg.content);
			const preview = content.length > 300 ? content.slice(0, 300) + '...' : content;
			const timestamp = now.toLocaleString('zh-CN', {
				year: 'numeric',
				month: '2-digit',
				day: '2-digit',
				hour: '2-digit',
				minute: '2-digit',
				second: '2-digit',
				hour12: false
			});
			return `[记录${idx + 1}] 时间:${timestamp} | 角色:${msg.role} | 内容:${preview}`;
		});
		return `请整理以下 ${records.length} 条对话记录:\n\n${recordTexts.join('\n')}\n\n请按照流程操作：先查询已有档案，再生成结构化描述，最后存储到向量数据库。每条记录必须严格遵循格式：[时间戳] 地点:{地点} | 人物:{参与者} | 事件:{事件摘要} | 话题:{关键词}。完成后请输出整理报告。`;
	}
	/** 确保记录中包含时间戳 */
	protected ensureTimestampInRecord(content: string): string {
		const timestampRegex = /^\[([^\]]+)\]/;
		if (timestampRegex.test(content)) {
			return content;
		}

		const now = new Date();
		const timestamp = now.toLocaleString('zh-CN', {
			year: 'numeric',
			month: '2-digit',
			day: '2-digit',
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit',
			hour12: false
		});

		return `[${timestamp}] ${content}`;
	}
}

/** 编纂者工具链 */
class Toolchain extends Prompt {
	/** 编纂者工具定义 */
	protected organizeTools: ToolCall[] = [
		{
			type: "function",
			function: {
				name: "query_existing_records",
				description: "查询向量数据库中已存在的历史档案记录，用于查重和关联。在生成新记录前，应先调用此工具确认是否已有相似记录。",
				parameters: {
					type: "object",
					properties: {
						query_text: {
							type: "string",
							description: "用于语义检索的查询关键词或描述文本"
						},
						top_k: {
							type: "integer",
							description: "返回最相关的记录数量，默认5条，建议不超过10条"
						}
					},
					required: ["query_text"]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "delete_existing_record",
				description: "删除向量数据库中已存在的重复或过时的历史档案记录。在合并或更新已有记录时，应先删除旧记录再存储新记录。",
				parameters: {
					type: "object",
					properties: {
						id: {
							type: "string",
							description: "要删除的历史记录ID，从 query_existing_records 返回结果中获得"
						}
					},
					required: ["id"]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "store_organized_record",
				description: "将整理好的结构化记录存储到向量数据库。每条记录必须严格遵循格式：[时间戳] 地点:{地点} | 人物:{参与者} | 事件:{事件摘要} | 话题:{关键词}",
				parameters: {
					type: "object",
					properties: {
						content: {
							type: "string",
							description: "结构化记录内容，严格遵循指定格式，包含时间、地点、人物、事件、话题五个维度的完整信息"
						}
					},
					required: ["content"]
				}
			}
		}
	];
	/** 执行编纂工具调用 */
	protected executeOrganizeTool(toolCall: ToolCallItem): string {
		const funcName = toolCall.function.name;
		let args: Record<string, any> = {};

		try {
			args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
		} catch {
			console.error(`[编纂者] 工具调用参数解析失败:`, toolCall.function.arguments);
			return `工具调用参数解析失败，请确保传入合法的 JSON 字符串`;
		}

		console.log(`[编纂者] 执行工具: ${funcName}`);

		switch (funcName) {
			case 'query_existing_records':
				return this.handleQueryRecords(args.query_text || '', args.top_k || 5);
			case 'delete_existing_record':
				return this.handleDeleteRecord(args.id || '');
			case 'store_organized_record':
				return this.handleStoreRecord(args.content || '');
			default:
				console.warn(`[编纂者] 未知工具: ${funcName}`);
				return `未知工具: ${funcName}，可用工具为 query_existing_records、delete_existing_record 和 store_organized_record`;
		}
	}
	/** 处理查询记录工具 */
	protected handleQueryRecords(queryText: string, topK: number): string {
		if (!queryText || queryText.trim().length === 0) {
			return '查询文本为空，请提供有效的查询关键词';
		}

		const [results, error] = chromemQuery(queryText.trim(), topK);
		if (error) {
			console.error('[编纂者] chromem 查询失败:', error);
			return `向量数据库查询失败: ${error}`;
		}

		if (!results || results.length === 0) {
			return '未找到相关历史记录，可以放心创建新档案';
		}

		return '找到以下相关历史记录:\n' + results
			.map((r: { id: string; role: string; content: string }, i: number) => `[已有记录${i + 1}] ID:${r.id} | 内容:${r.content}`)
			.join('\n');
	}
	/** 处理删除记录工具 */
	protected handleDeleteRecord(id: string): string {
		if (!id || id.trim().length === 0) {
			return '记录ID为空，已跳过删除';
		}

		if (!BaseConfig.chromemReady) {
			BaseConfig.initChromem();
			if (!BaseConfig.chromemReady) {
				return '向量数据库未就绪，删除失败，请稍后重试';
			}
		}

		const [result, error] = chromemDelete(id.trim());
		if (error) {
			console.error('[编纂者] chromem 删除失败:', error);
			return `向量数据库删除失败: ${error}`;
		}

		return result ? `记录 ${id} 已成功从向量数据库删除` : `删除操作已完成但未返回确认信息`;
	}
	/** 处理存储记录工具 */
	protected handleStoreRecord(content: string): string {
		if (!content || content.trim().length === 0) {
			return '记录内容为空，已跳过存储';
		}

		if (!BaseConfig.chromemReady) {
			BaseConfig.initChromem();
			if (!BaseConfig.chromemReady) {
				return '向量数据库未就绪，存储失败，请稍后重试';
			}
		}

		const finalContent = this.ensureTimestampInRecord(content.trim());
		const [result, error] = chromemAdd('system', finalContent);
		if (error) {
			console.error('[编纂者] chromem 存储失败:', error);
			return `向量数据库存储失败: ${error}`;
		}

		return result ? '记录已成功存储到向量数据库' : '存储操作已完成但未返回确认信息';
	}
}

/** 编纂者角色 */
export class OrganizeRole extends Toolchain {
	constructor() {
		super(fileView('prompts/organizeRole.md')[0]);
	}
	/** 组织历史记录 */
	public organizeHistoricalRecords(): void {
		console.log('[编纂者] 开始组织历史记录');
		if (OnlyData.unreadRecords.length === 0) {
			console.log('[编纂者] 没有未读记录需要整理');
			return;
		}

		if (!BaseConfig.chromemReady) {
			BaseConfig.initChromem();
			if (!BaseConfig.chromemReady) {
				console.warn('[编纂者] 向量数据库未就绪，保留未读记录待下次整理');
				return;
			}
		}

		try {
			const organizePrompt = this.buildOrganizePrompt(OnlyData.unreadRecords);
			this.coverContext({ role: 'user', content: organizePrompt });
			this.runtimeMessages = [
				{ role: 'user', content: `当前时间: ${new Date().toLocaleString()}` }
			];

			this.executeOrganizeLoop();

			console.log('[编纂者] 历史记录组织完成');
			OnlyData.unreadRecords = [];
		}
		catch (error) {
			console.error('[编纂者] 组织历史记录失败，保留未读记录待下次重试:', error);
		}
	}
	/** 持久化被抛弃的消息 */
	public persistDiscardedMessages(discarded: PostMessage[]): void {
		console.log('[编纂者] 开始持久化被抛弃的消息');
		if (!BaseConfig.chromemReady) BaseConfig.initChromem();
		if (!BaseConfig.chromemReady) return;
		for (const message of discarded) {
			const content = typeof message.content === 'string' ? message.content : JSON.stringify(message.content);
			chromemAdd(message.role, content);
		}
	}
	/** 查询历史记录 */
	public queryHistoricalRecords(queryText: string, topK: number = 10): (PostMessage & { id: string })[] {
		if (!BaseConfig.chromemReady) BaseConfig.initChromem();
		if (!BaseConfig.chromemReady) return [];

		const [results, error] = chromemQuery(queryText, topK);
		if (error) {
			console.error('[编纂者] 查询历史记录失败:', error);
			return [];
		}

		if (!results || results.length === 0) return [];

		return results.map((r: { id: string; role: string; content: string }) => ({
			id: r.id,
			role: r.role as PostMessage['role'],
			content: r.content
		}));
	}
	/** 获取历史记录上下文 */
	public getHistoricalContext(maxResults: number = 5): string {
		const records = this.queryHistoricalRecords('近期对话 重要事件', maxResults);
		if (records.length === 0) return '';
		return records.map(r => r.content).join('\n');
	}
	/** 执行组织历史记录循环 */
	private executeOrganizeLoop(): void {
		const MAX_ITERATIONS = 5;

		for (let i = 0; i < MAX_ITERATIONS; i++) {
			console.log(`[编纂者] 第 ${i + 1} 轮模型推理`);

			let response: modelResponse;
			try {
				response = this.run([], this.organizeTools);
			} catch (error) {
				console.error(`[编纂者] 第 ${i + 1} 轮推理失败:`, error);
				break;
			}

			const choice = response.body?.choices?.[0];
			if (!choice) {
				console.log('[编纂者] 模型返回空结果，结束循环');
				break;
			}

			const toolCalls = choice.message?.tool_calls;
			if (!toolCalls || toolCalls.length === 0) {
				const replyContent = choice.message?.content || '';
				console.log('[编纂者] 模型完成整理:', replyContent.slice(0, 300));
				if (replyContent) {
					this.writeContext(choice.message);
				}
				break;
			}

			console.log(`[编纂者] 第 ${i + 1} 轮工具调用, 共 ${toolCalls.length} 个工具`);

			this.writeContext(choice.message);

			for (const toolCall of toolCalls) {
				const result = this.executeOrganizeTool(toolCall);
				this.writeContext({
					role: 'tool',
					content: result,
					tool_call_id: toolCall.id
				});
			}
		}
	}
}
