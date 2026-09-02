import { GlobalConfig, ModelBuilder, modelResponse, SCHEDULE_TRIGGER_PREFIX } from '../../index';
import { ensureMemoryReady, extractTextFromMessage } from '../capabilities/memory';

/** 长期记忆集合名 */
const MEMORY_COLLECTION = 'lunar_messages';
/** RAG 摘要硬切断长度上限 */
const RAG_SUMMARY_HARD_LIMIT = 4096;
/** 每条用户消息查询记忆库时返回的相关度最高的记录数 */
const RAG_PER_QUERY_TOP_K = 10;
/** 去重后送入摘要的检索记录条数上限 */
const RAG_MAX_RECORDS = 32;

/** 记忆检索记录（与 memoryQuery 返回结构对齐） */
interface RagRecord {
	id: string;
	role: string;
	content?: string;
	image?: string;
	similarity: number;
}

/**
 * 记忆者智能体
 *
 * 承接两条记忆职责，替代以往把碎片直接塞入对话者 rag 数组的做法：
 *  1. 记忆写入（ingress）：把对话者淘汰的缓冲消息逐个整理后写入长期记忆库。
 *  2. 记忆检索摘要（egress）：继承原有搜索机制检索长期记忆，对命中的碎片做一次
 *     总结与摘要，输出一篇硬切断 4096 的连贯摘要；该摘要不再回写记忆库。
 */
export class MemorizerRole extends ModelBuilder {
	constructor() {
		super(fileView('prompts/memorizerRole.md')[0]);
	}

	/** 将缓冲的未读记录逐个写入记忆库（自 memorizeUnreadRecords 迁移） */
	public persistUnreadRecords(): void {
		// 缓冲池为空时跳过
		if (GlobalConfig.unreadRecords.length === 0) return;
		// 记忆库未就绪时保留缓冲消息，等待下次触发
		if (!ensureMemoryReady()) {
			console.warn('[记忆] 记忆库未就绪，保留缓冲消息待下次触发');
			return;
		}
		/** 成功写入的消息数量 */
		let written = 0;
		for (const message of GlobalConfig.unreadRecords) {
			// 过滤工具调用消息
			if (message.role === 'tool') continue;
			/** 提取文本内容并过滤空字符串 */
			const content = extractTextFromMessage(message).trim();
			// 过滤空字符串、长度小于等于5的消息
			if (!content || content.length <= 5) continue;
			// 过滤计划表触发的自动消息（统一前缀），避免写入长期记忆
			if (content.includes(SCHEDULE_TRIGGER_PREFIX)) continue;
			/** 逐个写入记忆库 */
			const [, error] = memoryAdd(MEMORY_COLLECTION, message.role, content);
			if (error) console.error('[记忆] 写入记忆库失败:', error);
			else written++;
		}
		console.log(`[记忆] 已写入 ${written} 条消息到记忆库`);
		// 清空消息缓冲池
		GlobalConfig.unreadRecords = [];
	}

	/**
	 * 检索长期记忆并生成摘要
	 *
	 * 继承原有搜索机制，对命中的碎片做一次总结与摘要，返回硬切断 4096 的连贯摘要；
	 * 无结果、记忆库未就绪或摘要失败时返回空字符串。
	 *
	 * @param userMessages 最新的用户消息（作为检索查询条件）
	 * @returns 连贯摘要文本（≤4096 字），失败时为空字符串
	 */
	public queryRagSummary(userMessages: string[]): string {
		if (!userMessages || userMessages.length === 0) return '';
		if (!ensureMemoryReady()) return '';
		// 搜索结果
		const records = this.retrieveRagRecords(userMessages);
		if (records.length === 0) {
			console.log('[记忆] 检索未命中任何相关记录');
			return '';
		}
		// 总结与摘要
		return this.summarizeRecords(records);
	}

	/** 检索长期记忆（沿用对话者原检索逻辑：多查询合并→按内容去重→按相似度降序→取前32条） */
	private retrieveRagRecords(userMessages: string[]): RagRecord[] {
		/** 所有查询结果汇总（含相似度分数） */
		const allResults: RagRecord[] = [];
		for (const userMessage of userMessages) {
			const [results, error] = memoryQuery(MEMORY_COLLECTION, userMessage, RAG_PER_QUERY_TOP_K);
			// 单条查询失败则跳过，继续处理下一条
			if (error) {
				console.error('记忆库查询失败:', error);
				continue;
			}
			if (results && results.length > 0) allResults.push(...results);
		}
		/** 基于内容去重，保留相似度最高的记录 */
		const seen = new Map<string, RagRecord>();
		for (const r of allResults) {
			const content = r.content || '';
			const existing = seen.get(content);
			if (!existing || r.similarity > existing.similarity) seen.set(content, r);
		}
		// 按相似度降序排列，相关度最高的在最前
		const uniqueResults = Array.from(seen.values()).sort((a, b) => b.similarity - a.similarity);
		console.log(`[记忆] 检索到 ${uniqueResults.length} 条相关记录，相似度范围: ${uniqueResults[0]?.similarity?.toFixed(4) ?? 'N/A'} ~ ${uniqueResults[uniqueResults.length - 1]?.similarity?.toFixed(4) ?? 'N/A'}`);
		return uniqueResults.slice(0, RAG_MAX_RECORDS);
	}

	/** 交给 LLM 对命中的内容碎片做总结与摘要，输出硬切断 4096 的连贯摘要 */
	private summarizeRecords(records: RagRecord[]): string {
		/** 摘要输入：标注角色 + 内容片段 */
		const fragments = records
			.map((r, i) => `--- 片段 ${i + 1}（${r.role}）---\n${r.content || '(空)'}`)
			.join('\n\n');

		// 覆写为本次独立摘要任务（逐次覆盖，不累积历史）
		this.coverContext({ role: 'user', content: `请整理以下从长期记忆检索到的内容碎片，输出一篇连贯的中文摘要。\n\n${fragments}` });
		this.runtimeMessages = [];

		/** 模型响应 */
		let response: modelResponse;
		try {
			response = this.run([], []);
		} catch (error) {
			console.error('[记忆] 摘要推理失败:', error);
			return '';
		}
		/** 摘要文本 */
		let digest = (response.body?.choices?.[0]?.message?.content || '').trim();
		// 长度硬切断 4096
		if (digest.length > RAG_SUMMARY_HARD_LIMIT) digest = digest.slice(0, RAG_SUMMARY_HARD_LIMIT);
		console.log(`[记忆] 已生成摘要（${digest.length}/${RAG_SUMMARY_HARD_LIMIT} 字）`);
		return digest;
	}
}