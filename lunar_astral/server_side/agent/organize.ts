import { OnlyData, modelResponse, ModelBuilder, PostMessage, BaseConfig } from '../index';

/** 记忆点摘要 */
interface MemorySummary {
	/** 时间信息 */
	time: string;
	/** 地点信息 */
	location: string;
	/** 事件核心内容 */
	content: string;
	/** 话题关键词 */
	topic: string;
}

/** 摘要处理决策 */
interface SummaryDecision {
	/** 对应的摘要 */
	summary: MemorySummary;
	/** 需要删除的旧记录ID列表 */
	deleteIds: string[];
	/** 是否需要写入新记录 */
	shouldStore: boolean;
	/** 写入的内容（可能是合并后的内容） */
	storeContent: string;
}

/** 编纂者提示词构建 */
class Prompt extends ModelBuilder {
	/** 当前地址缓存 */
	private currentLocation: string = '';

	/** 获取当前位置信息（通过 address() 调用，参见 builder.ts#L42） */
	private getCurrentLocation(): string {
		if (this.currentLocation) return this.currentLocation;
		const [addressResult, error] = address();
		if (error || !addressResult || addressResult.length === 0) {
			this.currentLocation = '未知地点';
		} else {
			this.currentLocation = addressResult.join(' ');
		}
		return this.currentLocation;
	}

	/** 获取当前时间字符串 */
	private getCurrentTime(): string {
		return new Date().toLocaleString('zh-CN', {
			year: 'numeric',
			month: '2-digit',
			day: '2-digit',
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit',
			hour12: false
		});
	}

	/** 构建摘要生成提示词（阶段一：每个事件独立摘要） */
	protected buildSummarizePrompt(records: PostMessage[]): string {
		const recordTexts = records.map((msg, idx) => {
			const content = typeof msg.content === 'string'
				? msg.content
				: JSON.stringify(msg.content);
			const preview = content.length > 500 ? content.slice(0, 500) + '...' : content;
			return `[事件${idx + 1}] 角色:${msg.role} | 内容:${preview}`;
		});
		const currentTime = this.getCurrentTime();
		const currentLocation = this.getCurrentLocation();
		return `请将以下 ${records.length} 条历史事件数据，每个事件独立摘要为一个简洁准确的记忆点摘要。

【事件数据】
${recordTexts.join('\n')}

【系统上下文】
- 当前时间: ${currentTime}（若事件未包含时间信息，使用此时间）
- 当前位置: ${currentLocation}（若事件未包含地点信息，使用此位置）

【处理规则】
1. 每个事件独立生成一条记忆点摘要，不要跨事件合并
2. 若事件明确包含时间信息，保留原时间；否则使用上述当前时间
3. 若事件明确包含地点信息，保留原地点；否则使用上述当前位置
4. 摘要内容简洁准确，聚焦核心事实

【输出格式】
请输出 JSON 数组，每个元素对应一个事件的记忆点摘要：
\`\`\`json
[
  {
    "time": "时间信息（从事件中提取，若无则使用当前时间）",
    "location": "地点信息（从事件中提取，若无则使用当前位置）",
    "content": "事件的核心内容摘要，简洁准确",
    "topic": "事件的关键词或主题"
  }
]
\`\`\`

仅输出 JSON 数组，不要包含其他说明文字。`;
	}

	/** 构建决策提示词（阶段二：针对单条摘要决策） */
	protected buildDecisionPrompt(summary: MemorySummary, existingRecords: string): string {
		return `请针对以下记忆点摘要，判断应执行的操作：

【当前摘要】
- 时间: ${summary.time}
- 地点: ${summary.location}
- 内容: ${summary.content}
- 话题: ${summary.topic}

【已有相关记录】
${existingRecords}

【决策要求】
请判断以下三项：
1. 是否需要与已有记录合并？若合并，需提供合并后的完整内容（合并方式：删除旧记录，写入新合并记录）
2. 是否需要删除某些已有记录？列出要删除的记录ID
3. 是否需要将当前摘要持久化存储到数据库？

【决策原则】
- 完全重复的信息：删除旧的，不写入新的（should_store=false）
- 语义关联可合并：删除旧的，写入合并后的新内容（should_store=true，store_content为合并后内容）
- 无相似记录：直接写入新摘要（should_store=true，store_content为原摘要格式化内容）
- 已有记录过时但新摘要无价值：仅删除旧的（should_store=false）

【输出格式】
请输出 JSON 对象：
\`\`\`json
{
  "delete_ids": ["需要删除的记录ID列表"],
  "should_store": true或false,
  "store_content": "若存储，使用的内容（合并时为合并后内容，否则为原摘要格式化内容）"
}
\`\`\`

仅输出 JSON 对象，不要包含其他说明文字。`;
	}

