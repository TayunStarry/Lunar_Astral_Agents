import { ToolCall } from '../config/tool';
import { GlobalConfig } from '../config/global';
import { MemorySearchHit, ensureMemoryReady } from '../capabilities/memory';

// ==== 记忆检索工具（对话者可用） ====
//
// 与 browse_resources 严格分工：
//   search_memory     检索记忆库中的历史对话/知识片段（谁之前说过什么）
//   browse_resources  检索统一资源目录中的文档/图片/音视频（用户给过的资料）
//
// 设计原则：降低对话者的心智负担——没有模式选择，只有一个关键词数组。
// 检索策略固定为两级：
//   1. 向量语义检索（首选）：对每个关键词做嵌入查询，汇总去重；
//   2. 关键词补足（向量不足 topK 时）：在全部记忆正文里找包含关键词的条目，
//      按命中的关键词数量降序补齐。
// 检索固定返回文本类型记忆（表情包等图像记忆不进入对话记忆检索）。

/** 默认返回条数 */
const DEFAULT_TOP_K = 8;
/** 最大返回条数 */
const MAX_TOP_K = 30;
/** 单条记忆正文的展示上限（避免一次塞爆上下文） */
const CONTENT_PREVIEW_CHARS = 400;
/** 关键词补足阶段最多参与匹配的词数（每个词一次全库扫描，需限流） */
const FALLBACK_MAX_TERMS = 8;

/** 记忆检索工具定义 */
export const memoryTools: ToolCall[] = [
	{
		type: 'function',
		function: {
			name: 'search_memory',
			description: '从长期记忆库检索历史对话与知识片段。请注意：这是聊天记忆检索，不是文档资源检索——查文件/资料请用 browse_resources。',
			parameters: {
				type: 'object',
				properties: {
					keywords: {
						type: 'array',
						items: { type: 'string' },
						description: '用自然语言描述或句柄视频链接或BV号均可，提供 1-4 种不同表述能显著提高命中率'
					},
					exclude: {
						type: 'array',
						items: { type: 'string' },
						description: '结果正文命中任意一个词就丢弃（如 ["表情包", "图片"]），用于排除噪音'
					},
					topK: {
						type: 'number',
						description: `希望返回的条目数，默认 ${DEFAULT_TOP_K}，最大 ${MAX_TOP_K}`
					}
				},
				required: ['keywords']
			}
		}
	}
];

/** JSON 解析容错（模型偶发传入非 JSON 字符串） */
function safeParse(text: string): Record<string, any> {
	try {
		const parsed = JSON.parse(text);
		return parsed && typeof parsed === 'object' ? parsed : {};
	}
	catch {
		return {};
	}
}

/** 角色 → 中文标签 */
function roleLabel(role: string): string {
	switch (role) {
		case 'user': return '用户';
		case 'assistant': return '月华';
		case 'system': return '系统';
		case 'tool': return '工具';
		case 'image': return '图片';
		default: return role || '未知';
	}
}

/** Unix 秒 → 本地时间文本 */
function formatTime(timestamp?: number): string {
	if (!timestamp) return '时间未知';
	try {
		return new Date(timestamp * 1000).toLocaleString('zh-CN', { hour12: false });
	}
	catch {
		return '时间未知';
	}
}

/** 正文截断（保留开头，避免单条记忆撑爆上下文） */
function preview(content: string): string {
	return content.length > CONTENT_PREVIEW_CHARS
		? content.slice(0, CONTENT_PREVIEW_CHARS) + '…'
		: content;
}

/** 规整字符串数组参数（兼容单个字符串） */
function parseTerms(value: unknown): string[] {
	const list = Array.isArray(value) ? value : (typeof value === 'string' ? [value] : []);
	return list.map(item => String(item || '').trim()).filter(Boolean);
}

/** 规整 topK：非法值回落默认，夹紧到 [1, MAX_TOP_K] */
function parseTopK(value: unknown): number {
	const raw = Number(value);
	return Number.isFinite(raw) ? Math.min(Math.max(Math.trunc(raw), 1), MAX_TOP_K) : DEFAULT_TOP_K;
}

/** 正文是否命中任意排除词（空列表视为不排除） */
function containsAny(content: string, terms: string[]): boolean {
	if (terms.length === 0) return false;
	const lower = content.toLowerCase();
	return terms.some(term => lower.includes(term));
}

