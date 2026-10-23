import { ToolCall } from '../config/tool';
import { GlobalConfig } from '../config/global';
import { ResourceEntry } from '../config/resource';
import {
	KIND_LABELS,
	formatHandle,
	grammarCheatsheet,
	parseHandle,
	sectionHandle
} from '../capabilities/handle';
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
import { describeResource, inspectTextResource } from '../capabilities/describe';

// ===== 统一资源工具（对话者可用） =====
//
// 对话者只看到两个工具：
//   browse_resources  目录 / 跨资源检索（不以单个句柄为作用域）
//   use_resource      按句柄读取、取描述或让描述者探查
//
// 句柄调用语法：
//   @D.003                       文档句柄
//   @D.003|§4 / |L120-168        章节 / 行区间锚点
//   use_resource 的 ask 字段     自然语言需求（唯一入口：句柄里不再拼接需求）
//
// 旧的 list_resources / read_resource / search_resources 仍保留 handler 注册，
// 供旧打包产物与历史调用兼容；它们不再出现在对话者的工具清单里。

/** 单次读取返回的字符上限（与 Go 侧 MaxReadBytes 同量级，避免一次读入撑爆上下文） */
const READ_CHAR_BUDGET = 12000;

/** 资源类型码的中文标签 */
function kindLabel(kind: string): string {
	return KIND_LABELS[kind as keyof typeof KIND_LABELS] ?? kind;
}

/** 内容状态说明（blob 丢失的条目会在列出时被清理，正常不会出现在列表里） */
function stateNote(entry: ResourceEntry): string {
	return entry.state === 'active' ? '' : ` [${LOST_FILE_NOTICE}]`;
}

/** 摘要单行预览（目录列表与搜索结果用，避免长描述的影像文本撑爆上下文） */
function previewSummary(text: string, maxChars: number): string {
	const clean = String(text || '').replace(/\s+/g, ' ').trim();
	return clean.length > maxChars ? `${clean.slice(0, maxChars).trim()}…` : clean;
}

/** 资源目录条目的单行展示 */
function entryLine(entry: ResourceEntry): string {
	const desc = entry.summary ? `：${previewSummary(entry.summary, 80)}` : '';
	const sectionNote = entry.section_count > 0 ? ` ${entry.section_count}节` : '';
	return `${entry.handle} [${kindLabel(entry.kind)}] ${entry.title || entry.name}${sectionNote}${stateNote(entry)}${desc}`;
}

/** 组装取回结果（blocks 为空时给出统一失败提示） */
function finishBlocks(blocks: string[], missing: string[]): string[] {
	if (blocks.length === 0) {
		return [`取回失败：${missing.join('、')} —— ${HANDLE_INVALID_NOTICE}。请先用 browse_resources 确认可用句柄`, ''];
	}
	let text = blocks.join('\n\n---\n\n');
	if (missing.length > 0) text += `\n\n以下句柄无法取回：${missing.join('、')}（${HANDLE_INVALID_NOTICE}）`;
	return [text, ''];
}

/**
 * 目录清单文本（browse_resources 无 query 时使用）
 *
 * @param kind 类型码过滤（空表示全部）
 * @param limit 条数上限
 */
function listResourcesText(kind: string, limit: number): string {
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
		return `资源目录为空：当前没有已收录的资源${purgedNote}。用户可以上传图片、发送文本文件，或让我生成图片后再引用`;
	}
	const stats = resourceStatistics();
	const lines = valid.map(entryLine);
	const purgeNote = purged > 0 ? `；已自动清理 ${purged} 条源文件丢失的失效句柄` : '';
	const head = `当前可用资源（共 ${valid.length} 条，按收录时间倒序，越靠前越新${purgeNote}${stats ? `；目录总计 ${stats.total} 条 / ${formatBytes(stats.bytes)}` : ''}）：`;
	const tail = `\n${grammarCheatsheet()}。需要内容时用 use_resource 按句柄取回；要描述者探查摘要、结构或引用关系时，在 ask 字段写需求。`;
	return `${head}\n${lines.join('\n')}${tail}`;
}

/**
 * 跨资源检索文本（browse_resources 有 query 时使用）
 *
 * @param query 关键词
 * @param kind 类型码过滤
 * @param limit 条数上限
 */
function searchResourcesText(query: string, kind: string, limit: number): string {
	const hits = searchResources(query, kind, limit);
	if (hits.length === 0) {
		return `未找到与「${query}」相关的资源。可用 browse_resources 查看全部资源，或让用户重新提供资料`;
	}
	const lines: string[] = [];
	for (const hit of hits) {
		if (hit.sections && hit.sections.length > 0) {
			const sectionLines = hit.sections.slice(0, 5).map(section => {
				const parsed = parseHandle(hit.handle);
				const anchor = parsed ? sectionHandle(parsed.kind, parsed.seq, section.ordinal) : hit.handle;
				return `    ${anchor} ${section.path || section.title || `第 ${section.ordinal} 段`}（第 ${section.line_start}-${section.line_end} 行）`;
			});
			lines.push(`${hit.handle} [${kindLabel(hit.kind)}] ${hit.title}${hit.summary ? `：${previewSummary(hit.summary, 120)}` : ''}\n  命中章节：\n${sectionLines.join('\n')}`);
		}
		else {
			lines.push(`${hit.handle} [${kindLabel(hit.kind)}] ${hit.title}${hit.summary ? `：${previewSummary(hit.summary, 120)}` : ''}`);
		}
	}
	return `与「${query}」相关的资源（共 ${hits.length} 条）：\n${lines.join('\n')}\n\n下一步：用 use_resource 按上述句柄取回正文`;
}

