import { ToolCall } from '../config/tool';
import { GlobalConfig } from '../config/global';
import { ResourceEntry, ResourceSection } from '../config/resource';
import { KIND_LABELS, formatHandle, grammarCheatsheet, parseHandle, sectionHandle } from '../capabilities/handle';
import {
	deleteResource,
	getResourceEntry,
	listResources,
	readResource,
	resourceStatistics,
	searchResources,
	LOST_FILE_NOTICE,
	HANDLE_INVALID_NOTICE
} from '../capabilities/resource';
import { describeResource } from '../capabilities/describe';

// ===== 统一资源工具（对话者可用） =====
//
// 全模态统一入口：文本 / 文件 / 图片 / 视频 / 音频共用同一套操作与同一套句柄文法。
//   list_resources   列出目录（句柄 + 类型 + 标题/描述 + 内容状态）
//   read_resource    按句柄整段展开正文（文档）或取回描述（图片/视频/音频）
//   outline_resource 取文档大纲（每节带 |§n 锚点），随后按需定向读取
//   search_resources 跨资源路由（定位到哪份资源的哪一节），再用 read_resource 取正文
//
// 命中路径刻意设计为「先结构定位、再整段展开」：不再有相似度 topK 拼接，
// 半句截断与碎片拼接由段落边界保证消除。

/** 单次读取返回的字符上限（与 Go 侧 MaxReadBytes 同量级，避免一次读入撑爆上下文） */
const READ_CHAR_BUDGET = 12000;
/** 落选资源的展示条数上限 */
const OUTLINE_PREVIEW_LIMIT = 60;

/** 资源类型码的中文标签 */
function kindLabel(kind: string): string {
	return KIND_LABELS[kind as keyof typeof KIND_LABELS] ?? kind;
}

/** 内容状态说明（blob 丢失的条目会在列出时被清理，正常不会出现在列表里） */
function stateNote(entry: ResourceEntry): string {
	return entry.state === 'active' ? '' : ` [${LOST_FILE_NOTICE}]`;
}

/** 资源目录条目的单行展示 */
function entryLine(entry: ResourceEntry): string {
	const desc = entry.summary ? `：${entry.summary}` : '';
	const sectionNote = entry.section_count > 0 ? ` ${entry.section_count}节` : '';
	return `${entry.handle} [${kindLabel(entry.kind)}] ${entry.title || entry.name}${sectionNote}${stateNote(entry)}${desc}`;
}

/** 处理资源目录工具：按类型列出可用资源 */
async function handleListResources(args?: Record<string, any> | string): Promise<string[]> {
	const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
	const kind = String(parsed.kind || '').trim().toUpperCase();
	const limit = Math.min(Math.max(Number(parsed.limit) || 30, 1), 200);
	if (kind && !KIND_LABELS[kind as keyof typeof KIND_LABELS]) {
		return [`类型码无效：${kind}。可用类型码：D 文档 / I 图片 / V 视频 / A 音频 / T 表格`, ''];
	}
	const entries = listResources(kind, limit);
	// 失效句柄清理：本体已丢失/损坏的条目不滞留目录，列出时顺带移除
	const valid: ResourceEntry[] = [];
	let purged = 0;
	for (const entry of entries) {
		if (entry.state === 'lost') {
			if (deleteResource(entry.handle)) purged++;
			continue;
		}
		valid.push(entry);
	}
	if (valid.length === 0) {
		const purgedNote = purged > 0 ? `（已自动清理 ${purged} 条源文件丢失的失效句柄）` : '';
		return [`资源目录为空：当前没有已收录的资源${purgedNote}。用户可以上传图片、发送文本文件，或让我生成图片后再引用`, ''];
	}
	const stats = resourceStatistics();
	const lines = valid.map(entryLine);
	const purgeNote = purged > 0 ? `；已自动清理 ${purged} 条源文件丢失的失效句柄` : '';
	const head = `当前可用资源（共 ${valid.length} 条，按收录时间倒序，越靠前越新${purgeNote}${stats ? `；目录总计 ${stats.total} 条 / ${formatBytes(stats.bytes)}` : ''}）：`;
	const tail = `\n${grammarCheatsheet()}。需要正文时用 read_resource 按句柄取回；文档可先用 outline_resource 看结构再定向读取。`;
	return [`${head}\n${lines.join('\n')}${tail}`, ''];
}

