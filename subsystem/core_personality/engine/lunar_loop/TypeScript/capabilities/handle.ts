import { ResourceKindCode } from '../config/resource';

// ===== 统一资源句柄文法（TypeScript 侧唯一实现） =====
//
// 权威定义在 Go 侧 module.HandlePattern（经 resourceGrammar() 下发）。
// 本模块在加载时优先取用下发值，取不到时回落到与 Go 完全一致的本地副本
// （Go 不可用的极早期阶段，例如 bundle 刚载入、运行时尚未注册适配器）。
//
//   resource-handle := "@" typecode "." seq3 [ anchor ]
//   typecode        := D | I | V | A | T
//   anchor          := "|§" section-ordinal | "|L" line-start "-" line-end
//
// 禁止在其它模块自写句柄正则：解析入口一律走本模块。

/** 回落用的句柄文法（必须与 Go 侧 module.HandlePattern 保持逐字一致） */
const FALLBACK_PATTERN = '@([DIVAT])\\.(\\d{3,})(?:\\|(?:§(\\d+)|L(\\d+)-(\\d+)))?';

/** 句柄文法（加载时从 Go 侧取回，失败回落本地副本） */
const HANDLE_PATTERN: string = (() => {
	try {
		const grammar = resourceGrammar();
		if (grammar && typeof grammar.pattern === 'string' && grammar.pattern.length > 0) return grammar.pattern;
	}
	catch {
		// Go 适配器尚未就绪：回落本地副本
	}
	return FALLBACK_PATTERN;
})();

/** 严格匹配整个字符串的句柄正则（全局副本用于正文提取） */
const HANDLE_EXACT = new RegExp(`^${HANDLE_PATTERN}$`);
/** 正文提取用的全局句柄正则 */
const HANDLE_GLOBAL = new RegExp(HANDLE_PATTERN, 'g');

/** 类型码 → 中文类型名（提示文案与 UI 展示用） */
export const KIND_LABELS: Record<ResourceKindCode, string> = {
	D: '文档',
	I: '图片',
	V: '视频',
	A: '音频',
	T: '表格'
};

/** 按类型码判断资源是否为文本类（可整段展开正文） */
export function isTextKind(kind: ResourceKindCode): boolean {
	return kind === 'D' || kind === 'T';
}

/** 解析后的资源句柄 */
export type ResourceHandle = {
	/** 类型码 */
	kind: ResourceKindCode;
	/** 资源序号 */
	seq: number;
	/** 章节序号（|§n 形态；无锚点为 0） */
	section: number;
	/** 行区间起点（|L.. 形态；无锚点为 0） */
	lineStart: number;
	/** 行区间终点（|L.. 形态；无锚点为 0） */
	lineEnd: number;
	/** 锚点种类 */
	anchor: 'none' | 'section' | 'line';
};

/**
 * 解析句柄文本（容忍首尾空白与方括号包裹）
 *
 * @param raw 待解析文本
 * @returns 解析结果；非法输入返回 null
 */
export function parseHandle(raw: string): ResourceHandle | null {
	if (!raw) return null;
	let text = String(raw).trim();
	// 容忍 [@D.003] / 【@D.003】 这类展示包裹
	text = text.replace(/^[[【]/, '').replace(/[\]】]$/, '');
	const match = HANDLE_EXACT.exec(text);
	if (!match) return null;
	const seq = Number(match[2]);
	if (!Number.isFinite(seq) || seq <= 0) return null;
	const handle: ResourceHandle = {
		kind: match[1] as ResourceKindCode,
		seq,
		section: 0,
		lineStart: 0,
		lineEnd: 0,
		anchor: 'none'
	};
	if (match[3] !== undefined && match[3] !== '') {
		handle.anchor = 'section';
		handle.section = Number(match[3]);
	}
	else if (match[4] !== undefined && match[4] !== '') {
		handle.anchor = 'line';
		handle.lineStart = Number(match[4]);
		handle.lineEnd = Number(match[5] ?? match[4]);
	}
	return handle;
}

/**
 * 规整句柄文本为规范形态（补零、剔除多余包裹与空白）
 *
 * @param raw 待规整文本
 * @returns 规范句柄文本；非法输入返回空字符串
 */
export function normalizeHandle(raw: string): string {
	const parsed = parseHandle(raw);
	return parsed ? formatHandle(parsed) : '';
}

