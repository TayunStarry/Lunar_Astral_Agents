import { GlobalConfig } from '../../config/global';
import { PostMessage, TextContent } from '../../config/model';
import { ResourceSection } from '../../config/resource';
import { parseDocumentSections } from '../../file/parse/resource-sections';
import { getFileIndexFromKnowledge, saveFileIndexToKnowledge } from '../../file/io/knowledge';
import { extractHandles, formatHandle, grammarCheatsheet, parseHandle } from '../capabilities/handle';
import {
	admitBytes,
	admitDocumentByName,
	degradeNotice,
	deleteResource,
	listResources,
	outlineResource,
	readResource,
	setResourceSections,
	setResourceSummary
} from '../capabilities/resource';
import { describeResource } from '../capabilities/describe';
import { interactEvent } from '../capabilities/ltp-event';

// 阅读者智能体：把用户推送的文本文件收进统一资源归档层（整篇一份 + 段落表），
// 并在正文出现句柄引用时按结构定位、整段展开。
//
// 设计依据：docs/adr/0001-统一资源寻址与三层存储.md
//   - 文件不再切片进向量库：整篇存一次，段落只作为资源目录里的索引单元
//   - 引用文法统一为 @D.003（可选锚点 |§4 / |L120-168），与图片 @I.007 同源
//   - 旧 file_mapping / file_<hash> 集合按需自动迁移，不丢正文
//
// 迁移约定：FileIndex 结构沿用旧字段名（collection / chunkCount 等），
// 使历史索引条目无需转换即可读出；新写入的条目带 version 字段标记为资源化索引。

/** 可入库的文本类扩展名白名单（其余按媒体/普通文本透传） */
const FILE_WHITELIST = [
	'ts', 'js', 'tsx', 'jsx', 'go', 'py', 'java', 'c', 'h', 'cpp', 'cxx', 'hpp', 'cs',
	'md', 'markdown', 'txt', 'json', 'xml', 'yaml', 'yml', 'toml', 'ini', 'cfg', 'log',
	'csv', 'rs', 'rb', 'sh', 'ps1', 'bat', 'sql', 'html', 'htm', 'css', 'vue'
];

/** 二进制扩展名（明确不走文本入库通道） */
const BINARY_EXTENSIONS = [
	'png', 'jpg', 'jpeg', 'webp', 'gif', 'bmp', 'tiff', 'tif', 'ico', 'svg',
	'mp4', 'webm', 'avi', 'mov', 'mkv', 'flv', 'm4v', 'mp3', 'wav', 'flac', 'ogg', 'm4a',
	'zip', '7z', 'rar', 'gz', 'exe', 'dll', 'pdf', 'docx', 'xlsx', 'pptx'
];

/** 文件围栏块起始标记：```file.ext */
const FENCE = '```';

/** 单资源单次引用的展开片段数上限 */
const QUERY_TOP_K = 8;
/** 单资源单次引用展开的字符总量上限（防止一次引用撑爆上下文） */
const QUERY_CHAR_BUDGET = 12000;
/** 段落正文参与词法评分的最大字符数（超出部分截断，避免全文扫描） */
const SCORE_BODY_CHARS = 4000;
/**
 * 无段落结构资源自动注入全文的字符上限
 *
 * 超过此值只注入资源卡片（标题 / 体积 / 摘要 / 取用提示），正文由模型按需调
 * read_resource 读取 —— 引用展开发生在上下文构建阶段，注入多少就直接占多少 token。
 */
const FULLTEXT_CHAR_LIMIT = 6000;

/** 资源索引条目（存于资源目录的 file_mapping 表，键为规范句柄 + 来源路径） */
type FileIndex = {
	/** 向量集合名（旧字段，资源化后仅作兼容留痕，不再写入向量） */
	collection: string;
	/** 原始文件名 */
	name: string;
	/** 来源路径（工作区相对路径或 LocalDir 相对路径；同内容不同路径的文件靠它区分） */
	path?: string;
	/** 内容哈希（sha16；用于判断来源文件是否已变更） */
	contentHash?: string;
	/** 小写扩展名（无点） */
	ext: string;
	/** 代码语言标识（非代码为 null） */
	lang: string | null;
	/** 段落数 */
	chunkCount: number;
	/** 处理时间（毫秒） */
	processedAt: number;
	/** 资源化索引版本标记（2 = 统一资源归档层） */
	version?: number;
	/** 资源目录主键 */
	resourceId?: number;
	/** 规范句柄 */
	handle?: string;
};

/** 解析后的引用块 */
type Ref = {
	/** 原始文本 */
	raw: string;
	/** 起始下标 */
	start: number;
	/** 结束下标（不含） */
	end: number;
	/** 规范句柄（解析成功时） */
	handle: string;
};