	/** 将摘要格式化为标准记录格式 */
	protected formatSummaryAsRecord(summary: MemorySummary): string {
		return `[${summary.time}] 地点:${summary.location} | 事件:${summary.content} | 话题:${summary.topic}`;
	}

	/** 确保记录中包含时间戳 */
	protected ensureTimestampInRecord(content: string): string {
		const timestampRegex = /^\[([^\]]+)\]/;
		if (timestampRegex.test(content)) return content;
		return `[${this.getCurrentTime()}] ${content}`;
	}
}

/** 编纂者工具链 */
class Toolchain extends Prompt {
	/** 查询已有记录 */
	protected queryExistingRecords(queryText: string, topK: number = 10): Array<{ id: string; content: string; similarity: number }> {
		if (!queryText || queryText.trim().length === 0) return [];
		const [results, error] = memoryQuery('lunar_messages', queryText.trim(), topK);
		if (error) {
			console.error('[编纂者] 记忆库查询失败:', error);
			return [];
		}
		return results || [];
	}

	/** 批量执行决策（先全部删除，再全部写入） */
	protected executeBatchActions(decisions: SummaryDecision[]): void {
		// 收集所有需要删除的ID到专用数组
		const allDeleteIds: string[] = [];
		for (const decision of decisions) {
			allDeleteIds.push(...decision.deleteIds);
		}

		// 先执行全部删除操作
		const uniqueDeleteIds = [...new Set(allDeleteIds)];
		console.log(`[编纂者] 准备删除 ${uniqueDeleteIds.length} 条旧记录`);
		for (const id of uniqueDeleteIds) {
			const trimmedId = id.trim();
			if (!trimmedId) continue;
			const [, error] = memoryDelete('lunar_messages', trimmedId);
			if (error) console.error(`[编纂者] 删除记录 ${trimmedId} 失败:`, error);
			else console.log(`[编纂者] 已删除记录 ${trimmedId}`);
		}

		// 再执行新摘要的写入操作
		const toStore = decisions.filter(d => d.shouldStore);
		console.log(`[编纂者] 准备写入 ${toStore.length} 条新记录`);
		for (const decision of toStore) {
			if (!decision.storeContent || decision.storeContent.trim().length === 0) continue;
			const finalContent = this.ensureTimestampInRecord(decision.storeContent.trim());
			const [, error] = memoryAdd('lunar_messages', 'assistant', finalContent);
			if (error) console.error('[编纂者] 写入记录失败:', error);
			else console.log('[编纂者] 已写入新记录');
		}
	}

	/** 解析模型返回的 JSON */
	protected parseJsonResponse<T>(content: string): T | null {
		try {
			// 提取 JSON 部分（处理模型可能包裹在 markdown 代码块中的情况）
			const jsonMatch = content.match(/```(?:json)?\s*([\s\S]*?)```/);
			const jsonStr = jsonMatch ? jsonMatch[1].trim() : content.trim();
			return JSON.parse(jsonStr) as T;
		} catch (error) {
			console.error('[编纂者] JSON 解析失败:', error, '原始内容:', content.slice(0, 200));
			return null;
		}
	}
}

/** 编纂者角色 */
export class OrganizeRole extends Toolchain {
	constructor() {
		super(fileView('prompts/organizeRole.md')[0]);
	}

	/** 组织历史记录（主入口） */
	public organizeHistoricalRecords(): void {
		console.log('[编纂者] 开始组织历史记录');
		if (OnlyData.unreadRecords.length === 0) {
			console.log('[编纂者] 没有未读记录需要整理');
			return;
		}

		if (!BaseConfig.memoryReady) {
			BaseConfig.initMemory();
			if (!BaseConfig.memoryReady) {
				console.warn('[编纂者] 记忆库未就绪，保留未读记录待下次整理');
				return;
			}
		}

		try {
			// 阶段一：基于历史事件数据，将每个事件独立摘要为记忆点
			const summaries = this.generateMemorySummaries(OnlyData.unreadRecords);
			if (summaries.length === 0) {
				console.log('[编纂者] 未生成有效摘要，结束整理');
				return;
			}
			console.log(`[编纂者] 阶段一完成，生成 ${summaries.length} 条记忆点摘要`);

			// 阶段二：遍历摘要，针对每条决策合并/删除/存储
			const decisions = this.processSummaries(summaries);
			console.log(`[编纂者] 阶段二完成，生成 ${decisions.length} 条决策`);

			// 批量执行：先全部删除，再全部写入
			this.executeBatchActions(decisions);

			console.log('[编纂者] 历史记录组织完成');
			OnlyData.unreadRecords = [];
		}
		catch (error) {
			console.error('[编纂者] 组织历史记录失败，保留未读记录待下次重试:', error);
		}
	}