/** 处理读取工具：按句柄整段展开正文或取回描述 */
async function handleReadResource(args?: Record<string, any> | string): Promise<string[]> {
	const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
	const rawHandles: unknown[] = Array.isArray(parsed.handles)
		? parsed.handles
		: (parsed.handle ? [parsed.handle] : []);
	if (rawHandles.length === 0) {
		return ['请提供资源句柄，例如 ["@D.003"]。可用清单见 list_resources', ''];
	}
	const question = String(parsed.question || '').trim().slice(0, 200);
	const blocks: string[] = [];
	const missing: string[] = [];

	for (const raw of rawHandles) {
		const handle = String(raw || '').trim();
		const parsedHandle = parseHandle(handle);
		if (!parsedHandle) {
			missing.push(handle || '(空)');
			continue;
		}
		const normalized = formatHandle(parsedHandle);
		const entry = getResourceEntry(normalized);

		// 非文本资源（图片/视频/音频）：返回描述，不注入本体
		if (parsedHandle.kind === 'I' || parsedHandle.kind === 'V' || parsedHandle.kind === 'A') {
			if (!entry) {
				missing.push(normalized);
				continue;
			}
			// 本体已丢失/损坏：移除失效句柄并返回固定提示，不再降级为描述
			if (entry.state === 'lost') {
				deleteResource(normalized);
				blocks.push(`【${normalized}】\n${LOST_FILE_NOTICE}`);
				continue;
			}
			const desc = describeResource(normalized, question);
			blocks.push(`【${normalized}｜${kindLabel(parsedHandle.kind)}｜${entry.title || entry.name}】\n${desc || '（暂无描述，且本体不可直接解读）'}`);
			continue;
		}

		// 文本资源：按锚点整段展开
		const result = readResource(normalized, READ_CHAR_BUDGET);
		if (!result) {
			missing.push(normalized);
			continue;
		}
		if (result.lost) {
			// 本体已丢失/损坏：readResource 已顺带移除句柄条目，这里幂等兜底并返回固定提示
			deleteResource(normalized);
			blocks.push(`【${normalized}｜${result.title}】\n${LOST_FILE_NOTICE}`);
			continue;
		}
		const rangeNote = result.line_start && result.line_end ? `｜第 ${result.line_start}-${result.line_end} 行` : '';
		const sectionNote = result.section_title ? `｜${result.section_title}` : '';
		const truncated = result.truncated ? '\n（内容较长已截断，可用 outline_resource 查看结构后按节定向读取）' : '';
		blocks.push(`【${normalized}${sectionNote}${rangeNote}｜共 ${result.total_lines || 0} 行】\n${result.content || ''}${truncated}`);
	}

	if (blocks.length === 0) {
		return [`取回失败：${missing.join('、')} —— ${HANDLE_INVALID_NOTICE}。请先用 list_resources 确认可用句柄`, ''];
	}
	let text = blocks.join('\n\n---\n\n');
	if (missing.length > 0) text += `\n\n以下句柄无法取回：${missing.join('、')}（${HANDLE_INVALID_NOTICE}）`;
	return [text, ''];
}