/** 扩展名（小写，无点） */
function extOf(fileName: string): string {
	const parts = (fileName || '').split('.');
	return parts.length > 1 ? parts.pop()!.toLowerCase() : '';
}

/** 去掉扩展名的基名 */
function baseName(fileName: string): string {
	const idx = (fileName || '').lastIndexOf('.');
	return idx > 0 ? fileName.slice(0, idx) : fileName;
}

/** 判断某文本是否像文件围栏块的开头（```后紧跟一个文件名） */
function looksLikeFileHeader(firstLine: string): boolean {
	return /^[A-Za-z0-9_\-.\u4e00-\u9fff]+\.\w+$/.test((firstLine || '').trim());
}

/** 文本转 UTF-8 字节（入库层按字节计算内容哈希） */
function toUTF8Bytes(text: string): Uint8Array {
	const bytes = new Uint8Array(text.length * 3);
	let offset = 0;
	for (let i = 0; i < text.length; i++) {
		let code = text.charCodeAt(i);
		// 代理对：合并为完整码点
		if (code >= 0xd800 && code <= 0xdbff && i + 1 < text.length) {
			const next = text.charCodeAt(i + 1);
			if (next >= 0xdc00 && next <= 0xdfff) {
				code = ((code - 0xd800) << 10) + (next - 0xdc00) + 0x10000;
				i++;
			}
		}
		if (code < 0x80) bytes[offset++] = code;
		else if (code < 0x800) {
			bytes[offset++] = 0xc0 | (code >> 6);
			bytes[offset++] = 0x80 | (code & 0x3f);
		}
		else if (code < 0x10000) {
			bytes[offset++] = 0xe0 | (code >> 12);
			bytes[offset++] = 0x80 | ((code >> 6) & 0x3f);
			bytes[offset++] = 0x80 | (code & 0x3f);
		}
		else {
			bytes[offset++] = 0xf0 | (code >> 18);
			bytes[offset++] = 0x80 | ((code >> 12) & 0x3f);
			bytes[offset++] = 0x80 | ((code >> 6) & 0x3f);
			bytes[offset++] = 0x80 | (code & 0x3f);
		}
	}
	return bytes.subarray(0, offset);
}

/** 代码语言标识（用于提示文案；段落切分已统一按行结构，不再按语言分流） */
const CODE_LANGS: Record<string, string> = {
	ts: 'TypeScript', tsx: 'TypeScript', js: 'JavaScript', jsx: 'JavaScript',
	go: 'Go', py: 'Python', java: 'Java', c: 'C', h: 'C', cpp: 'C++', cxx: 'C++',
	hpp: 'C++', cs: 'C#', rs: 'Rust', rb: 'Ruby', sh: 'Shell', ps1: 'PowerShell',
	bat: 'Batch', sql: 'SQL', html: 'HTML', htm: 'HTML', css: 'CSS', vue: 'Vue'
};

/** 读取资源索引条目（新句柄键优先，来源路径键其次，旧 #名 键兜底） */
function loadFileIndex(handle: string, fileName?: string, sourcePath?: string): FileIndex | null {
	const keys = [handle];
	if (sourcePath) keys.push(`@path:${sourcePath}`);
	if (fileName) keys.push(`#${fileName}`);
	for (const key of keys) {
		const cached = getFileIndexFromKnowledge(key);
		if (!cached) continue;
		try {
			return JSON.parse(cached) as FileIndex;
		}
		catch {
			continue;
		}
	}
	return null;
}

/**
 * 写入资源索引条目
 *
 * 同时登记两个键，缺一不可：
 *   - `@句柄`：按句柄引用时的直接命中
 *   - `@path:来源路径`：**按路径判断来源文件是否已收录** —— 没有它，每次引用都会
 *     重新走一遍收录流程，同名不同路径的文件（如仓库根与两个子系统各有一个
 *     build.ps1）会不断累积成多条目录记录
 */
function storeFileIndex(handle: string, fileName: string, index: FileIndex): void {
	saveFileIndexToKnowledge(handle, JSON.stringify(index));
	if (index.path) saveFileIndexToKnowledge(`@path:${index.path}`, JSON.stringify(index));
	// 旧键改写为新索引：下次引用旧 `#名` 形态时可直接复用，不重复入库
	if (fileName && `#${fileName}` !== handle) saveFileIndexToKnowledge(`#${fileName}`, JSON.stringify(index));
}

