import { GlobalConfig } from '../config/global';
import { PostMessage, TextContent } from '../config/model';
import { getFileIndexFromKnowledge, saveFileIndexToKnowledge } from '../file/knowledge';
import { splitTextToStrings } from '../file/split';
import { resolveCodeLang, extractCodeTags } from '../file/code_split';

/** 阅读者智能体：处理用户推送的长文本/文本文件，切片入库，并在被引用时查询整理 */

/** 可入库的文件扩展名白名单 */
const FILE_WHITELIST = ['ts','js','tsx','jsx','go','py','java','c','h','cpp','cxx','hpp','cs','md','txt','json'];
/** 切片理想长度（后端无前端 Slider，按默认 1024 处理） */
const SLICE_LEN = 1024;
/** 查询时每个文件返回的片段数 */
const QUERY_TOP_K = 10;
/** 文件围栏块起始标记：```file.ext */
const FENCE = '```';

/** 文件索引结构（存于 FileMapping 表，键为识别ID #fileName.ext） */
type FileIndex = {
	collection: string;
	name: string;
	ext: string;
	lang: string | null;
	chunkCount: number;
	processedAt: number;
};

/** 确定性内容哈希（FNV-1a 双桶→16位hex），用于生成稳定的文件集合名 */
function contentHash(input: string): string {
	const s = input.replace(/^\uFEFF/, '');
	let h1 = 0xdeadbeef, h2 = 0x41c6ce57;
	for (let i = 0; i < s.length; i++) {
		const c = s.charCodeAt(i);
		h1 = Math.imul(h1 ^ c, 2654435761);
		h2 = Math.imul(h2 ^ c, 1597334677);
		h1 = (h1 << 13) | (h1 >>> 19);
		h2 = (h2 << 7) | (h2 >>> 25);
	}
	return (h1 >>> 0).toString(16).padStart(8, '0') + (h2 >>> 0).toString(16).padStart(8, '0');
}

/** 从文件名取扩展名（小写，无点） */
function extOf(fileName: string): string {
	const t = (fileName || '').split('.').pop() || '';
	return t.toLowerCase();
}

/** 判断某文本是否像文件围栏块的开头 */
function looksLikeFileHeader(firstLine: string): boolean {
	return /^[A-Za-z0-9_\-.]+\.\w+$/.test(firstLine.trim());
}

/** 从切片片段首行解析出层级路径（如 `class Foo / func Bar`） */
function pathFromChunk(chunk: string): string {
	const m = /^\*[^*]*>\s*([\s\S]*?)\*(?:\n|$)/.exec(chunk);
	return m ? m[1].trim() : '';
}

/** 为代码切片生成启发式标签（路径标识 + 语言 + 文件名） */
function tagsForCodeChunk(chunk: string, codeLang: string, fileName: string): string[] {
	const path = pathFromChunk(chunk);
	const name = (fileName || '').replace(/\.[^.]+$/, '') || fileName;
	const tags = extractCodeTags(path, codeLang as Parameters<typeof extractCodeTags>[1]);
	if (name && !tags.includes(name)) tags.push(name);
	if (name && !tags.includes(name.toLowerCase())) tags.push(name.toLowerCase());
	return tags;
}

/** 为文本/MD 切片生成启发式标签（标题路径分段 + 文件名 + 类型，零 LLM） */
function tagsForTextChunk(chunk: string, fileName: string, fileExt: string): string[] {
	const tags: string[] = [(fileExt || 'text').toLowerCase()];
	const base = (fileName || '').replace(/\.[^.]+$/, '') || fileName;
	if (base) tags.push(base);
	// 从 `*标题> 路径*` 前缀的标题层级路径提取各段标题做标签
	const path = pathFromChunk(chunk);
	if (path) {
		for (const seg of path.split('/')) {
			const name = seg.split('(')[0].trim();
			if (name && name.length <= 64) tags.push(name);
		}
	}
	// 去重
	return [...new Set(tags)];
}

/** 判断集合中是否已有文档（用于文件级去重，防止索引残留指向已删除的空集合） */
function collectionHasData(collection: string): boolean {
	try {
		const [results, err] = memoryQuery(collection, '文件内容 结构 概览 摘要', 1);
		return !err && Array.isArray(results) && results.length > 0;
	} catch {
		return false;
	}
}

