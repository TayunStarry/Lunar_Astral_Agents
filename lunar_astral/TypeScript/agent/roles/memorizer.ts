import { GlobalConfig } from '../../config/global';
import { ModelBuilder } from '../base/builder';
import { modelResponse } from '../../config/model';
import { SCHEDULE_TRIGGER_PREFIX } from '../../tool/schedule-defs';
import { ensureMemoryReady, extractTextFromMessage } from '../capabilities/memory';
import { interactEvent } from '../capabilities/ltp-event';

/** 长期记忆集合名 */
const MEMORY_COLLECTION = 'lunar_messages';
/** RAG 摘要硬切断长度上限 */
const RAG_SUMMARY_HARD_LIMIT = 4096;
/** 记忆时间线硬切断长度上限（按行移除超限行） */
const RAG_TIMELINE_HARD_LIMIT = 2048;
/** 每条用户消息查询记忆库时返回的相关度最高的记录数 */
const RAG_PER_QUERY_TOP_K = 10;
/** 去重后送入摘要的检索记录条数上限 */
const RAG_MAX_RECORDS = 24;

/** 记忆检索记录（与 memoryQuery 返回结构对齐；图片记忆取 base64，文本记忆取 content） */
interface RagRecord {
	/** 记忆记录唯一标识 */
	id: string;
	/** 记忆记录角色 */
	role: string;
	/** 记忆记录内容 */
	content?: string;
	/** 记忆记录图片（base64） */
	base64?: string;
	/** 记忆记录相似度 */
	similarity: number;
	/** 入库时间 Unix 秒级时间戳（旧数据无此字段） */
	timestamp?: number;
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
	 * 检索长期记忆并生成记忆上下文
	 *
	 * 继承原有搜索机制检索长期记忆，随后按模式分派：
	 *  - summarize 为 true：对命中的碎片做一次总结与摘要，输出硬切断 4096 的连贯摘要；
	 *  - summarize 为 false：按时间顺序将命中记录拼接为时间线文本，硬切断 2048（按行移除超限行）。
	 * 检索结果为空、记忆库未就绪或整理失败时返回空字符串。
	 *
	 * @param userMessages 未读用户消息（作为检索查询条件）
	 * @param summarize 是否调用 LLM 整理摘要（深度回忆意图命中时为 true）
	 * @returns 记忆文本，检索为空或失败时为空字符串
	 */
	public queryRagDigest(userMessages: string[], summarize: boolean): string {
		// 条件不满足时跳过
		if (!userMessages || userMessages.length === 0 || !ensureMemoryReady()) return '';
		// 搜索结果
		let records = this.retrieveRagRecords(userMessages);
		// 检索结果为空时跳过
		if (records.length === 0) {
			console.log('[记忆] 检索未命中任何相关记录');
			return '';
		}
		// 事件 -> 构建记忆前：推送检索记录与用户消息，插件可改写后再整理
		const feedback: RagRecord[] = interactEvent('build_memory_before', { userMessages, records }).return;
		// 如果插件返回了非空的检索记录则直接使用（空数组 every 恒真，需显式排除，避免用空记录做无意义的整理）
		if (feedback && Array.isArray(feedback) && feedback.length > 0 && feedback.every(r => r.id && r.role && r.content && r.similarity !== undefined)) {
			records = feedback;
		}
		// 深度回忆意图命中时由 LLM 总结与摘要，否则默认按时间顺序拼接时间线
		return summarize ? this.summarizeRecords(records) : this.buildTimeline(records);
	}

	/** 按入库时间升序将检索记录拼接为时间线文本，硬切断 2048（到达字数上限即移除多余的行） */
	private buildTimeline(records: RagRecord[]): string {
		// 按入库时间升序排列（无时间戳的旧数据排最前）
		const sorted = [...records].sort((a, b) => (a.timestamp ?? 0) - (b.timestamp ?? 0));
		/** 时间线文本 */
		let timeline = '';
		for (const r of sorted) {
			/** 记录时间标注（旧数据无时间戳则标注未知） */
			const time = r.timestamp ? new Date(r.timestamp * 1000).toLocaleString('zh-CN', { hour12: false }) : '未知';
			/** 单行记录 */
			const line = `[ 时间: ${time} ] [ 内容: ${r.content || '(空)'} ]`;
			// 到达字数上限即停止拼接，移除多余的行
			if (timeline.length + line.length + 1 > RAG_TIMELINE_HARD_LIMIT) break;
			timeline += (timeline ? '\n' : '') + line;
		}
		console.log(`[记忆] 已生成时间线（${timeline.length}/${RAG_TIMELINE_HARD_LIMIT} 字，${timeline.split('\n').length} 行）`);
		return timeline;
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

	/** 记忆摘要任务模板（占位符 {fragments} 由检索片段填充） */
	private readonly summaryTaskTemplate = fileView('prompts/memorizerSummaryTask.md')[0];

	/** 交给 LLM 对命中的内容碎片做总结与摘要，输出硬切断 4096 的连贯摘要 */
	private summarizeRecords(records: RagRecord[]): string {
		/** 摘要输入：标注角色 + 入库时间 + 内容片段（时间供整理记忆时参考，旧数据无时间戳则省略） */
		const fragments = records
			.map((r, i) => {
				/** 记录头部标注 */
				let header = `--- 片段 ${i + 1}（${r.role}`;
				if (r.timestamp) header += `，${new Date(r.timestamp * 1000).toLocaleString('zh-CN', { hour12: false })}`;
				header += `）---`;
				return `${header}\n${r.content || '(空)'}`;
			})
			.join('\n\n');

		// 覆写为本次独立摘要任务（逐次覆盖，不累积历史）
		this.coverContext({ role: 'user', content: this.summaryTaskTemplate.replace('{fragments}', fragments) });
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