/** 处理大纲工具：返回文档结构，每节附规范锚点句柄 */
async function handleOutlineResource(args?: Record<string, any> | string): Promise<string[]> {
	const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
	const handle = String(parsed.handle || '').trim();
	const parsedHandle = parseHandle(handle);
	if (!parsedHandle) return ['请提供文档句柄，例如 "@D.003"', ''];
	const normalized = formatHandle(parsedHandle);

	const entry = getResourceEntry(normalized);
	if (!entry) return [`【${normalized}】\n${HANDLE_INVALID_NOTICE}。请用 list_resources 确认可用句柄`, ''];
	// 本体已丢失/损坏：移除失效句柄并返回固定提示，不滞留
	if (entry.state === 'lost') {
		deleteResource(normalized);
		return [`【${normalized}｜${entry.title || entry.name}】\n${LOST_FILE_NOTICE}`, ''];
	}
	if (entry.kind !== 'D' && entry.kind !== 'T') {
		return [`${normalized} 是${kindLabel(entry.kind)}，没有段落大纲；图片/视频/音频请用 read_resource 取描述`, ''];
	}

	// 大纲走 Go 侧段落表（缺失且在位时现场补建）
	const before = listResources(entry.kind, 1000).find(item => item.seq === entry.seq);
	const sections = outlineFor(normalized);
	if (sections.length === 0) {
		return [`${normalized}（${entry.title || entry.name}）没有可用的段落结构，可用 read_resource 直接读取全文`, ''];
	}
	const lines = sections.slice(0, OUTLINE_PREVIEW_LIMIT).map(section => {
		const anchor = sectionHandle('D', entry.seq, section.ordinal);
		const label = section.path || section.title || `第 ${section.ordinal} 段`;
		const level = section.level > 0 ? `${'#'.repeat(section.level)} ` : '';
		return `${anchor} ${level}${label}（第 ${section.line_start}-${section.line_end} 行，${section.chars} 字）`;
	});
	const head = `【${normalized}｜${entry.title || entry.name}｜共 ${before?.section_count ?? sections.length} 节${before?.state === 'active' ? '' : '，正文已回收'}】`;
	const tail = sections.length > OUTLINE_PREVIEW_LIMIT ? `\n（仅显示前 ${OUTLINE_PREVIEW_LIMIT} 节，共 ${sections.length} 节）` : '';
	return [`${head}\n${lines.join('\n')}${tail}\n\n读取某一节：read_resource({"handles":["${sectionHandle('D', entry.seq, sections[0].ordinal)}"]})`, ''];
}

/** 处理跨资源搜索工具：只做路由定位，正文交给 read_resource */
async function handleSearchResources(args?: Record<string, any> | string): Promise<string[]> {
	const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
	const query = String(parsed.query || '').trim();
	if (!query) return ['请提供搜索关键词', ''];
	const kind = String(parsed.kind || '').trim().toUpperCase();
	const limit = Math.min(Math.max(Number(parsed.limit) || 10, 1), 50);

	const hits = searchResources(query, kind, limit);
	if (hits.length === 0) {
		return [`未找到与「${query}」相关的资源。可用 list_resources 查看全部资源，或让用户重新提供资料`, ''];
	}
	const lines: string[] = [];
	for (const hit of hits) {
		if (hit.sections && hit.sections.length > 0) {
			const sectionLines = hit.sections.slice(0, 5).map(section => {
				const parsed = parseHandle(hit.handle);
				const anchor = parsed ? sectionHandle(parsed.kind, parsed.seq, section.ordinal) : hit.handle;
				return `    ${anchor} ${section.path || section.title || `第 ${section.ordinal} 段`}（第 ${section.line_start}-${section.line_end} 行）`;
			});
			lines.push(`${hit.handle} [${kindLabel(hit.kind)}] ${hit.title}${hit.summary ? `：${hit.summary}` : ''}\n  命中章节：\n${sectionLines.join('\n')}`);
		}
		else {
			lines.push(`${hit.handle} [${kindLabel(hit.kind)}] ${hit.title}${hit.summary ? `：${hit.summary}` : ''}`);
		}
	}
	return [`与「${query}」相关的资源（共 ${hits.length} 条）：\n${lines.join('\n')}\n\n下一步：用 read_resource 按上述句柄取回正文`, ''];
}