/** 导入单个文件围栏块：切片→入库→登记索引；返回识别ID */
async function importFileBlock(fileName: string, content: string): Promise<{ id: string; skipped: boolean }> {
	const ext = extOf(fileName);
	// 白名单校验：不支持的扩展名跳过（交由普通文本透传），raw 由调用方保留原样
	if (!FILE_WHITELIST.includes(ext)) return { id: '', skipped: true };

	// 识别代码语言；非代码则按 MD/普通文本拆分（启发式标签，零 LLM）
	const codeLang = resolveCodeLang(ext);
	const key = `#${fileName}`;

	// 集合名 = file_ + 内容哈希（内容相同自动复用，天然合法且规避文件名非法字符）
	const collection = 'file_' + contentHash(content);

	// 幂等初始化集合（已存在则直接打开，不清空数据）
	const [initOk, initErr] = memoryInit(collection, 'text');
	if (!initOk) {
		console.error(`[阅读者] 集合初始化失败 ${collection}:`, initErr);
		return { id: '', skipped: true };
	}

	// 文件级去重：仅当该索引指向的集合实际仍有文档时才复用。
	// 否则（集合被删除 / 首次导入）清除可能残留的旧索引并重新入库，
	// 避免“索引还在但集合已空”导致重导一直被跳过、记忆库变空。
	if (getFileIndexFromKnowledge(key) && collectionHasData(collection)) return { id: key, skipped: false };
	saveFileIndexToKnowledge(key, '');

	// 切片：指定 lang 时走代码感知拆分（捕获 Function/Class），否则走 MD/普通文本拆分
	const chunks = splitTextToStrings(content, { idealLen: SLICE_LEN, lang: codeLang || undefined });

	// 逐片入库：代码与文本/MD 统一走启发式标签（memoryAddWithTags，零 LLM）
	// 单片失败/抛异常只跳过该片并继续，避免中途中断导致整文件只存前几片
	let written = 0;
	for (const chunk of chunks) {
		try {
			if (codeLang) {
				const tags = tagsForCodeChunk(chunk, codeLang, fileName);
				const [, err] = memoryAddWithTags(collection, 'user', chunk, tags);
				if (err) { console.error(`[阅读者] 写入代码切片失败:`, err); continue; }
			} else {
				const tags = tagsForTextChunk(chunk, fileName, ext);
				const [, err] = memoryAddWithTags(collection, 'user', chunk, tags);
				if (err) { console.error(`[阅读者] 写入文本切片失败:`, err); continue; }
			}
			written++;
		} catch (error) {
			console.error(`[阅读者] 切片写入抛异常（已跳过该片，继续后续入库）:`, error);
		}
	}

	// 登记索引：识别ID → 索引信息（FileMapping 精确键）
	const index: FileIndex = { collection, name: fileName, ext, lang: codeLang, chunkCount: written, processedAt: Date.now() };
	saveFileIndexToKnowledge(key, JSON.stringify(index));

	console.log(`[阅读者] 已导入 ${key} → 集合 ${collection}，切片 ${written} 片`);
	return { id: key, skipped: false };
}

/** 在原文本中定位并替换文件围栏块，返回替换后文本与导入的文件ID列表 */
async function processImportBlocksInText(raw: string): Promise<{ text: string; imported: string[] }> {
	if (!raw.includes(FENCE)) return { text: raw, imported: [] };
	let out = '';
	let i = 0;
	const imported: string[] = [];
	while (true) {
		const start = raw.indexOf(FENCE, i);
		if (start < 0) { out += raw.slice(i); break; }
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
			// 未闭合 → 视为普通文本
			out += raw.slice(start, start + FENCE.length);
			i = start + FENCE.length;
			continue;
		}
		const content = rest.slice(eol + 1, closeIdx);
		const res = await importFileBlock(firstLine, content);
		if (res.skipped) {
			// 不支持的类型 → 保留原文
			out += raw.slice(start, start + FENCE.length + closeIdx + FENCE.length);
		} else {
			// 读取完成 → 将围栏块消息替换为通知文本（前端已在发送时自构造 (#id) 引用，无需再推送）
			out += `月华收到了${firstLine}文件`;
			if (res.id && !imported.includes(res.id)) imported.push(res.id);
		}
		i = start + FENCE.length + closeIdx + FENCE.length;
	}
	return { text: out, imported };
}