/** 从查询文本中提取B站视频标识（BV 号）；用于召回已看视频摘要 */
function extractVideoKeys(keywords: string[]): string[] {
	const keys = new Set<string>();
	for (const keyword of keywords) {
		for (const match of String(keyword || '').matchAll(/BV[0-9A-Za-z]{10}/g)) {
			keys.add(match[0]);
		}
	}
	return [...keys];
}

/**
 * 用视频标识在视频摘要库（video_summary 表，键=视频主页 URL）中召回已看视频的理解文本。
 *
 * 月华刷B站时会把每条视频的理解写进该库；查询记忆时带上视频链接或 BV 号，
 * 即可直接取回当时的观看理解，不需要重新看一遍视频。
 */
function findVideoSummaries(keys: string[]): string[] {
	if (!keys.length) return [];
	try {
		const [entries, error] = knowledgeLoad('video_summary');
		if (error || !Array.isArray(entries)) return [];
		const found: string[] = [];
		for (const pair of entries) {
			if (!Array.isArray(pair) || typeof pair[0] !== 'string') continue;
			const key = pair[0];
			const upperKey = key.toUpperCase();
			if (!keys.some(id => upperKey.includes(id.toUpperCase()))) continue;
			const text = typeof pair[1] === 'string' ? pair[1] : String(pair[1] ?? '');
			if (text.trim()) found.push(`[已看视频摘要] ${key}\n${preview(text)}`);
		}
		return found;
	}
	catch (error) {
		console.error('[记忆检索] 视频摘要召回失败:', error);
		return [];
	}
}

/**
 * 向量语义检索：对每个关键词各做一次嵌入查询，按记忆 ID 合并。
 *
 * 每条记忆记录「被几个关键词召回」（hitCount）与「最高相似度」；
 * 输出按最高相似度降序。单条查询失败只跳过该关键词，不中断整体检索。
 */
function vectorSearch(keywords: string[], topK: number, exclude: string[]): MemorySearchHit[] {
	/** id → 命中记录（保留最高相似度）与召回它的关键词数 */
	const merged = new Map<string, { hit: MemorySearchHit; hitCount: number }>();
	for (const keyword of keywords) {
		try {
			const [results, error] = memoryQuery(keyword, 'text', topK, false);
			if (error || !results) continue;
			for (const item of results) {
				const content = String(item.content || '').trim();
				if (!content) continue;
				if (containsAny(content, exclude)) continue;
				const id = String(item.id || '');
				if (!id) continue;
				const hit: MemorySearchHit = {
					id,
					type: String(item.type || 'text'),
					role: String(item.role || 'unknown'),
					content,
					similarity: Number(item.similarity) || 0,
					timestamp: item.timestamp,
					collection: item.collection,
					source: item.source
				};
				const existing = merged.get(id);
				if (existing) {
					existing.hitCount++;
					if (hit.similarity > existing.hit.similarity) existing.hit = hit;
				}
				else {
					merged.set(id, { hit, hitCount: 1 });
				}
			}
		}
		catch (error) {
			console.error('[记忆检索] 向量查询失败:', error);
		}
	}
	return [...merged.values()]
		.sort((a, b) => b.hit.similarity - a.hit.similarity)
		.map(entry => entry.hit);
}

/**
 * 关键词补足：在全部记忆正文里找包含关键词的条目，按命中的关键词数量降序。
 *
 * 每个词一次单词检索（命中该词即算，OR 语义），统计每条记忆命中了几个不同的词；
 * 词数相同再按时间倒序。分词策略：关键词原文过短（≤20 字）时整句参与，
 * 否则空格分词——长句整句做 LIKE 匹配几乎必然落空。
 */