/**
 * 收录文本资源：同一份文件（同名 + 同内容）重复导入时复用既有句柄
 *
 * 为什么需要显式复用：统一入库入口按「每次入库各分配一个句柄」实现（既有的
 * 句柄永不复用），若前端重复发送同一份文件就不断新增目录行与句柄，很快把目录撑满。
 * 这里以「同名 + 同内容」为去重键：内容相同但文件名不同的两份资料仍各占一个句柄
 * （它们在使用上是两份资料）。
 *
 * @param fileName 文件名
 * @param content 文件正文
 * @returns 入库结果（可能是复用既有句柄）
 */
function admitDocumentContent(fileName: string, content: string) {
	const admitted = admitBytes(toUTF8Bytes(content), fileName, 'client-upload', 'normal');
	if (!admitted) return null;
	if (!admitted.deduped) return admitted;
	// 内容哈希命中既有 blob：确认目录中是否已有同名资源，有则复用其句柄
	const existing = listResources(admitted.kind, 500).find(entry =>
		entry.name === fileName && entry.content_hash === admitted.content_hash
	);
	if (!existing || existing.id === admitted.id) return admitted;
	// 复用既有资源：删除本次新增的冗余目录行（blob 共享，不删实体）
	deleteResource(admitted.handle);
	return {
		...admitted,
		id: existing.id,
		handle: existing.handle,
		seq: existing.seq,
		deduped: true
	};
}

/**
 * 导入一段文本文件围栏块：整篇入库 → 解析段落表 → 登记索引
 *
 * 资源消耗对比旧实现：旧路径按 1024 字切片逐片 embedding 写入向量库（3000 行 ≈ 30+ 次
 * 嵌入调用）；新路径只有一次内容哈希 + 一次目录写入，段落表为纯结构解析（零模型调用）。
 *
 * @param fileName 文件名（围栏首行）
 * @param content 文件正文（逐字节原样，不得规范化换行，否则段落行号会漂移）
 * @returns 入库结果：句柄与是否跳过
 */