/**
 * 单个句柄的读取/探查（use_resource 的核心）
 *
 * 路由：
 *   - 图片 / 动图 / 视频 / 音频：交给 describeResource（描述者按需生成并入库）
 *   - 文档 / 表格带 ask：一律交给描述者探查（锚点原样传入，由描述者按该段作答）
 *   - 文档 / 表格不带 ask：直接返回原文（带锚点即该段原文）
 *
 * @param rawHandle 句柄（含可选锚点）
 * @param ask 自然语言需求（唯一入口，可空）
 * @param missing 无法取回的句柄收集器
 * @returns 展示块；无法取回时返回空串
 */
function readHandleBlock(rawHandle: string, ask: string, missing: string[]): string {
	const parsedHandle = parseHandle(rawHandle);
	if (!parsedHandle) {
		missing.push(rawHandle || '(空)');
		return '';
	}
	const normalized = formatHandle(parsedHandle);
	const entry = getResourceEntry(normalized);
	if (!entry) {
		missing.push(normalized);
		return '';
	}
	if (entry.state === 'lost') {
		deleteResource(normalized);
		return `【${normalized}】\n${LOST_FILE_NOTICE}`;
	}

	// 非文本资源：描述者统一处理「有描述返回描述，没有现场生成」
	if (parsedHandle.kind === 'I' || parsedHandle.kind === 'V' || parsedHandle.kind === 'A') {
		const desc = describeResource(normalized, ask);
		if (!desc && !getResourceEntry(normalized)) {
			return `【${normalized}】\n${LOST_FILE_NOTICE}`;
		}
		return `【${normalized}｜${kindLabel(parsedHandle.kind)}｜${entry.title || entry.name}】\n${desc || '（暂无描述，且本体不可直接解读）'}`;
	}

	// 文本类：只要给了 ask 就调用描述者（锚点原样传入，由描述者只针对该范围作答）
	if (ask) {
		const answer = inspectTextResource(normalized, ask);
		if (!answer) {
			return `【${normalized}｜${entry.title || entry.name}】\n探查失败：描述者没有给出答复，可去掉 ask 直接取原文（或用 |L起始-结束 读片段）`;
		}
		return `【${normalized}｜${entry.title || entry.name}】\n${answer}`;
	}

	const result = readResource(normalized, READ_CHAR_BUDGET);
	if (!result) {
		missing.push(normalized);
		return '';
	}
	if (result.lost) {
		deleteResource(normalized);
		return `【${normalized}｜${result.title}】\n${LOST_FILE_NOTICE}`;
	}
	const rangeNote = result.line_start && result.line_end ? `｜第 ${result.line_start}-${result.line_end} 行` : '';
	const sectionNote = result.section_title ? `｜${result.section_title}` : '';
	const truncated = result.truncated ? '\n（内容较长已截断；可带 |L起-止 或 |§n 定向读取）' : '';
	return `【${normalized}${sectionNote}${rangeNote}｜共 ${result.total_lines || 0} 行】\n${result.content || ''}${truncated}`;
}

/** 处理按句柄读取/探查（use_resource） */
async function handleUseResource(args?: Record<string, any> | string): Promise<string[]> {
	const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
	const rawHandles: unknown[] = Array.isArray(parsed.handles)
		? parsed.handles
		: (parsed.handle ? [parsed.handle] : []);
	if (rawHandles.length === 0) {
		return ['请提供资源句柄，例如 ["@D.003"]；需要描述者协助时把需求写进 ask 字段', ''];
	}
	const globalAsk = String(parsed.ask || '').trim();
	const blocks: string[] = [];
	const missing: string[] = [];
	for (const raw of rawHandles) {
		const block = readHandleBlock(String(raw || ''), globalAsk, missing);
		if (block) blocks.push(block);
	}
	return finishBlocks(blocks, missing);
}

/** 处理目录浏览 / 跨资源检索（browse_resources） */
async function handleBrowseResources(args?: Record<string, any> | string): Promise<string[]> {
	const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
	const kind = String(parsed.kind || '').trim().toUpperCase();
	if (kind && !KIND_LABELS[kind as keyof typeof KIND_LABELS]) {
		return [`类型码无效：${kind}。可用类型码：D 文档 / I 图片 / V 视频 / A 音频 / T 表格`, ''];
	}
	const query = String(parsed.query || '').trim();
	const fallback = query ? 10 : 30;
	const limit = Math.min(Math.max(Number(parsed.limit) || fallback, 1), query ? 50 : 200);
	if (query) return [searchResourcesText(query, kind, limit), ''];
	return [listResourcesText(kind, limit), ''];
}

