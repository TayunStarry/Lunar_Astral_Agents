import { ToolCall } from '../config/tool';
import { GlobalConfig } from '../config/global';
import { MemorySearchMode, searchMemoryGlobal } from '../capabilities/memory';

// ==== 记忆检索工具（对话者可用） ====
//
// 与 search_resources 严格分工：
//   search_memory     检索记忆库中的历史对话/知识片段（谁之前说过什么）
//   search_resources  检索统一资源目录中的文档/图片/音视频（用户给过的资料）
//
// 集合清单属于底层实现，不暴露给模型：工具只接收「查什么、怎么查、过滤什么、要几条」。
// 三种检索方式：
//   vector  向量语义检索（默认，适合模糊描述）
//   keyword 关键词精确匹配（空格分词、全部命中，适合人名/原话/专有名词）
//   regex   正则匹配（RE2 语法，适合精确模式）

/** 默认返回条数 */
const DEFAULT_LIMIT = 8;
/** 最大返回条数 */
const MAX_LIMIT = 30;
/** 单条记忆正文的展示上限（避免一次塞爆上下文） */
const CONTENT_PREVIEW_CHARS = 400;

/** 检索方式 → 中文标签 */
const MODE_LABELS: Record<MemorySearchMode, string> = {
	vector: '语义检索',
	keyword: '关键词检索',
	regex: '正则检索'
};

/** 记忆检索工具定义 */
export const memoryTools: ToolCall[] = [
	{
		type: 'function',
		function: {
			name: 'search_memory',
			description: '从长期记忆库检索历史对话与知识片段。用于回答「某用户之前说过什么」「以前聊过什么」「你还记不记得」这类问题。三种方式：vector（默认，语义检索，适合模糊描述）、keyword（关键词精确匹配，空格分词且全部命中，适合人名/原话/专有名词）、regex（正则匹配，RE2 语法）。filter 为必须包含的词（如说话人昵称），exclude 为必须排除的词。注意：这是聊天记忆检索，不是文档资源检索——查文件/资料请用 search_resources。',
			parameters: {
				type: 'object',
				properties: {
					query: {
						type: 'string',
						description: '正向检索词：vector 模式写自然语言描述；keyword 模式写空格分隔的关键词（全部命中）；regex 模式写正则表达式'
					},
					mode: {
						type: 'string',
						enum: ['vector', 'keyword', 'regex'],
						description: '检索方式，默认 vector。精确找人名/原话用 keyword；需要模式匹配时才用 regex'
					},
					filter: {
						type: 'array',
						items: { type: 'string' },
						description: '可选：结果正文必须全部包含这些词（如 ["钛宇星光阁"]），用于锁定说话人或主题'
					},
					exclude: {
						type: 'array',
						items: { type: 'string' },
						description: '可选：结果正文命中任意一个词就丢弃（如 ["表情包", "图片"]），用于排除噪音'
					},
					limit: {
						type: 'number',
						description: `返回条数上限，默认 ${DEFAULT_LIMIT}，最大 ${MAX_LIMIT}`
					}
				},
				required: ['query']
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

/** 规整检索方式 */
function parseMode(value: unknown): MemorySearchMode {
	const mode = String(value || '').trim().toLowerCase();
	return mode === 'keyword' || mode === 'regex' ? mode : 'vector';
}

/** 处理记忆检索工具：按指定方式检索 → 过滤 → 返回带元信息的原文 */
async function handleSearchMemory(args?: Record<string, any> | string): Promise<string[]> {
	const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
	const query = String(parsed.query || '').trim();
	if (!query) return ['记忆检索失败：query 不能为空', ''];

	const mode = parseMode(parsed.mode);
	// 正则模式先在 JS 侧校验一次：Go 侧编译失败只会返回空结果，模型看不到具体原因
	if (mode === 'regex') {
		try {
			new RegExp(query, 'i');
		}
		catch (error) {
			return [`记忆检索失败：正则表达式无效（${String(error)}）。可改用 keyword 模式或修正表达式`, ''];
		}
	}

	const rawLimit = Number(parsed.limit);
	const limit = Number.isFinite(rawLimit)
		? Math.min(Math.max(Math.trunc(rawLimit), 1), MAX_LIMIT)
		: DEFAULT_LIMIT;
	const filters = parseTerms(parsed.filter);
	const exclude = parseTerms(parsed.exclude);

	const hits = searchMemoryGlobal(query, limit, { mode, filters, exclude });
	if (hits.length === 0) {
		const hint = mode === 'vector'
			? '可以改用 mode=keyword 并只保留人名/主题词再试一次'
			: '可以放宽 filter/exclude，或改用默认的 vector 模式再试一次';
		return [`记忆库中没有检索到相关记录。${hint}；不要编造记忆内容。`, ''];
	}

	const lines = hits.map((hit, index) => {
		const meta = [
			`角色=${roleLabel(hit.role)}`,
			`时间=${formatTime(hit.timestamp)}`,
			// 向量模式展示相关度；关键词/正则模式没有相似度，改为标明检索方式
			mode === 'vector' ? `相关度=${hit.similarity.toFixed(3)}` : `方式=${MODE_LABELS[mode]}`
		].join('｜');
		return `[${index + 1}] ${meta}\n${preview(hit.content)}`;
	});

	const text = `以${MODE_LABELS[mode]}检索到 ${hits.length} 条相关记忆：`
		+ `\n${lines.join('\n\n')}`
		+ '\n\n只依据以上原文回答；原文不足以回答用户问题时，直接说明没有找到相关信息，不要编造或把无关内容当答案。';
	return [text, ''];
}

// ==== 模块级注册 ====

GlobalConfig.LTPfunction.set('search_memory', handleSearchMemory);