export async function importDocumentBlock(fileName: string, content: string): Promise<{ handle: string; skipped: boolean }> {
	const ext = extOf(fileName);
	if (BINARY_EXTENSIONS.includes(ext)) return { handle: '', skipped: true };
	if (!FILE_WHITELIST.includes(ext)) return { handle: '', skipped: true };

	const admitted = admitDocumentContent(fileName, content);
	if (!admitted) return { handle: '', skipped: true };

	// 复用既有句柄时无需重建段落表与索引（内容哈希相同即正文相同）
	if (!admitted.deduped) {
		const drafts = parseDocumentSections(content);
		if (drafts.length > 0) setResourceSections(admitted.id, drafts);
		// 摘要：首个非空行（不带 Markdown 标题符号）写入资源目录，供清单与搜索展示
		const firstLine = content.replace(/^\uFEFF/, '')
			.split('\n')
			.map(line => line.replace(/^#+\s*/, '').trim())
			.find(line => line.length > 0) || '';
		if (firstLine) setResourceSummary(admitted.id, firstLine.slice(0, 200));
		console.log(`[阅读者] 已收录 ${admitted.handle}（${fileName}，${drafts.length} 段，${admitted.size_bytes} 字节）`);
	}

	const index: FileIndex = {
		collection: '',
		name: fileName,
		// 上传件没有相对路径概念，用文件名作为路径键：同名同内容的重复上传同样能命中
		path: fileName,
		contentHash: admitted.content_hash,
		ext,
		lang: CODE_LANGS[ext] ?? null,
		chunkCount: 0,
		processedAt: Date.now(),
		version: 2,
		resourceId: admitted.id,
		handle: admitted.handle
	};
	storeFileIndex(admitted.handle, fileName, index);
	return { handle: admitted.handle, skipped: false };
}

/**
 * 迁移历史 `#文件名.ext` 引用：把旧的「切片进向量库」形态反演重建为整篇资源
 *
 * 旧路径把文件按 1024 字切片，每片以 `*标题> 路径*` 前缀写入记忆库集合 file_<内容哈希>，
 * 并在 file_mapping 表登记 `#文件名.ext → {collection, chunkCount, ...}`。
 * 本函数一次性完成：
 *   1. 按索引找到旧集合，读回全部切片
 *   2. 按路径前缀重建标题层级（`*标题> 路径*` 本身就是结构信息），拼成整篇正文
 *   3. 整篇写入统一资源归档层并重建段落表
 *   4. 把 file_mapping 的原键改写为新索引（旧集合保留只读，作为冷备）
 *
 * 这是幂等操作：迁移后原键即指向 version=2 的新索引，二次调用直接命中缓存。
 *
 * @param fileName 文件名（历史引用 `#文件名.ext` 中 # 之后的部分）
 * @returns 资源句柄；迁移失败返回空串
 */
function migrateLegacyFileIndex(fileName: string): string {
	const cached = getFileIndexFromKnowledge(`#${fileName}`);
	if (!cached) return '';
	let legacy: FileIndex | null = null;
	try {
		legacy = JSON.parse(cached) as FileIndex;
	}
	catch {
		return '';
	}
	if (!legacy) return '';
	// 已是资源化索引：直接返回句柄，无需迁移
	if (legacy.version === 2 && legacy.handle) return legacy.handle;
	if (!legacy.collection) return '';

	try {
		const [ready] = memoryInit(legacy.collection);
		if (!ready) {
			console.error(`[阅读者] 迁移失败：记忆库集合不可用 ${legacy.collection}`);
			return '';
		}
		// 取回全部旧切片（topK 给足，避免只反演出部分正文）
		const [results, queryError] = memoryQuery(legacy.collection, '文件内容 结构 概览 摘要', Math.max(legacy.chunkCount || 0, 64));
		if (queryError || !results || results.length === 0) {
			console.error(`[阅读者] 迁移失败：旧集合无可用切片 ${legacy.collection}`);
			return '';
		}
		const chunks = (results as Array<{ content?: string }>)
			.map(item => item.content || '')
			.filter(text => text.length > 0);
		if (chunks.length === 0) return '';

		const rebuilt = rebuildFromChunks(chunks);
		const admitted = admitBytes(toUTF8Bytes(rebuilt), fileName, 'client-upload', 'normal');
		if (!admitted) return '';

		const drafts = parseDocumentSections(rebuilt);
		if (drafts.length > 0) setResourceSections(admitted.id, drafts);
		setResourceSummary(admitted.id, `（由历史切片重建，${chunks.length} 片 → ${drafts.length} 段）`);

		const index: FileIndex = {
			collection: legacy.collection,
			name: fileName,
			path: fileName,
			contentHash: admitted.content_hash,
			ext: extOf(fileName),
			lang: legacy.lang ?? null,
			chunkCount: drafts.length,
			processedAt: Date.now(),
			version: 2,
			resourceId: admitted.id,
			handle: admitted.handle
		};
		storeFileIndex(admitted.handle, fileName, index);
		console.log(`[阅读者] 历史索引已迁移：${fileName} → ${admitted.handle}（${chunks.length} 片 → ${drafts.length} 段）`);
		return admitted.handle;
	}
	catch (error) {
		console.error(`[阅读者] 迁移历史索引异常 ${fileName}:`, error);
		return '';
	}
}

/** 旧切片路径前缀：`*标题> 路径*` */
const CHUNK_PATH_PREFIX = /^\*[^*]*>\s*([\s\S]*?)\*(?:\n|$)/;

/**
 * 从旧切片反演重建整篇正文
 *
 * 算法：
 *   1. 逐片取出 `*标题> 路径*` 前缀中的层级路径，按 ` / ` 切分为标题段
 *   2. 比较相邻两片的路径，推出新标题相对上一标题的层级（公共前缀长度即父级数）
 *   3. 把标题行按层级重建回 Markdown（`#`×level + 标题），续上该片正文
 *   4. 路径完全相同的连续片段只写一次标题（同一节被拆成多片的情形）
 *
 * @param chunks 旧切片（按检索顺序即原始写入顺序）
 * @returns 重建后的整篇正文
 */
function rebuildFromChunks(chunks: string[]): string {
	const lines: string[] = [];
	let previousSegments: string[] = [];
	for (const chunk of chunks) {
		const match = CHUNK_PATH_PREFIX.exec(chunk);
		const body = match ? chunk.slice(match[0].length) : chunk;
		const path = match ? match[1].trim() : '';
		const segments = path ? path.split('/').map(segment => segment.split('(')[0].trim()).filter(Boolean) : [];

		if (segments.length > 0) {
			// 公共前缀长度决定起始层级；逐段补齐新增的标题行
			let common = 0;
			while (common < segments.length && common < previousSegments.length && segments[common] === previousSegments[common]) common++;
			for (let i = common; i < segments.length; i++) {
				lines.push('#'.repeat(Math.min(i + 1, 6)) + ' ' + segments[i]);
			}
			previousSegments = segments;
		}
		const trimmed = body.replace(/\s+$/, '');
		if (trimmed) lines.push(trimmed);
	}
	return lines.join('\n') + '\n';
}

/**
 * 在正文中定位并替换文件围栏块，返回替换后的文本与已收录的资源句柄
 *
 * @param raw 原始消息文本
 * @returns 替换后文本与句柄列表
 */
async function processImportBlocksInText(raw: string): Promise<{ text: string; imported: string[] }> {
	if (!raw.includes(FENCE)) return { text: raw, imported: [] };
	let out = '';
	let i = 0;
	const imported: string[] = [];
	while (true) {
		const start = raw.indexOf(FENCE, i);
		if (start < 0) {
			out += raw.slice(i);
			break;
		}
		out += raw.slice(i, start);

		// 解析首行文件名（```file.ext）
		const rest = raw.slice(start + FENCE.length);
		const nl = rest.indexOf('\n');
		const eol = nl < 0 ? rest.length : nl;
		const firstLine = rest.slice(0, eol).trim();
		if (!looksLikeFileHeader(firstLine)) {
			// 不是文件块（普通 markdown 代码围栏）→ 原样保留
			out += FENCE;
			i = start + FENCE.length;
			continue;
		}
		// 闭合围栏取整个消息最后一个 ```（正文内嵌的代码块 ``` 不会被误判为收尾）
		const closeIdx = rest.lastIndexOf(FENCE);
		if (closeIdx <= eol) {
			out += raw.slice(start, start + FENCE.length);
			i = start + FENCE.length;
			continue;
		}
		const content = rest.slice(eol + 1, closeIdx);
		const result = await importDocumentBlock(firstLine, content);
		if (result.skipped) {
			// 不支持的类型 → 保留原文
			out += raw.slice(start, start + FENCE.length + closeIdx + FENCE.length);
		}
		else {
			// 收录完成 → 围栏块置换为携带句柄的收录回执（前端据句柄渲染引用条）
			out += `[已收录] ${result.handle} ${firstLine}`;
			if (!imported.includes(result.handle)) imported.push(result.handle);
		}
		i = start + FENCE.length + closeIdx + FENCE.length;
	}
	return { text: out, imported };
}

/** 确保资源的段落表可用（缺失且在位时由 outline 现场补建） */
function ensureSections(handle: string): ResourceSection[] {
	const sections = outlineResource(handle);
	return sections && sections.length > 0 ? sections : [];
}

/** 词法评分：把查询拆为字符与二元组，命中的段落按出现次数加权 */
function lexicalScore(handle: string, section: ResourceSection, query: string): number {
	if (!query) return 0;
	const head = query.trim().toLowerCase();
	if (!head) return 0;
	let score = 0;
	// 标题与路径命中权重最高
	const headText = `${section.title} ${section.path}`.toLowerCase();
	if (headText.includes(head)) score += 10;
	// 正文命中：按行区间取正文（受 SCORE_BODY_CHARS 限制）
	const body = readResource(`${handle}|L${section.line_start}-${section.line_end}`, SCORE_BODY_CHARS);
	const bodyText = (body?.content || '').toLowerCase();
	if (bodyText) {
		if (bodyText.includes(head)) score += 5;
		// 二元组命中：中文场景下比单字更稳定
		for (let i = 0; i + 1 < head.length; i++) {
			const bigram = head.slice(i, i + 2);
			if (bigram.trim().length < 2) continue;
			let from = 0;
			while (true) {
				const found = bodyText.indexOf(bigram, from);
				if (found < 0) break;
				score += 1;
				from = found + bigram.length;
			}
		}
	}
	return score;
}

/**
 * 选择要展开的段落：显式锚点优先，其次按查询词法评分，最后回退前若干段
 *
 * @param handle 资源句柄
 * @param query 用户问题（可为空）
 * @returns 段落列表
 */
function selectSections(handle: string, query: string): ResourceSection[] {
	const sections = ensureSections(handle);
	if (sections.length === 0) return [];
	if (!query || !query.trim()) {
		// 无问题：给开头若干段作为概览，避免一次展开整篇
		return sections.slice(0, 3);
	}
	const scored = sections.map(section => ({ section, score: lexicalScore(handle, section, query) }));
	scored.sort((a, b) => b.score - a.score || a.section.ordinal - b.section.ordinal);
	const picked = scored.filter(item => item.score > 0).slice(0, QUERY_TOP_K).map(item => item.section);
	// 词法全零命中：回退前若干段（保证非空返回，由模型自行判断相关性）
	return picked.length > 0 ? picked : sections.slice(0, 3);
}

/**
 * 展开单个引用：把句柄置换为「资源标题 + 相关段落正文」
 *
 * @param ref 引用块
 * @param query 该引用之后的问题文本
 * @returns 替换文本
 */
function expandReference(ref: Ref, query: string): string {
	const parsed = parseHandle(ref.handle);
	if (!parsed) return ref.raw;

	// 带行锚点：直接整段取回
	if (parsed.anchor === 'line') {
		const result = readResource(ref.handle);
		if (!result) return ref.raw;
		if (result.lost) return `${ref.handle}\n${degradeNotice(ref.handle, '内容已不可取，仅保留目录条目')}`;
		return `【${ref.handle}｜${result.title}｜第 ${result.line_start}-${result.line_end} 行】\n${result.content || ''}`;
	}

	// 目录里查不到该句柄：原样保留，等对话者处理
	const exists = listResources(parsed.kind, 500).some(entry => entry.handle === ref.handle.replace(/\|.*$/, ''));
	if (!exists && parsed.kind !== 'D' && parsed.kind !== 'T') {
		// 非文本资源：取回描述（图片/视频/音频），失败则原样保留
		const described = describeResource(ref.handle, query);
		return described ? `【${ref.handle}】${described}` : ref.raw;
	}
	if (!exists) return ref.raw;

	// 带章节锚点：直接整段取回
	if (parsed.anchor === 'section') {
		const result = readResource(ref.handle);
		if (!result) return ref.raw;
		if (result.lost) return `${ref.handle}\n${degradeNotice(ref.handle, '内容已不可取，仅保留目录条目')}`;
		const label = result.section_title ? `｜${result.section_title}` : '';
		return `【${ref.handle}${label}｜第 ${result.line_start}-${result.line_end} 行】\n${result.content || ''}`;
	}

	// 无锚点的文档引用：按问题选段后整段展开
	const sections = selectSections(ref.handle, query);
	if (sections.length === 0) {
		// 没有段落结构（未收录段落表 / 非 Markdown）：仅在体积可控时才注入全文，
		// 否则只给资源卡片，把「读多少」的决定权交回模型（read_resource）
		return expandFlatResource(ref, query);
	}
	const parts: string[] = [];
	let budget = QUERY_CHAR_BUDGET;
	for (const section of sections) {
		if (budget <= 0) break;
		const slice = readResource(`${ref.handle.replace(/\|.*$/, '')}|L${section.line_start}-${section.line_end}`, budget);
		const body = slice?.content || '';
		if (!body) continue;
		budget -= body.length;
		const label = section.path || section.title || `第 ${section.ordinal} 节`;
		parts.push(`【${ref.handle.replace(/\|.*$/, '')}|§${section.ordinal}｜${label}｜第 ${section.line_start}-${section.line_end} 行】\n${body}`);
	}
	if (parts.length === 0) return ref.raw;
	return parts.join('\n---\n');
}

/**
 * 无段落结构资源的展开：小文件注入全文，大文件只给资源卡片
 *
 * 为什么要有体积闸门：引用展开发生在思考链的上下文构建阶段，注入多少就直接
 * 占用多少 token。此前对无段落结构的资源无条件注入全文 —— 一份大配置/日志被引用
 * 时会整篇灌进上下文。超过 FASTAIL_CHAR_LIMIT 时改为只报资源信息，由模型按需
 * 调 read_resource 读取，成本可控且不丢可寻址性。
 *
 * @param ref 引用块
 * @param query 该引用之后的问题文本（透传给描述能力）
 * @returns 替换文本
 */
function expandFlatResource(ref: Ref, query: string): string {
	const handle = ref.handle.replace(/\|.*$/, '');
	const entry = listResources('', 500).find(item => item.handle === handle);
	const result = readResource(handle, FULLTEXT_CHAR_LIMIT + 1);
	if (!result) return ref.raw;
	if (result.lost) return `${handle}\n${degradeNotice(handle, '内容已不可取，仅保留目录条目')}`;
	const content = result.content || '';

	// 非文本资源（图片/视频/音频）：走描述能力
	if (!content) {
		const described = describeResource(handle, query);
		return described ? `【${handle}】${described}` : ref.raw;
	}
	// 小资源：直接注入全文（对话体验最好，成本可控）
	if (content.length <= FULLTEXT_CHAR_LIMIT && !result.truncated) {
		return `【${handle}｜${result.title}｜共 ${result.total_lines || 0} 行】\n${content}`;
	}
	// 大资源：只给卡片 + 结构提示，正文按需取
	const sizeNote = `约 ${content.length} 字符 / ${result.total_lines || 0} 行`;
	const sectionNote = entry?.section_count ? `，${entry.section_count} 节` : '';
	const summary = entry?.summary ? `\n摘要：${entry.summary}` : '';
	return `【${handle}｜${result.title}｜${sizeNote}${sectionNote}】体积较大，未自动注入正文。`
		+ `${summary}\n需要时请用 read_resource 按句柄读取（可带 |§n 章节锚点或 |L<起>-<止> 行区间）`
		+ `${entry?.section_count ? '，先用 outline_resource 查看结构再定向读取' : ''}。`;
}

/** 匹配正文中的资源句柄引用（含锚点） */
const HANDLE_REF = /@[DIVAT]\.\d{3,}(?:\|(?:§\d+|L\d+-\d+))?/g;

/**
 * 解析自由文本引用（文件名形态）为资源句柄
 *
 * 顺序（与用户心智一致：先已有索引，再已有目录，最后按需收录 / 历史迁移）：
 *   1. 按来源路径查索引（`@path:` 键）—— 同一份文件重复引用必须在这里就命中，
 *      否则每次引用都会再收录一遍
 *   2. 资源目录中已有的同名资源（内容哈希与来源路径都对得上才复用）
 *   3. 按需收录：工作区相对路径 → LocalDir 相对路径
 *   4. 历史 file_mapping 键 → 现场迁移重建为整篇资源
 *
 * 第 3 步是「用户在对话里点名一个从未上传过的文件」的路径：
 * 例如 `@AGENTS.md`、`@subsystem/build_common.ps1`。
 * Go 侧对工作区来源只复制/硬链接，**绝不移动或删除用户的项目文件**。
 *
 * @param fileName 文件名或相对路径（可带目录前缀）
 * @returns 规范句柄；无法解析返回空串
 */
function resolveFreeTextReference(fileName: string): string {
	const target = String(fileName || '').replace(/^#/, '').trim();
	if (!target) return '';

	// 1. 来源路径索引：完全相同的引用直接复用既有句柄（零 IO、零新增目录行）
	const indexed = loadFileIndex('', undefined, target);
	if (indexed?.handle && indexed.version === 2) return indexed.handle;

	// 2. 目录中已有同名资源：仅在内容哈希与索引一致时才复用，避免把同名不同路径的文件混为一谈
	const candidates = listResources('', 500).filter(item => item.name === target);
	if (candidates.length > 0) {
		const samePath = indexed?.contentHash
			? candidates.find(item => item.content_hash === indexed.contentHash)
			: undefined;
		if (samePath) return samePath.handle;
		// 文件已收录但索引缺失（例如索引表被清理）：补齐索引后复用首个同名条目
		if (!indexed && candidates.length === 1) {
			storeFileIndex(candidates[0].handle, target, {
				collection: '',
				name: target,
				path: target,
				contentHash: candidates[0].content_hash,
				ext: extOf(target),
				lang: CODE_LANGS[extOf(target)] ?? null,
				chunkCount: candidates[0].section_count,
				processedAt: Date.now(),
				version: 2,
				resourceId: candidates[0].id,
				handle: candidates[0].handle
			});
			return candidates[0].handle;
		}
	}

	// 3. 按需收录（工作区优先，其次 LocalDir）
	const admitted = admitDocumentByName(target);
	if (admitted) {
		const content = readResource(admitted.handle, 4 * 1024 * 1024)?.content || '';
		const drafts = parseDocumentSections(content);
		if (drafts.length > 0) setResourceSections(admitted.id, drafts);
		storeFileIndex(admitted.handle, target, {
			collection: '',
			name: target,
			path: target,
			contentHash: admitted.content_hash,
			ext: extOf(target),
			lang: CODE_LANGS[extOf(target)] ?? null,
			chunkCount: drafts.length,
			processedAt: Date.now(),
			version: 2,
			resourceId: admitted.id,
			handle: admitted.handle
		});
		console.log(`[阅读者] 按需收录 ${admitted.handle}（${target}，${drafts.length} 段）`);
		return admitted.handle;
	}

	// 4. 历史索引懒迁移：只在真正被引用时重建，避免启动期集中反演全部旧集合
	return migrateLegacyFileIndex(target);
}

/** 匹配前端下发的文件名引用标记（`@文件名.ext`，排除邮箱形态） */
const FILE_REF = /(?<![\w.@])@([A-Za-z0-9_\-.]+\.\w{1,8})/g;

/**
 * 解析消息文本中的资源引用并展开为正文
 *
 * 支持三种引用形态：
 *   1. `@D.003` / `@D.003|§4` / `@D.003|L120-168`：直接句柄（用户手写或前端回执带回）
 *   2. `@文件名.ext`：前端下发的文件名引用，按资源目录解析为句柄后再展开
 *   3. `#[文件名.ext]`：历史形态，自动迁移到资源目录
 *
 * 查询切分与旧实现一致：每个引用之后到下一个引用之前的问题文本即该引用的检索条件。
 *
 * @param raw 原始消息文本
 * @returns 展开后的文本与是否发生变化
 */
export function processReferencesInText(raw: string): { text: string; changed: boolean } {
	if (!raw || (!raw.includes('@') && !raw.includes('(#'))) return { text: raw, changed: false };

	// 事件 -> 阅读文件前：推送原文，插件可经 return.text 改写待阅读消息文本后再解析引用
	const feedback: { text?: string } = interactEvent('read_file_before', { text: raw }).return;
	if (feedback && typeof feedback.text === 'string' && feedback.text !== raw) raw = feedback.text;

	const refs: Ref[] = [];

	// 形态 1：直接句柄
	HANDLE_REF.lastIndex = 0;
	let match: RegExpExecArray | null;
	while ((match = HANDLE_REF.exec(raw)) !== null) {
		const normalized = formatHandle(parseHandle(match[0])!);
		refs.push({ raw: match[0], start: match.index, end: match.index + match[0].length, handle: normalized });
	}

	// 形态 2：前端下发的文件名引用（在句柄之外的区域匹配，避免重复）
	FILE_REF.lastIndex = 0;
	while ((match = FILE_REF.exec(raw)) !== null) {
		const start = match.index;
		const end = start + match[0].length;
		if (refs.some(ref => start < ref.end && end > ref.start)) continue;
		const handle = resolveFreeTextReference(match[1]);
		if (!handle) continue;
		refs.push({ raw: match[0], start, end, handle });
	}

	// 形态 3：历史 `(#文件名.ext)` 引用
	const LEGACY_REF = /\(#([\w.\-]+)\)/g;
	while ((match = LEGACY_REF.exec(raw)) !== null) {
		const start = match.index;
		const end = start + match[0].length;
		if (refs.some(ref => start < ref.end && end > ref.start)) continue;
		const handle = resolveFreeTextReference(match[1]);
		if (!handle) continue;
		refs.push({ raw: match[0], start, end, handle });
	}

	if (refs.length === 0) return { text: raw, changed: false };
	refs.sort((a, b) => a.start - b.start);

	const parts: string[] = [];
	// 保留首个引用之前的引导语句（用户的整体指示），避免意图丢失
	const lead = raw.slice(0, refs[0].start).trim();
	if (lead) parts.push(lead + '\n');

	let changed = false;
	for (let i = 0; i < refs.length; i++) {
		const ref = refs[i];
		// 该引用的问题文本 = 引用块后到下一个引用块前（去头尾、去首个冒号）。
		// 它只作为「按问题选段」与「定向描述」的检索条件，**不再拼回上下文**：
		// 用户的问题本来就在这条消息里（紧跟引用之后），另行追加会造成同一句话出现两次。
		const gapStart = ref.end;
		const gapEnd = i + 1 < refs.length ? refs[i + 1].start : raw.length;
		let query = raw.slice(gapStart, gapEnd).trim();
		query = query.replace(/^[：:]/, '').trim();

		const expanded = expandReference(ref, query);
		if (expanded !== ref.raw) changed = true;
		parts.push(expanded);
		// 问题原文原样留在展开内容之后（引用原位替换、问题原位保留）
		if (query) parts.push('\n' + query + '\n');
	}

	return { text: parts.join('').trimEnd(), changed };
}

/** 处理单条消息中的文件导入块与引用，返回是否发生改变 */
async function processMessage(message: PostMessage): Promise<{ changed: boolean; imported: string[] }> {
	// 纯文本消息
	if (typeof message.content === 'string') {
		const original = message.content;
		const imported = await processImportBlocksInText(original);
		const ref = processReferencesInText(imported.text);
		message.content = ref.text;
		return { changed: message.content !== original, imported: imported.imported };
	}
	// 多模态消息：提取文本项处理，保留图片项
	if (Array.isArray(message.content)) {
		const textParts = message.content.filter(it => it.type === 'text').map(it => (it as TextContent).text);
		const others = message.content.filter(it => it.type !== 'text');
		if (textParts.length === 0) return { changed: false, imported: [] };
		const raw = textParts.join('\n');
		const imported = await processImportBlocksInText(raw);
		const ref = processReferencesInText(imported.text);
		message.content = [...others, { type: 'text', text: ref.text }];
		return { changed: true, imported: imported.imported };
	}
	return { changed: false, imported: [] };
}

/**
 * 阅读者主流程：处理 unreadContext 中的文件导入块与引用。
 * 在对话者 createChatMessage 之前调用。
 */
export async function processUnreadFiles(): Promise<void> {
	// 处理所有消息中的文件导入块与引用；前端已在发送时自携带文件名引用，无需再推送识别 ID
	for (const message of GlobalUnreadContext()) {
		try {
			await processMessage(message);
		}
		catch (error) {
			console.error('[阅读者] 处理消息失败:', error);
		}
	}
}

/** 取当前未读上下文（经 global 配置间接访问，避免与 loop 形成静态循环） */
function GlobalUnreadContext(): PostMessage[] {
	return GlobalConfig.unreadContext;
}

/** 句柄文法速查（供提示词与工具描述引用，避免各处硬编码） */
export const RESOURCE_GRAMMAR_CHEATSHEET = grammarCheatsheet();

// 说明：文件收录回执的文案与前端渲染约定见 assets/client/js/websocket.js 的 [已收录] 解析；
// 修改此处文案必须同步前端解析正则。
export const IMPORT_RECEIPT_PREFIX = '[已收录]';
