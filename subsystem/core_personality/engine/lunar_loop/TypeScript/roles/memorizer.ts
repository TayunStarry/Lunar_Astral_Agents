import { GlobalConfig } from '../config/global';
import { SCHEDULE_TRIGGER_PREFIX } from '../tools/schedule/defs';
import {
	ensureMemoryReady,
	extractMemoryText,
	LUNAR_MESSAGES_COLLECTION,
	searchMemoryGlobal
} from '../capabilities/memory';
import { interactEvent } from '../capabilities/ltp-event';

/** 长期记忆集合名 */
const MEMORY_COLLECTION = LUNAR_MESSAGES_COLLECTION;
/** 记忆时间线硬切断长度上限（按行移除超限行） */
const RAG_TIMELINE_HARD_LIMIT = 2048;
/** 每条用户消息查询记忆库时返回的相关度最高的记录数 */
const RAG_PER_QUERY_TOP_K = 10;
/** 去重后送入时间线的检索记录条数上限 */
const RAG_MAX_RECORDS = 24;
/** 单轮最多执行的检索查询数（跨集合检索每条查询都会调用嵌入服务，需限流） */
const RAG_MAX_QUERIES = 4;
/**
 * 进入自动记忆的最低相似度门槛。
 *
 * Go 侧已过滤 <0.30 的候选；这里再抬到 0.40，避免把「语义上勉强相似、
 * 但实际回答不了问题」的碎片注入对话上下文。
 */
const RAG_MIN_SIMILARITY = 0.40;

/** 记忆检索记录（与 memoryQuery 返回结构对齐） */
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
	/** 命中的记忆集合名 */
	collection?: string;
}

/**
 * 去掉群聊/用户前缀，保留实际发言文本作为检索词。
 *
 * 记忆里的用户消息形如 `[群聊: …][用户: 某某]: 正文`；整串直接做嵌入会把
 * 前缀当成语义的一部分，正文关键词反而被稀释。
 */
function memoryQueryText(message: string): string {
	const text = String(message || '').trim();
	const stripped = text.replace(/^(?:\s*\[[^\]]*\])+\s*[:：]?\s*/u, '').trim();
	return stripped || text;
}

/**
 * 记忆者
 *
 * 主动检索已交给对话者的 `search_memory` 工具；本类只保留两项**不依赖 LLM** 的职责：
 *  1. 记忆写入：把对话者淘汰的缓冲消息整理后写入长期记忆库；
 *  2. 自动背景：每轮跨集合检索，并按时间顺序拼接为时间线注入对话上下文。
 *
 * 不再调用 LLM 做摘要——检索不准时，摘要只会把无关碎片写成一段看似有用的噪音；
 * 需要更多信息时，对话者可以自己调用 `search_memory` 定向查询原文。
 */
export class MemorizerRole {
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
			/** 提取文本内容并过滤空字符串（图片标签已消解，避免重启后错取图片） */
			const content = extractMemoryText(message).trim();
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
	 * 检索长期记忆并生成时间线上下文。
	 *
	 * 跨集合检索 → 相似度过滤 → 按内容去重 → 按入库时间升序拼接为时间线，
	 * 硬切断 2048（按行移除超限行）。检索结果为空或记忆库未就绪时返回空字符串。
	 *
	 * @param userMessages 未读用户消息（作为检索查询条件）
	 * @returns 记忆时间线文本，检索为空或失败时为空字符串
	 */
	public queryRagDigest(userMessages: string[]): string {
		// 条件不满足时跳过
		if (!userMessages || userMessages.length === 0 || !ensureMemoryReady()) return '';
		/** 检索结果 */
		let records = this.retrieveRagRecords(userMessages);
		// 检索结果为空时跳过
		if (records.length === 0) return '';
		// 事件 -> 构建记忆前：推送检索记录与用户消息，插件可改写后再整理
		const feedback: RagRecord[] = interactEvent('build_memory_before', { userMessages, records }).return;
		// 如果插件返回了非空的检索记录则直接使用（空数组 every 恒真，需显式排除）
		if (feedback && Array.isArray(feedback) && feedback.length > 0 && feedback.every(r => r.id && r.role && r.content && r.similarity !== undefined)) {
			records = feedback;
		}
		// 按时间顺序拼接时间线
		return this.buildTimeline(records);
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

	/** 跨集合检索长期记忆（按内容去重 → 相似度降序 → 取前若干条） */
	private retrieveRagRecords(userMessages: string[]): RagRecord[] {
		// 去重 + 去掉群聊前缀 + 限制查询条数：每条查询都会触发嵌入调用，不能随消息数量无限增长
		const queries = [...new Set(userMessages.map(text => memoryQueryText(text)).filter(text => text.length > 0))].slice(-RAG_MAX_QUERIES);
		/** 所有查询结果汇总（含相似度分数） */
		const allResults: RagRecord[] = [];
		for (const query of queries) {
			// 跨集合检索：同时覆盖聊天记忆与其它对话类集合，而不是只在 lunar_messages 里碰运气
			for (const hit of searchMemoryGlobal(query, RAG_PER_QUERY_TOP_K)) {
				// 弱命中不进入时间线：它们只会污染上下文
				if (hit.similarity < RAG_MIN_SIMILARITY) continue;
				allResults.push({
					id: hit.id,
					role: hit.role,
					content: hit.content,
					similarity: hit.similarity,
					timestamp: hit.timestamp,
					collection: hit.collection
				});
			}
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
		console.log(`[记忆] 跨集合检索到 ${uniqueResults.length} 条相关记录，相似度范围: ${uniqueResults[0]?.similarity?.toFixed(4) ?? 'N/A'} ~ ${uniqueResults[uniqueResults.length - 1]?.similarity?.toFixed(4) ?? 'N/A'}`);
		return uniqueResults.slice(0, RAG_MAX_RECORDS);
	}
}
