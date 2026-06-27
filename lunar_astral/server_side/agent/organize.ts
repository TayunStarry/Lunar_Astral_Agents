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
		return `请整理以下 ${records.length} 条对话记录:\n\n${recordTexts.join('\n')}\n\n【重要原则】合并优先，新增为辅！请严格按照以下流程操作：\n1. 先用 query_existing_records 充分查询已有档案（建议 top_k=10），确认是否存在相似记录\n2. 如果找到语义相似的已有记录，必须使用 merge_existing_record 合并到已有记录中，而非创建新条目\n3. 仅当确认无任何相似记录时，才使用 store_organized_record 新增\n4. 完全重复的信息直接跳过，不存储\n\n每条记录必须严格遵循格式：[时间戳] 地点:{地点} | 人物:{参与者} | 事件:{事件摘要} | 话题:{关键词}。完成后请输出整理报告。`;
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
				description: "查询向量数据库中已存在的历史档案记录，用于查重和关联。在生成新记录前，必须先调用此工具确认是否已有相似记录。建议使用较大的 top_k 值（如10）以确保充分查重。",
				parameters: {
					type: "object",
					properties: {
						query_text: {
							type: "string",
							description: "用于语义检索的查询关键词或描述文本，建议使用多个关键词组合查询"
						},
						top_k: {
							type: "integer",
							description: "返回最相关的记录数量，建议设为10以确保充分查重，最大不超过20条"
						}
					},
					required: ["query_text"]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "merge_existing_record",
				description: "将新内容合并到已有的历史档案记录中。当新内容与已有记录存在语义关联（同一话题延续、同一事件更新、内容补充等）时，必须使用此工具而非 store_organized_record。操作会删除旧记录并存储合并后的新记录。",
				parameters: {
					type: "object",
					properties: {
						id: {
							type: "string",
							description: "要合并的已有记录ID，从 query_existing_records 返回结果中获得"
						},
						merged_content: {
							type: "string",
							description: "合并后的完整记录内容，必须包含旧记录和新记录的所有关键信息，严格遵循格式：[时间戳] 地点:{地点} | 人物:{参与者} | 事件:{事件摘要} | 话题:{关键词}"
						}
					},
					required: ["id", "merged_content"]
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
				description: "将整理好的结构化记录存储到向量数据库。仅当通过 query_existing_records 确认无相似记录时才可使用此工具。如果存在相似记录，应使用 merge_existing_record 合并而非新建。每条记录必须严格遵循格式：[时间戳] 地点:{地点} | 人物:{参与者} | 事件:{事件摘要} | 话题:{关键词}",
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
				return this.handleQueryRecords(args.query_text || '', args.top_k || 10);
			case 'merge_existing_record':
				return this.handleMergeRecord(args.id || '', args.merged_content || '');
			case 'delete_existing_record':
				return this.handleDeleteRecord(args.id || '');
			case 'store_organized_record':
				return this.handleStoreRecord(args.content || '');
			default:
				console.warn(`[编纂者] 未知工具: ${funcName}`);
				return `未知工具: ${funcName}，可用工具为 query_existing_records、merge_existing_record、delete_existing_record 和 store_organized_record`;
		}
	}
	/** 处理查询记录工具 */
	protected handleQueryRecords(queryText: string, topK: number): string {
		if (!queryText || queryText.trim().length === 0) {
			return '查询文本为空，请提供有效的查询关键词';
		}

		const [results, error] = chromemQuery('lunar_messages', queryText.trim(), topK);
		if (error) {
			console.error('[编纂者] chromem 查询失败:', error);
			return `向量数据库查询失败: ${error}`;
		}

		if (!results || results.length === 0) {
			return '未找到相关历史记录，可以放心创建新档案';
		}

		// chromem-go 已按相似度降序返回结果，直接使用即可
		return '找到以下相关历史记录（按相关度从高到低排列）:\n' + results
			.map((r: { id: string; role: string; content: string; similarity: number }, i: number) =>
				`[已有记录${i + 1}] ID:${r.id} | 相似度:${(r.similarity * 100).toFixed(1)}% | 内容:${r.content}`)
			.join('\n');
	}
	/** 处理合并记录工具 */
	protected handleMergeRecord(id: string, mergedContent: string): string {
		if (!id || id.trim().length === 0) {
			return '记录ID为空，无法合并，请提供从 query_existing_records 获取的记录ID';
		}
		if (!mergedContent || mergedContent.trim().length === 0) {
			return '合并内容为空，已跳过合并';
		}

		if (!BaseConfig.chromemReady) {
			BaseConfig.initChromem();
			if (!BaseConfig.chromemReady) {
				return '向量数据库未就绪，合并失败，请稍后重试';
			}
		}

		// 先删除旧记录
		const [deleteResult, deleteError] = chromemDelete('lunar_messages', id.trim());
		if (deleteError) {
			console.error('[编纂者] 合并时删除旧记录失败:', deleteError);
			return `合并失败：删除旧记录 ${id} 时出错: ${deleteError}`;
		}
		console.log(`[编纂者] 合并：已删除旧记录 ${id}`);

		// 存储合并后的新记录
		const finalContent = this.ensureTimestampInRecord(mergedContent.trim());
		const [addResult, addError] = chromemAdd('lunar_messages', 'assistant', finalContent);
		if (addError) {
			console.error('[编纂者] 合并时存储新记录失败:', addError);
			return `合并失败：旧记录 ${id} 已删除，但存储合并内容时出错: ${addError}。合并内容: ${finalContent.slice(0, 200)}`;
		}

		console.log(`[编纂者] 合并成功：旧记录 ${id} 已替换为合并内容`);
		return `记录合并成功：已将旧记录 ${id} 替换为合并后的内容`;
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

		const [result, error] = chromemDelete('lunar_messages', id.trim());
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

		// 存储前进行相似度检查：提取关键词查询已有记录
		const trimmedContent = content.trim();
		const topicMatch = trimmedContent.match(/话题[:：](.+)/);
		const eventMatch = trimmedContent.match(/事件[:：](.+?)[|｜]/);
		const checkQuery = topicMatch ? topicMatch[1].trim() : eventMatch ? eventMatch[1].trim() : trimmedContent.slice(0, 50);

		if (checkQuery) {
			const [existingResults] = chromemQuery('lunar_messages', checkQuery, 5);
			if (existingResults && existingResults.length > 0) {
				const similarRecords = existingResults
					.map((r: { id: string; content: string; similarity: number }, i: number) =>
						`[相似记录${i + 1}] ID:${r.id} | 相似度:${(r.similarity * 100).toFixed(1)}% | 内容:${r.content}`)
					.join('\n');
				console.warn('[编纂者] 存储前发现相似记录，建议合并而非新增');
				return `⚠️ 检测到可能存在相似的历史记录，建议使用 merge_existing_record 合并而非新建：\n${similarRecords}\n\n如果确认这些记录与新内容无关，请再次调用 store_organized_record 并说明理由。`;
			}
		}

		const finalContent = this.ensureTimestampInRecord(trimmedContent);
		const [result, error] = chromemAdd('lunar_messages', 'assistant', finalContent);
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
			chromemAdd('lunar_messages', message.role, content);
		}
	}
	/** 查询历史记录 */
	public queryHistoricalRecords(queryText: string, topK: number = 10): (PostMessage & { id: string })[] {
		if (!BaseConfig.chromemReady) BaseConfig.initChromem();
		if (!BaseConfig.chromemReady) return [];

		const [results, error] = chromemQuery('lunar_messages', queryText, topK);
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
		const MAX_ITERATIONS = 8;

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