/** 规范化序号：三位起补零，超出三位按实际宽度（与 Go 侧 padSeq 一致） */
export function padSeq(seq: number): string {
	const width = seq >= 1000 ? 4 : 3;
	return String(seq).padStart(width, '0');
}

/**
 * 按文法格式化句柄
 *
 * @param handle 解析后的句柄（锚点可选）
 * @returns 规范句柄文本
 */
export function formatHandle(handle: Pick<ResourceHandle, 'kind' | 'seq'> & Partial<ResourceHandle>): string {
	let text = `@${handle.kind}.${padSeq(handle.seq)}`;
	if (handle.anchor === 'section' && handle.section) text += `|§${handle.section}`;
	else if (handle.anchor === 'line' && handle.lineStart) {
		text += `|L${handle.lineStart}-${handle.lineEnd || handle.lineStart}`;
	}
	return text;
}

/** 构造不带锚点的资源句柄 */
export function handleOf(kind: ResourceKindCode, seq: number): string {
	return `@${kind}.${padSeq(seq)}`;
}

/** 构造章节锚点句柄（文档大纲展开用） */
export function sectionHandle(kind: ResourceKindCode, seq: number, ordinal: number): string {
	return `${handleOf(kind, seq)}|§${ordinal}`;
}

/** 构造行区间锚点句柄 */
export function lineHandle(kind: ResourceKindCode, seq: number, start: number, end: number): string {
	return `${handleOf(kind, seq)}|L${start}-${end}`;
}

/**
 * 从任意文本中提取全部句柄（按出现顺序，保留重复）
 *
 * @param text 待扫描文本
 * @returns 句柄文本数组（规范形态）
 */
export function extractHandles(text: string): string[] {
	if (!text) return [];
	const found: string[] = [];
	HANDLE_GLOBAL.lastIndex = 0;
	let match: RegExpExecArray | null;
	while ((match = HANDLE_GLOBAL.exec(text)) !== null) {
		const parsed = parseHandle(match[0]);
		if (parsed) found.push(formatHandle(parsed));
	}
	return found;
}

/** 判断文本是否整体就是一个句柄 */
export function isHandleText(text: string): boolean {
	return parseHandle(text) !== null;
}

/** 句柄调用查询：句柄 + 可选自然语言需求 */
export type ResourceQuery = {
	/** 规范句柄部分（含锚点） */
	handle: string;
	/** `&` 之后的自然语言需求（无则为空串） */
	ask: string;
};

/**
 * 解析句柄调用语法 `handle [ "&" ask ]`
 *
 * 句柄本身仍由 handle.ts 的唯一文法解析；`&` 只是调用层分隔符，
 * 不进入资源目录、不参与持久化。约束：
 *   - 只按**第一个** `&` 切分，ask 内可以继续出现 `&`、换行与中文标点；
 *   - 容忍 `[@D.003&说明]` / `【@D.003&说明】` 这类展示包裹；
 *   - 未带 `&` 时 ask 为空串。
 *
 * @param raw 原始句柄或句柄+需求文本
 * @returns { handle, ask }
 */
export function parseResourceQuery(raw: string): ResourceQuery {
	let text = String(raw || '').trim();
	// 先剥展示包裹，再切 &，避免 ask 末尾的 ] 被误当成句柄的一部分
	text = text.replace(/^[[【]/, '').replace(/[\]】]$/, '');
	const index = text.indexOf('&');
	if (index < 0) return { handle: text.trim(), ask: '' };
	return {
		handle: text.slice(0, index).trim(),
		ask: text.slice(index + 1).trim()
	};
}

/** 面向用户的句柄说明（提示文案与占位文本使用） */
export function describeHandle(raw: string): string {
	const parsed = parseHandle(raw);
	if (!parsed) return String(raw || '');
	const label = KIND_LABELS[parsed.kind] ?? parsed.kind;
	const base = `${label} ${handleOf(parsed.kind, parsed.seq)}`;
	if (parsed.anchor === 'section') return `${base} 第 ${parsed.section} 节`;
	if (parsed.anchor === 'line') return `${base} 第 ${parsed.lineStart}-${parsed.lineEnd} 行`;
	return base;
}

/** 供工具描述拼接的文法速查（避免各处硬编码示例） */
export function grammarCheatsheet(): string {
	return '句柄形如 @D.003（文档）、@I.007（图片）、@V.001（视频）、@A.002（音频）；'
		+ '段落锚点可写 @D.003|§4（第 4 节）或 @D.003|L120-168（第 120-168 行）；'
		+ '需要描述者探查时可写成 @D.003&自然语言需求';
}