/** 旧工具别名：list_resources（仅历史兼容，不再出现在工具清单） */
async function handleListResources(args?: Record<string, any> | string): Promise<string[]> {
	const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
	const kind = String(parsed.kind || '').trim().toUpperCase();
	if (kind && !KIND_LABELS[kind as keyof typeof KIND_LABELS]) {
		return [`类型码无效：${kind}。可用类型码：D 文档 / I 图片 / V 视频 / A 音频 / T 表格`, ''];
	}
	const limit = Math.min(Math.max(Number(parsed.limit) || 30, 1), 200);
	return [listResourcesText(kind, limit), ''];
}

/** 旧工具别名：search_resources（仅历史兼容） */
async function handleSearchResources(args?: Record<string, any> | string): Promise<string[]> {
	const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
	const query = String(parsed.query || '').trim();
	if (!query) return ['请提供搜索关键词', ''];
	const kind = String(parsed.kind || '').trim().toUpperCase();
	const limit = Math.min(Math.max(Number(parsed.limit) || 10, 1), 50);
	return [searchResourcesText(query, kind, limit), ''];
}

/** 旧工具别名：read_resource（仅历史兼容，按新路由执行） */
async function handleReadResource(args?: Record<string, any> | string): Promise<string[]> {
	const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
	const rawHandles: unknown[] = Array.isArray(parsed.handles)
		? parsed.handles
		: (parsed.handle ? [parsed.handle] : []);
	if (rawHandles.length === 0) {
		return ['请提供资源句柄，例如 ["@D.003"]。可用清单见 browse_resources', ''];
	}
	const question = String(parsed.question || '').trim().slice(0, 200);
	const blocks: string[] = [];
	const missing: string[] = [];
	for (const raw of rawHandles) {
		const block = readHandleBlock(String(raw || ''), question, missing);
		if (block) blocks.push(block);
	}
	return finishBlocks(blocks, missing);
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

/** 对话者可见的统一资源工具定义 */
export const resourceTools: ToolCall[] = [
	{
		type: 'function',
		function: {
			name: 'use_resource',
			description: `按句柄读取或探查资源。每个句柄可写成 @D.003、@D.003|§4、@D.003|L120-168。规则：不填 ask 时，文档/表格直接返回原文（带锚点即该段原文）；填写 ask 时，文档/表格一律交给描述者按需求探查（带锚点则只关注该段）；图片/动图/视频返回描述——已有描述直接返回，没有则由描述者现场生成并入库；音频返回已有转写。${grammarCheatsheet()}。`,
			parameters: {
				type: 'object',
				properties: {
					handles: {
						type: 'array',
						items: { type: 'string' },
						description: '资源句柄数组，如 ["@D.003", "@D.003|§4", "@D.003|L120-168", "@I.007", "@V.001"]；需求请写 ask 字段，不要拼进句柄'
					},
					ask: {
						type: 'string',
						description: '可选：交给描述者的自然语言需求。文档/表格填了它就一定会调用描述者（带锚点时按该段范围作答）；不填则直接返回原文。'
					}
				},
				required: ['handles']
			}
		}
	},
	{
		type: 'function',
		function: {
			name: 'browse_resources',
			description: '浏览资源目录或跨资源检索。不传 query 时按 kind 列出句柄 + 类型 + 标题/描述 + 内容状态；传 query 时在标题、摘要、标签与段落标题中匹配，返回「哪份资源的哪一节」命中了。本工具只做资源目录，不检索聊天记忆——问「谁之前说过什么 / 以前聊过什么」请用 search_memory。拿到句柄后用 use_resource 读取。',
			parameters: {
				type: 'object',
				properties: {
					query: {
						type: 'string',
						description: '可选：搜索关键词；省略时按目录列出'
					},
					kind: {
						type: 'string',
						description: '可选：只处理某一类资源，取 D 文档 / I 图片 / V 视频 / A 音频 / T 表格'
					},
					limit: {
						type: 'number',
						description: '返回条目数量上限；列目录默认 30，检索默认 10'
					}
				},
				required: []
			}
		}
	}
];

// ==== 模块级注册 ====

// 对话者工具清单（新）
GlobalConfig.LTPfunction.set('use_resource', handleUseResource);
GlobalConfig.LTPfunction.set('browse_resources', handleBrowseResources);
// 历史别名：旧打包产物 / 旧提示词仍可能按旧名调用（不出现在新工具清单）
GlobalConfig.LTPfunction.set('list_resources', handleListResources);
GlobalConfig.LTPfunction.set('read_resource', handleReadResource);
GlobalConfig.LTPfunction.set('search_resources', handleSearchResources);