/** 解析消息文本中的 `(#file.ext)` 引用，并调用阅读者查询整理 */
function processReferencesInText(raw: string): { text: string; changed: boolean } {
	if (!raw.includes('(#')) return { text: raw, changed: false };

	// 收集所有引用块 (id)，id 为 fileName.ext 内部名（不含#与括号）
	type Ref = { id: string; start: number; end: number };
	const refs: Ref[] = [];
	const re = /\(#([\w.-]+)\)/g;
	let m: RegExpExecArray | null;
	while ((m = re.exec(raw))) refs.push({ id: m[1], start: m.index, end: m.index + m[0].length });
	if (refs.length === 0) return { text: raw, changed: false };

	let changed = false;
	const parts: string[] = [];
	// 保留首个引用之前的引导语句（用户的整体指示），避免意图丢失
	const lead = raw.slice(0, refs[0].start).trim();
	if (lead) parts.push(lead + "\n");
	for (let i = 0; i < refs.length; i++) {
		const ref = refs[i];
		// 该引用的问题文本 = 引用块后到下一个引用块前（去头尾、去首个冒号）
		const gapStart = ref.end;
		const gapEnd = i + 1 < refs.length ? refs[i + 1].start : raw.length;
		let query = raw.slice(gapStart, gapEnd).trim();
		query = query.replace(/^[：:]/, '').trim();

		// 读取文件索引
		const key = `#${ref.id}`;
		let index: FileIndex | null = null;
		const cached = getFileIndexFromKnowledge(key);
		if (cached) { try { index = JSON.parse(cached); } catch { index = null; } }

		if (!index) {
			// 未知文件引用 → 保留原样，等待对话者处理
			parts.push(raw.slice(ref.start, gapEnd));
			continue;
		}

		// 查询该文件集合，整理相关片段
		const [ok2] = memoryInit(index.collection, 'text');
		if (!ok2) { parts.push(raw.slice(ref.start, gapEnd)); continue; }
		const [results, qErr] = memoryQuery(index.collection, query || '文件总体内容概括', QUERY_TOP_K);
		const snippets = qErr ? [] : ((results || []) as Array<{ content?: string }>)
			.map(r => (r.content || '').trim()).filter(Boolean);
		const joined = snippets.length > 0 ? snippets.join('\n---\n') : '（无相关片段）';

		let block = `【文件 ${key}】\n${joined}\n`;
		if (query) block += `\n用户问题：${query}\n`;
		parts.push(block);
		changed = true;
	}

	return { text: parts.join('').trimEnd(), changed };
}

/** 处理单条消息中的文件导入块与引用，返回是否发生改变 */
async function processMessage(message: PostMessage): Promise<{ changed: boolean; imported: string[] }> {
	// 纯文本消息
	if (typeof message.content === 'string') {
		const res = await processImportBlocksInText(message.content);
		const ref = processReferencesInText(res.text);
		if (res.text !== ref.text) message.content = ref.text;
		return { changed: res.text !== message.content || ref.changed, imported: res.imported };
	}
	// 多模态消息：提取文本项处理，保留图片项
	if (Array.isArray(message.content)) {
		const textParts = message.content.filter(it => it.type === 'text').map(it => (it as TextContent).text);
		const others = message.content.filter(it => it.type !== 'text');
		if (textParts.length === 0) return { changed: false, imported: [] };
		const raw = textParts.join('\n');
		const imp = await processImportBlocksInText(raw);
		const ref = processReferencesInText(imp.text);
		const newText = ref.text;
		if (newText !== raw || imp.imported.length > 0) {
			message.content = [...others, { type: 'text', text: newText }];
		}
		return { changed: true, imported: imp.imported };
	}
	return { changed: false, imported: [] };
}

/**
 * 阅读者主流程：处理 unreadContext 中的文件导入块与引用。
 * 在对话者 createChatMessage 之前调用。
 */
export async function processUnreadFiles(): Promise<void> {
	// 处理所有消息中的文件导入块与引用；前端已在发送时自携带 (#id):问题，无需再推送识别ID
	for (const message of GlobalConfig.unreadContext) {
		try {
			await processMessage(message);
		} catch (error) {
			console.error('[阅读者] 处理消息失败:', error);
		}
	}
}