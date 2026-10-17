import type { ResourceSection, ResourceSectionDraft } from '../../config/resource';

/** 无标题文本退化为段落时的行块大小（必须与 Go 侧 module.SectionBlockLines 一致） */
export const SECTION_BLOCK_LINES = 60;

/**
 * 从资源段落表条目推导行区间（Go 侧 ResourceSection 的 snake_case 字段）
 *
 * @param section 段落表条目
 * @returns 1 基行区间（含端点）
 */
export function sectionLines(section: ResourceSection): [number, number] {
	return [section.line_start, section.line_end];
}

/**
 * 解析文档的段落结构（行区间为 1 基、含端点）
 *
 * 本函数是 Go 侧 module/archive_sections.go ParseMarkdownSections 的镜像实现：
 * 两者必须保持逐条一致的切段规则，否则模型从大纲拿到的行区间与 Go 侧整段展开
 * 会错位。改动其一时必须同步另一处。
 *
 * 规则：
 *   - 存在 Markdown 标题 → 每个标题一节，正文为标题行之后到下一标题行之前（含标题行）
 *   - 无标题 → 按 SECTION_BLOCK_LINES 行分块，标题为空、层级为 0
 *
 * @param text 文档原始文本
 * @returns 段落草稿数组（ordinal 从 1 开始）
 */
export function parseDocumentSections(text: string): ResourceSectionDraft[] {
	const lines = String(text ?? '').replace(/\r\n/g, '\n').split('\n');
	if (lines.length === 0) return [];

	type Heading = { index: number; level: number; title: string };
	const headings: Heading[] = [];
	for (let i = 0; i < lines.length; i++) {
		const parsed = parseHeadingLine(lines[i]);
		if (parsed) headings.push({ index: i, level: parsed.level, title: parsed.title });
	}
	if (headings.length === 0) return fallbackSections(lines);

	const drafts: ResourceSectionDraft[] = [];
	const pathStack: Heading[] = [];
	for (let ordinal = 0; ordinal < headings.length; ordinal++) {
		const heading = headings[ordinal];
		// 维护层级路径栈：遇到更浅或同级标题就弹出
		while (pathStack.length > 0 && pathStack[pathStack.length - 1].level >= heading.level) pathStack.pop();
		pathStack.push(heading);
		const endIndex = ordinal + 1 < headings.length ? headings[ordinal + 1].index : lines.length;
		drafts.push({
			ordinal: ordinal + 1,
			level: heading.level,
			title: heading.title,
			path: pathStack.map(item => item.title).join(' / '),
			lineStart: heading.index + 1,
			lineEnd: endIndex,
			chars: countChars(lines.slice(heading.index, endIndex))
		});
	}
	return drafts;
}

/** 无标题文本的行块切分（镜像 Go 侧 fallbackSections） */
function fallbackSections(lines: string[]): ResourceSectionDraft[] {
	const drafts: ResourceSectionDraft[] = [];
	const total = lines.length;
	for (let start = 0; start < total; start += SECTION_BLOCK_LINES) {
		const end = Math.min(start + SECTION_BLOCK_LINES, total);
		drafts.push({
			ordinal: drafts.length + 1,
			level: 0,
			title: '',
			path: '',
			lineStart: start + 1,
			lineEnd: end,
			chars: countChars(lines.slice(start, end))
		});
	}
	return drafts;
}

/** 解析 Markdown ATX 标题行（# 到 ######，后接空格或直接结束） */
function parseHeadingLine(line: string): { level: number; title: string } | null {
	const trimmed = (line ?? '').replace(/^[ \t]+/, '');
	let level = 0;
	while (level < trimmed.length && trimmed[level] === '#') level++;
	if (level === 0 || level > 6) return null;
	const rest = trimmed.slice(level);
	if (rest === '') return { level, title: '' };
	if (rest[0] !== ' ' && rest[0] !== '\t') return null;
	return { level, title: rest.trim() };
}

/** 统计行块字符数（不含换行，与 Go 侧 countChars 一致） */
function countChars(lines: string[]): number {
	let total = 0;
	for (const line of lines) total += Array.from(line).length;
	return total;
}

/**
 * 按行区间截取文本（1 基、含端点；越界自动收敛）
 *
 * @param text 文档全文
 * @param start 起始行
 * @param end 结束行
 * @returns 该区间的文本
 */
export function sliceLines(text: string, start: number, end: number): string {
	const lines = String(text ?? '').replace(/\r\n/g, '\n').split('\n');
	const from = Math.max(1, start);
	const to = Math.min(lines.length, Math.max(from, end));
	if (from > lines.length) return '';
	return lines.slice(from - 1, to).join('\n');
}

/**
 * 统计文本行数（用于段落表缺失时的兜底展示）
 *
 * @param text 文本
 * @returns 行数（空文本为 0）
 */
export function countLines(text: string): number {
	if (!text) return 0;
	return String(text).replace(/\r\n/g, '\n').split('\n').length;
}