/** 取资源大纲（内部封装，失败返回空数组） */
function outlineFor(handle: string): ResourceSection[] {
	try {
		const [sections, , error] = resourceOutline(handle);
		if (error || !sections) return [];
		return sections;
	}
	catch (error) {
		console.error('[资源工具] 读大纲失败:', error);
		return [];
	}
}

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

/** 字节数格式化（展示用） */
function formatBytes(bytes: number): string {
	if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
	const units = ['B', 'KB', 'MB', 'GB'];
	let value = bytes;
	let unit = 0;
	while (value >= 1024 && unit < units.length - 1) {
		value /= 1024;
		unit++;
	}
	return `${value >= 100 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

/** 统一资源工具定义 */
export const resourceTools: ToolCall[] = [
	{
		type: 'function',
		function: {
			name: 'list_resources',
			description: `列出当前可用的全部资源（文本文件、图片、视频、音频共用一份目录），返回句柄 + 类型 + 标题/描述 + 内容状态。当你需要确认某个句柄是否还有效、或想知道现在手上有哪些资料时调用。${grammarCheatsheet()}。`,
			parameters: {
				type: 'object',
				properties: {
					kind: {
						type: 'string',
						description: '可选：只列出某一类资源，取 D 文档 / I 图片 / V 视频 / A 音频 / T 表格'
					},
					limit: {
						type: 'number',
						description: '返回条目数量上限，默认 30'
					}
				},
				required: []
			}
		}
	},
	{
		type: 'function',
		function: {
			name: 'read_resource',
			description: '按句柄取回资源内容：文档返回完整段落正文（绝不半句截断），图片/视频/音频返回内容描述（不注入本体，上下文成本极低）。句柄可带锚点定向读取某一节或某几行，例如 @D.003|§4 或 @D.003|L120-168。正文已从内容仓库回收时返回目录摘要与结构提示，句柄依然有效。',
			parameters: {
				type: 'object',
				properties: {
					handles: {
						type: 'array',
						items: { type: 'string' },
						description: '资源句柄数组，如 ["@D.003", "@D.003|§4", "@I.007"]'
					},
					question: {
						type: 'string',
						description: '可选：用户当前关心的具体问题（图片/视频/音频按此定向生成描述）'
					}
				},
				required: ['handles']
			}
		}
	},
	{
		type: 'function',
		function: {
			name: 'outline_resource',
			description: '取文档的段落结构（目录树），每节返回规范锚点句柄 @D.003|§n 与行区间、字数。不知道文档里有什么、或需要定位到具体章节时先调用本工具，再用 read_resource 按锚点整段读取 —— 这比一次性读全文更省上下文。',
			parameters: {
				type: 'object',
				properties: {
					handle: {
						type: 'string',
						description: '文档句柄，如 "@D.003"'
					}
				},
				required: ['handle']
			}
		}
	},
	{
		type: 'function',
		function: {
			name: 'search_resources',
			description: '跨资源搜索：在全部资源的标题、摘要、标签与段落标题中匹配关键词，返回「哪份资源的哪一节」命中了。本工具只做文档资源定位，**不检索聊天记忆**——问「谁之前说过什么 / 以前聊过什么」请用 search_memory。取正文请随后调用 read_resource。适合用户问「我之前给过你的那份配置里…」这类跨资料检索。',
			parameters: {
				type: 'object',
				properties: {
					query: {
						type: 'string',
						description: '搜索关键词'
					},
					kind: {
						type: 'string',
						description: '可选：限定资源类型，取 D / I / V / A / T'
					},
					limit: {
						type: 'number',
						description: '返回条数上限，默认 10'
					}
				},
				required: ['query']
			}
		}
	}
];

// ==== 模块级注册 ====
GlobalConfig.LTPfunction.set('list_resources', handleListResources);
GlobalConfig.LTPfunction.set('read_resource', handleReadResource);
GlobalConfig.LTPfunction.set('outline_resource', handleOutlineResource);
GlobalConfig.LTPfunction.set('search_resources', handleSearchResources);