function keywordFallback(keywords: string[], topK: number, exclude: string[], knownIds: Set<string>): MemorySearchHit[] {
	/** 收集补足词：短原文整句 + 长原文分词，去重后限量 */
	const terms: string[] = [];
	const seen = new Set<string>();
	for (const keyword of keywords) {
		const text = String(keyword || '').trim();
		if (!text) continue;
		if (text.length <= 20) {
			if (!seen.has(text)) { seen.add(text); terms.push(text); }
		}
		else {
			for (const part of text.split(/\s+/)) {
				const word = part.trim();
				if (word && word.length >= 2 && !seen.has(word)) { seen.add(word); terms.push(word); }
			}
		}
	}
	if (!terms.length) return [];
	const limited = terms.slice(0, FALLBACK_MAX_TERMS);

	/** id → 命中记录与命中的词数 */
	const merged = new Map<string, { hit: MemorySearchHit; hitCount: number }>();
	for (const term of limited) {
		try {
			const [results, error] = memoryKeywordSearch('text', [term], false, topK * 2);
			if (error || !results) continue;
			for (const item of results) {
				const content = String(item.content || '').trim();
				if (!content) continue;
				if (containsAny(content, exclude)) continue;
				const id = String(item.id || '');
				if (!id || knownIds.has(id)) continue;
				const existing = merged.get(id);
				if (existing) existing.hitCount++;
				else {
					merged.set(id, {
						hit: {
							id,
							type: String(item.type || 'text'),
							role: String(item.role || 'unknown'),
							content,
							similarity: Number(item.similarity) || 0,
							timestamp: item.timestamp,
							collection: item.collection,
							source: item.source
						},
						hitCount: 1
					});
				}
			}
		}
		catch (error) {
			console.error('[记忆检索] 关键词补足失败:', error);
		}
	}
	// 词数多的排前；同词数按时间倒序
	return [...merged.values()]
		.sort((a, b) => b.hitCount - a.hitCount || (b.hit.timestamp ?? 0) - (a.hit.timestamp ?? 0))
		.map(entry => entry.hit);
}

/** 处理记忆检索工具：向量优先 → 关键词补足 → BV 号摘要 → 拼装输出 */
async function handleSearchMemory(args?: Record<string, any> | string): Promise<string[]> {
	const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
	const keywords = parseTerms(parsed.keywords);
	if (!keywords.length) return ['记忆检索失败：keywords 不能为空，请提供至少一个查询词', ''];
	const topK = parseTopK(parsed.topK);
	const exclude = parseTerms(parsed.exclude);

	if (!ensureMemoryReady()) {
		return ['记忆库未就绪，无法检索。请稍后再试一次。', ''];
	}

	// 1. 向量语义检索（首选）
	const vectorHits = vectorSearch(keywords, topK, exclude);
	// 2. 向量结果不足 topK 时，用关键词匹配补足（按命中词数排序）
	const knownIds = new Set(vectorHits.map(hit => hit.id));
	const fallbackHits = vectorHits.length >= topK
		? []
		: keywordFallback(keywords, topK, exclude, knownIds);
	const hits = [...vectorHits, ...fallbackHits].slice(0, topK);
	// 3. 查询里带视频链接/BV 号时，顺带召回视频摘要库中的已看视频理解
	const videoSummaries = findVideoSummaries(extractVideoKeys(keywords));

	if (hits.length === 0 && videoSummaries.length === 0) {
		return ['记忆库中没有检索到相关记录。可以换一种表述（如具体的人名、事件名）再试一次；不要编造记忆内容。', ''];
	}

	const sections: string[] = [];
	if (hits.length > 0) {
		const lines = hits.map((hit, index) => {
			const meta = `角色=${roleLabel(hit.role)}｜时间=${formatTime(hit.timestamp)}｜相关度=${hit.similarity.toFixed(3)}`;
			return `[${index + 1}] ${meta}\n${preview(hit.content)}`;
		});
		const viaFallback = fallbackHits.length > 0 && vectorHits.length < topK
			? `（语义检索 ${vectorHits.length} 条 + 关键词补足 ${fallbackHits.length} 条）`
			: '';
		sections.push(`检索到 ${hits.length} 条相关记忆${viaFallback}：\n${lines.join('\n\n')}`);
	}
	if (videoSummaries.length > 0) {
		sections.push(`已看视频摘要 ${videoSummaries.length} 条（月华自己刷B站时留下的观看理解）：\n${videoSummaries.join('\n\n')}`);
	}

	const text = sections.join('\n\n')
		+ '\n\n只依据以上原文回答；原文不足以回答用户问题时，直接说明没有找到相关信息，不要编造或把无关内容当答案。';
	return [text, ''];
}

// ==== 模块级注册 ====

GlobalConfig.LTPfunction.set('search_memory', handleSearchMemory);
