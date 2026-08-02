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

/** 摘要分类 */
type SummaryCategory = 'duplicate' | 'merge' | 'new';

/** 摘要处理决策 */
interface SummaryDecision {
	/** 对应的摘要 */
	summary: MemorySummary;
	/** 分类 */
	category: SummaryCategory;
	/** 需要删除的旧记录ID列表 */
	deleteIds: string[];
	/** 是否需要写入新记录 */
	shouldStore: boolean;
	/** 写入的内容 */
	storeContent: string;
}

/** 待合并项 */
interface MergeCandidate {
	/** 当前摘要 */
	summary: MemorySummary;
	/** 已有相关记录（仅含相似度在合并区间的） */
	existingRecords: Array<{ id: string; content: string; similarity: number }>;
}

/** LLM 合并结果 */
interface MergeResult {
	/** 需要删除的记录ID */
	delete_ids: string[];
	/** 合并后的内容（空字符串表示放弃合并，保留原记录） */
	merged_content: string;
}

/** 编纂者提示词构建 */
class Prompt extends ModelBuilder {
	/** 当前地址缓存 */
	private currentLocation: string = '';

	/** 获取当前位置信息 */
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

	/** 构建摘要生成提示词（阶段一：批量摘要） */
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

	/** 构建批量合并提示词（阶段四：对多组待合并项一次性决策） */
	protected buildBatchMergePrompt(candidates: MergeCandidate[]): string {
		const candidateTexts = candidates.map((c, idx) => {
			const existingText = c.existingRecords.map((r, i) =>
				`[历史记录${i + 1}] ID:${r.id} | 相似度:${(r.similarity * 100).toFixed(1)}% | 内容:${r.content}`
			).join('\n');
			return `--- 待合并项${idx + 1} ---
当前摘要: 时间:${c.summary.time} | 地点:${c.summary.location} | 内容:${c.summary.content} | 话题:${c.summary.topic}
已有历史记录:
${existingText}`;
		}).join('\n\n');

		return `请对以下 ${candidates.length} 个待合并项逐一判断：当前摘要与历史记录内容相似但需要合并更新。

${candidateTexts}

【决策原则】
对每个待合并项：
1. 若当前摘要与历史记录语义完全相同 → merged_content 为空字符串，delete_ids 包含需要清理的历史记录ID
2. 若当前摘要包含历史记录中没有的新信息 → 合并为更完整的记录，merged_content 为合并后内容（以 [时间] 地点:... | 事件:... | 话题:... 格式输出），delete_ids 包含被合并的历史记录ID
3. 若历史记录已足够完整，当前摘要无新增信息 → merged_content 为空字符串，delete_ids 为空数组（保留原记录不变）

【输出格式】
请输出 JSON 数组，每个元素对应一个待合并项的决策结果（顺序与输入一致）：
\`\`\`json
[
  {
    "delete_ids": ["需要删除的历史记录ID"],
    "merged_content": "合并后的完整内容（空字符串表示放弃合并）"
  }
]
\`\`\`

仅输出 JSON 数组，不要包含其他说明文字。`;
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
		const allDeleteIds: string[] = [];
		for (const decision of decisions) {
			allDeleteIds.push(...decision.deleteIds);
		}

		const uniqueDeleteIds = [...new Set(allDeleteIds)];
		console.log(`[编纂者] 准备删除 ${uniqueDeleteIds.length} 条旧记录`);
		for (const id of uniqueDeleteIds) {
			const trimmedId = id.trim();
			if (!trimmedId) continue;
			const [, error] = memoryDelete('lunar_messages', trimmedId);
			if (error) console.error(`[编纂者] 删除记录 ${trimmedId} 失败:`, error);
			else console.log(`[编纂者] 已删除记录 ${trimmedId}`);
		}

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
	/** 重复判定阈值：相似度 >= 此值视为重复，直接放弃 */
	private readonly DUPLICATE_THRESHOLD = 0.85;
	/** 合并判定下限：相似度在此区间需合并更新 */
	private readonly MERGE_THRESHOLD = 0.55;
	/** 去重扫描阈值：写入后扫描时判定重复的相似度 */
	private readonly DEDUP_THRESHOLD = 0.93;
	/** 批量合并每批最大条目数 */
	private readonly MERGE_BATCH_SIZE = 10;

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
			// 阶段一：预处理 — 批量生成结构化事件摘要
			const summaries = this.generateMemorySummaries(OnlyData.unreadRecords);
			if (summaries.length === 0) {
				console.log('[编纂者] 未生成有效摘要，结束整理');
				return;
			}
			console.log(`[编纂者] 阶段一完成，生成 ${summaries.length} 条记忆点摘要`);

			// 阶段二：记忆检索 — 对每条摘要查询相似历史记录
			// 阶段三：重复判断与过滤 — 基于相似度程序化分类
			// 阶段五：新增条目处理 — 无重叠的直接写入
			const { decisions, mergeCandidates } = this.classifyAndDecide(summaries);

			const dupCount = decisions.filter(d => d.category === 'duplicate').length;
			const newCount = decisions.filter(d => d.category === 'new').length;
			const mergeCount = mergeCandidates.length;
			console.log(`[编纂者] 阶段二三完成 — 重复:${dupCount} | 待合并:${mergeCount} | 新增:${newCount}`);

			// 阶段四：内容更新与替换 — 仅对合并类调用 LLM 批量处理
			if (mergeCandidates.length > 0) {
				this.processMergeCandidates(decisions, mergeCandidates);
			}

			// 批量执行：先全部删除，再全部写入
			this.executeBatchActions(decisions);

			// 阶段六：记忆库维护 — 写入后去重扫描
			this.deduplicateMemory();

			console.log('[编纂者] 历史记录组织完成');
			OnlyData.unreadRecords = [];
		}
		catch (error) {
			console.error('[编纂者] 组织历史记录失败，保留未读记录待下次重试:', error);
		}
	}

	/** 阶段一：预处理 — 批量生成结构化事件摘要（1 次 LLM 调用） */
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

		return summaries.filter(s => s && s.content && s.content.trim().length > 0);
	}

	/**
	 * 阶段二~三：记忆检索 + 重复判断与过滤 + 新增条目处理
	 *
	 * 对每条摘要查询记忆库相似记录，基于相似度程序化分类：
	 * - 重复（>= DUPLICATE_THRESHOLD）：放弃当前摘要
	 * - 合并（MERGE_THRESHOLD ~ DUPLICATE_THRESHOLD）：收集待合并项
	 * - 新增（< MERGE_THRESHOLD）：直接写入
	 *
	 * 全程无 LLM 调用，仅向量数据库查询
	 */
	private classifyAndDecide(summaries: MemorySummary[]): {
		decisions: SummaryDecision[];
		mergeCandidates: MergeCandidate[];
	} {
		const decisions: SummaryDecision[] = [];
		const mergeCandidates: MergeCandidate[] = [];

		for (const summary of summaries) {
			const queryText = summary.topic || summary.content.slice(0, 50);
			const existing = this.queryExistingRecords(queryText, 10);

			if (existing.length === 0) {
				// 无相似记录 → 新增
				decisions.push({
					summary,
					category: 'new',
					deleteIds: [],
					shouldStore: true,
					storeContent: this.formatSummaryAsRecord(summary),
				});
				continue;
			}

			// 取最高相似度作为分类依据
			const maxSimilarity = Math.max(...existing.map(r => r.similarity));

			if (maxSimilarity >= this.DUPLICATE_THRESHOLD) {
				// 与已有记录重复 → 放弃当前摘要
				console.log(`[编纂者] 重复过滤（相似度 ${(maxSimilarity * 100).toFixed(1)}%）: "${summary.content.slice(0, 30)}..."`);
				decisions.push({
					summary,
					category: 'duplicate',
					deleteIds: [],
					shouldStore: false,
					storeContent: '',
				});
			} else if (maxSimilarity >= this.MERGE_THRESHOLD) {
				// 需要合并 → 收集待合并项
				const relevantRecords = existing.filter(r => r.similarity >= this.MERGE_THRESHOLD);
				decisions.push({
					summary,
					category: 'merge',
					deleteIds: [],
					shouldStore: false,
					storeContent: '',
				});
				mergeCandidates.push({ summary, existingRecords: relevantRecords });
			} else {
				// 相似度较低 → 新增
				decisions.push({
					summary,
					category: 'new',
					deleteIds: [],
					shouldStore: true,
					storeContent: this.formatSummaryAsRecord(summary),
				});
			}
		}

		return { decisions, mergeCandidates };
	}

	/**
	 * 阶段四：内容更新与替换 — 批量合并处理
	 *
	 * 将所有待合并项分批调用 LLM，每批最多 MERGE_BATCH_SIZE 条，
	 * 用合并结果回填对应决策的 deleteIds / shouldStore / storeContent
	 */
	private processMergeCandidates(decisions: SummaryDecision[], mergeCandidates: MergeCandidate[]): void {
		console.log(`[编纂者] 阶段四：开始批量合并 ${mergeCandidates.length} 项`);

		for (let batchStart = 0; batchStart < mergeCandidates.length; batchStart += this.MERGE_BATCH_SIZE) {
			const batch = mergeCandidates.slice(batchStart, batchStart + this.MERGE_BATCH_SIZE);
			const batchResults = this.executeBatchMerge(batch);

			// 用合并结果回填决策
			for (let i = 0; i < batch.length; i++) {
				const candidate = batch[i];
				const result = batchResults[i];

				// 找到对应的决策项
				const decisionIdx = decisions.findIndex(d =>
					d.category === 'merge' && d.summary === candidate.summary
				);
				if (decisionIdx === -1) continue;

				if (result && result.merged_content && result.merged_content.trim()) {
					// 有合并内容：删除旧记录 + 写入合并后内容
					decisions[decisionIdx].deleteIds = result.delete_ids || [];
					decisions[decisionIdx].shouldStore = true;
					decisions[decisionIdx].storeContent = result.merged_content.trim();
					console.log(`[编纂者] 合并成功: "${candidate.summary.content.slice(0, 30)}..." → 删${decisions[decisionIdx].deleteIds.length}条+写1条`);
				} else if (result && result.delete_ids && result.delete_ids.length > 0) {
					// LLM 判定语义相同，仅删除旧记录
					decisions[decisionIdx].deleteIds = result.delete_ids;
					decisions[decisionIdx].shouldStore = false;
					console.log(`[编纂者] LLM判定重复: "${candidate.summary.content.slice(0, 30)}..." → 仅删${result.delete_ids.length}条`);
				} else {
					// LLM 判定保留原记录不变 → 回退为直接存储当前摘要
					decisions[decisionIdx].shouldStore = true;
					decisions[decisionIdx].storeContent = this.formatSummaryAsRecord(candidate.summary);
					console.log(`[编纂者] 合并无变化，直接存储: "${candidate.summary.content.slice(0, 30)}..."`);
				}
			}
		}
	}

	/** 执行一批合并项的 LLM 推理（1 次 LLM 调用处理多条） */
	private executeBatchMerge(batch: MergeCandidate[]): MergeResult[] {
		const prompt = this.buildBatchMergePrompt(batch);
		this.coverContext({ role: 'user', content: prompt });
		this.runtimeMessages = [];

		let response: modelResponse;
		try {
			response = this.run([], []);
		} catch (error) {
			console.error('[编纂者] 阶段四批量合并推理失败:', error);
			// 回退：每条都直接存储原摘要
			return batch.map(() => ({ delete_ids: [], merged_content: '' }));
		}

		const content = response.body?.choices?.[0]?.message?.content || '';
		const results = this.parseJsonResponse<MergeResult[]>(content);

		if (!results || !Array.isArray(results)) {
			console.warn('[编纂者] 批量合并结果解析失败，回退为直接存储');
			return batch.map(() => ({ delete_ids: [], merged_content: '' }));
		}

		// 确保结果数量与输入一致
		while (results.length < batch.length) {
			results.push({ delete_ids: [], merged_content: '' });
		}

		return results.slice(0, batch.length);
	}

	/**
	 * 阶段六：记忆库维护 — 写入后去重扫描
	 *
	 * 对本次写入的每条新记录查询记忆库，若发现与已有记录高度重复
	 * （相似度 >= DEDUP_THRESHOLD 且 ID 不同），删除内容较短的那条
	 */
	private deduplicateMemory(): void {
		// 从最近写入的记录中提取内容进行去重检查
		const recentRecords = this.queryExistingRecords('近期对话 重要事件', 15);
		if (recentRecords.length === 0) return;

		const toDelete: string[] = [];

		// 对每条记录，查询是否有高相似度的其他记录
		for (const record of recentRecords) {
			const queryText = record.content.slice(0, 80);
			const matches = this.queryExistingRecords(queryText, 5);
			for (const match of matches) {
				if (match.id === record.id) continue;
				if (match.similarity >= this.DEDUP_THRESHOLD) {
					// 两条高度重复，删除内容较短的那条
					const shorterId = record.content.length <= match.content.length ? record.id : match.id;
					if (!toDelete.includes(shorterId)) {
						toDelete.push(shorterId);
						console.log(`[编纂者] 去重扫描发现重复记录，删除较短项 ${shorterId}（相似度 ${(match.similarity * 100).toFixed(1)}%）`);
					}
				}
			}
		}

		for (const id of toDelete) {
			const trimmedId = id.trim();
			if (!trimmedId) continue;
			const [, error] = memoryDelete('lunar_messages', trimmedId);
			if (error) console.error(`[编纂者] 去重删除 ${trimmedId} 失败:`, error);
			else console.log(`[编纂者] 已去重删除记录 ${trimmedId}`);
		}
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