	/** 阶段一：生成记忆点摘要（每个事件独立摘要，填充时间/地点） */
	private generateMemorySummaries(records: PostMessage[]): MemorySummary[] {
		const prompt = this.buildSummarizePrompt(records);
		this.coverContext({ role: 'user', content: prompt });
		this.runtimeMessages = [];

		let response: modelResponse;
		try {
			response = this.run([], []);
		} catch (error) {
			console.error('[编纂者] 阶段一模型推理失败:', error);
			return [];
		}

		const content = response.body?.choices?.[0]?.message?.content || '';
		const summaries = this.parseJsonResponse<MemorySummary[]>(content);
		if (!summaries || !Array.isArray(summaries)) return [];

		// 过滤无效摘要
		return summaries.filter(s => s && s.content && s.content.trim().length > 0);
	}

	/** 阶段二：遍历所有摘要，针对每条决策合并/删除/存储 */
	private processSummaries(summaries: MemorySummary[]): SummaryDecision[] {
		const decisions: SummaryDecision[] = [];
		for (const summary of summaries) {
			const decision = this.processMemorySummary(summary);
			decisions.push(decision);
		}
		return decisions;
	}

	/** 针对单条摘要决策：判断合并/删除/存储 */
	private processMemorySummary(summary: MemorySummary): SummaryDecision {
		// 查询已有相关记录（用于判断合并/删除）
		const queryText = summary.topic || summary.content.slice(0, 50);
		const existing = this.queryExistingRecords(queryText, 10);

		const existingText = existing.length === 0
			? '无相关已有记录'
			: existing.map((r, i) =>
				`[记录${i + 1}] ID:${r.id} | 相似度:${(r.similarity * 100).toFixed(1)}% | 内容:${r.content}`
			).join('\n');

		// 调用模型决策
		const prompt = this.buildDecisionPrompt(summary, existingText);
		this.coverContext({ role: 'user', content: prompt });
		this.runtimeMessages = [];

		let response: modelResponse;
		try {
			response = this.run([], []);
		} catch (error) {
			console.error('[编纂者] 阶段二模型推理失败，使用默认决策（存储原摘要）:', error);
			return this.buildFallbackDecision(summary, existing);
		}

		const content = response.body?.choices?.[0]?.message?.content || '';
		const decision = this.parseJsonResponse<{
			delete_ids: string[];
			should_store: boolean;
			store_content: string;
		}>(content);

		if (!decision) {
			// 决策解析失败，使用默认决策（存储原摘要）
			return this.buildFallbackDecision(summary, existing);
		}

		// 若需要存储但未提供 store_content，使用原摘要格式化内容
		let storeContent = decision.store_content || '';
		if (decision.should_store && !storeContent) {
			storeContent = this.formatSummaryAsRecord(summary);
		}

		return {
			summary,
			deleteIds: decision.delete_ids || [],
			shouldStore: !!decision.should_store,
			storeContent
		};
	}

	/** 构建回退决策（模型决策失败时使用） */
	private buildFallbackDecision(summary: MemorySummary, existing: Array<{ id: string; similarity: number }>): SummaryDecision {
		// 回退策略：高相似度的旧记录删除，新摘要直接存储
		const SIMILARITY_THRESHOLD = 0.85;
		const deleteIds = existing
			.filter(r => r.similarity >= SIMILARITY_THRESHOLD)
			.map(r => r.id);
		return {
			summary,
			deleteIds,
			shouldStore: true,
			storeContent: this.formatSummaryAsRecord(summary)
		};
	}

	/** 持久化被抛弃的消息 */
	public persistDiscardedMessages(discarded: PostMessage[]): void {
		console.log('[编纂者] 开始持久化被抛弃的消息');
		if (!BaseConfig.memoryReady) BaseConfig.initMemory();
		if (!BaseConfig.memoryReady) return;
		for (const message of discarded) {
			const content = typeof message.content === 'string' ? message.content : JSON.stringify(message.content);
			memoryAdd('lunar_messages', message.role, content);
		}
	}

	/** 查询历史记录 */
	public queryHistoricalRecords(queryText: string, topK: number = 10): (PostMessage & { id: string })[] {
		if (!BaseConfig.memoryReady) BaseConfig.initMemory();
		if (!BaseConfig.memoryReady) return [];

		const [results, error] = memoryQuery('lunar_messages', queryText, topK);
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
}
