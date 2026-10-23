// ===== 对话缓存存储：载入 / 追加 / 落盘 / 还原为上下文 =====

import type { PostMessage } from '../../config/model';
import type { CachedMessage } from './types';
import { getPromptFromKnowledgeAny } from '../knowledge';
import { fileNameOf, saveBase64Media, splitUserPrefix, toDisplaySource, toVisionSource } from './codec';

/** 缓存文件（相对 LocalDir） */
const CACHE_FILE = 'database/messages.json';
/** 缓存条数上限：与前端 MAX_CACHED_MESSAGES 保持一致 */
export const MAX_CACHE_MESSAGES = 48;

/** 缓存内存副本（与文件内容一一对应） */
let cachedMessages: CachedMessage[] = [];
/** 上次成功写盘的序列化快照，避免同一内容重复写盘 */
let lastSnapshot = '';
/** 历史图片动态图判定缓存：避免每轮都解码几十 MB 的 GIF */
const animatedHistoryCache = new Map<string, boolean>();

/** 生成消息 ID（时间戳 + 随机后缀） */
function newId(): string {
	return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
}

/** 截断到缓存上限（只保留最新消息） */
function trimCache(): void {
	if (cachedMessages.length > MAX_CACHE_MESSAGES) cachedMessages = cachedMessages.slice(-MAX_CACHE_MESSAGES);
}

/**
 * 修复历史坏图片地址：/file/read/@I.00x（把资源句柄当成了文件路径）
 *
 * 资源登记入口曾把 (canonicalRel, handle) 的返回顺序写反，pushImage 因此把句柄拼进
 * /file/read/，前端与 QQ 适配器都会 404（QQ 侧报「下载文件失败: Not Found」）。
 * 载入缓存时按句柄回查真实 blob_path；查不到就保留原样（前端显示占位，不影响启动）。
 */
function repairHandleImageURL(source: string): string {
	const match = /^\/file\/read\/(@[DIVAT]\.\d{3,})\/?$/.exec(String(source || ''));
	if (!match) return source;
	try {
		// 只需要 blob_path：maxBytes=1 避免把整张图读进 JS 侧
		const [resource, error] = resourceRead(match[1], 1);
		if (!error && resource && !resource.lost && resource.blob_path) return '/file/read/' + resource.blob_path;
	}
	catch (error) {
		console.warn('[对话缓存] 句柄图片地址修复失败:', error);
	}
	return source;
}

/** 追加一条缓存消息（不立即写盘） */
export function appendCachedMessage(record: CachedMessage): void {
	cachedMessages.push(record);
	trimCache();
}

/** 把当前缓存写入文件（内容未变化时跳过） */
export function persistConversation(): void {
	trimCache();
	const text = JSON.stringify(cachedMessages);
	if (text === lastSnapshot) return;
	try {
		const [, , error] = saveFile(CACHE_FILE, true, text);
		if (error) {
			console.warn('[对话缓存] 写入失败:', error);
			return;
		}
		lastSnapshot = text;
	}
	catch (error) {
		console.warn('[对话缓存] 写入异常:', error);
	}
}

/** 规整缓存条目（容忍旧版前端写入的缺省字段） */
function normalizeRecord(raw: any): CachedMessage | null {
	if (!raw || (raw.role !== 'user' && raw.role !== 'assistant')) return null;
	let content = typeof raw.content === 'string' ? raw.content : '';
	let sender = typeof raw.sender === 'string' ? raw.sender.trim() : '';
	// 旧缓存可能只有正文前缀、没有 sender 字段：补出 sender 供前端展示，
	// 正文保持原样（前缀留在缓存与模型历史里，前端渲染时再隐藏）。
	const parsed = splitUserPrefix(content);
	if (!sender && parsed.sender) sender = parsed.sender;
	return {
		id: typeof raw.id === 'string' && raw.id ? raw.id : newId(),
		role: raw.role,
		categories: Array.isArray(raw.categories) ? raw.categories.map(String) : ['text'],
		content: content,
		sender: sender || undefined,
		attachments: Array.isArray(raw.attachments) ? raw.attachments : undefined,
		imageSrc: typeof raw.imageSrc === 'string' ? raw.imageSrc : undefined,
		imageLabel: typeof raw.imageLabel === 'string' ? raw.imageLabel : undefined,
		audio: typeof raw.audio === 'string' ? raw.audio : undefined,
		abcNotation: typeof raw.abcNotation === 'string' ? raw.abcNotation : undefined,
		actionType: typeof raw.actionType === 'string' ? raw.actionType : undefined,
		fileRef: typeof raw.fileRef === 'string' ? raw.fileRef : undefined,
		fileLabel: typeof raw.fileLabel === 'string' ? raw.fileLabel : undefined,
		timestamp: Number(raw.timestamp) || Date.now(),
	};
}

/** 构造动图理解缓存的候选键：同一资源可能以 read/miniature、带 tag 或相对路径缓存 */
function animatedSummaryKeys(source: string): string[] {
	const raw = String(source || '');
	if (!raw) return [];
	const normalize = (value: string): string => String(value || '').split('?')[0].split('#')[0];
	const vision = toVisionSource(raw);
	const keys = new Set<string>();
	for (const value of [raw, vision, normalize(raw), normalize(vision)]) {
		if (value) keys.add(value);
	}
	const path = normalize(raw);
	const marker = path.indexOf('/multimedia/');
	if (marker >= 0) {
		const rel = path.slice(marker + 1); // multimedia/xxx.gif
		keys.add(rel);
		keys.add('/file/read/' + rel);
		keys.add('/file/miniature/' + rel);
		keys.add('file://' + rel);
		try {
			const base = url()[0];
			if (base) {
				keys.add(base + '/file/read/' + rel);
				keys.add(base + '/file/miniature/' + rel);
			}
		}
		catch { /* url() 不可用时只查相对键 */ }
	}
	return [...keys];
}

/** 把一条缓存消息还原成对话者上下文消息（月华自己的图片不回读） */
function cachedToContext(record: CachedMessage): PostMessage | null {
	if (record.role === 'user') {
		const parsed = splitUserPrefix(record.content);
		const sender = (record.sender || parsed.sender).trim();
		const text = record.content;
		// 正文本身已带前缀时不再重复添加；旧缓存只有 sender、正文已净化时补回前缀
		const prefix = sender && !parsed.sender ? `[用户: ${sender}]: ` : '';
		const blocks: any[] = [];
		if (text) blocks.push({ type: 'text', text: prefix + text });
		else if (sender) blocks.push({ type: 'text', text: `[用户: ${sender}]:` });
		for (const attachment of record.attachments || []) {
			if (attachment.type === 'image') {
				// 只处理本机视觉端点可承载的图片；远程 URL / data: / blob: 维持原有的跳过策略，
				// 避免在载入历史时发起网络请求或把原图塞进上下文。
				const vision = toVisionSource(attachment.src);
				if (!vision) continue;
				// 历史动图不能直接回读：/file/miniature 对动图原样直通，会把几十 MB 的 GIF
				// 交给多模态服务，llama-server 拉取/解码失败时整轮模型请求 500
				// （error: cannot make GET request）。历史动图只保留文本占位；
				// 当前新消息由 LiteImageFile 原位置换为句柄，内容描述按需由句柄系统生成。
				const [animated, animatedError] = animatedHistoryCache.has(attachment.src)
					? [animatedHistoryCache.get(attachment.src) === true, null] as [boolean, Error | null]
					: isAnimatedImage(attachment.src);
				if (!animatedError && !animatedHistoryCache.has(attachment.src)) {
					animatedHistoryCache.set(attachment.src, animated);
				}
				if (animatedError) {
					blocks.push({ type: 'text', text: `[历史图片不可用：${attachment.label || fileNameOf(attachment.src)}]` });
					continue;
				}
				if (animated) {
					// 历史动图不回传原图（GIF 太大且会让多模态服务 500），
					// 旧版理解缓存命中时把文本带回上下文，没命中则只留标记。
					const summary = (getPromptFromKnowledgeAny(animatedSummaryKeys(attachment.src)) || '').trim();
					const text = summary
						? `[动态图: ${summary.length > 1200 ? summary.slice(0, 1200) + '…' : summary}]`
						: '[动态图]';
					blocks.push({ type: 'text', text });
					continue;
				}
				// 只接受视觉读取端点形态；data:/blob: 等原图来源一律跳过
				blocks.push({ type: 'image_url', image_url: { url: vision } });
			}
			else if (attachment.type === 'video') {
				const display = toDisplaySource(attachment.src);
				if (display.startsWith('/file/read/')) blocks.push({ type: 'video_url', video_url: { url: url()[0] + display } });
			}
		}
		if (!blocks.length) return text ? { role: 'user', content: text } : null;
		if (blocks.length === 1 && blocks[0].type === 'text') return { role: 'user', content: blocks[0].text };
		return { role: 'user', content: blocks };
	}
	// 月华自己输出的图片/动作/乐谱只用于前端展示，不回读上下文
	if (record.categories.includes('image') || record.categories.includes('action') || record.abcNotation) return null;
	const text = String(record.content || '').trim();
	if (!text) return null;
	return { role: 'assistant', content: text };
}

/** 读取缓存文件并载入对话者上下文 */
export function initConversationCache(loadContext: (messages: PostMessage[]) => void): void {
	try {
		const [content, error] = readTextFile(CACHE_FILE);
		if (error || !content) return;
		const parsed = JSON.parse(content);
		if (!Array.isArray(parsed)) return;
		cachedMessages = parsed
			.map(normalizeRecord)
			.filter((item): item is CachedMessage => !!item)
			.slice(-MAX_CACHE_MESSAGES);
		// 旧版前端把月华图片存成 data URI：迁移成 /file/read/ 文件地址，避免缓存越写越大；
		// 同时修复历史回归留下的 /file/read/@I.00x 句柄地址（见 repairHandleImageURL）
		let migrated = false;
		for (const record of cachedMessages) {
			if (record.imageSrc) {
				const fixed = repairHandleImageURL(record.imageSrc);
				if (fixed !== record.imageSrc) {
					record.imageSrc = fixed;
					migrated = true;
				}
			}
			for (const attachment of record.attachments || []) {
				if (attachment.type !== 'image' || !attachment.src) continue;
				const fixed = repairHandleImageURL(attachment.src);
				if (fixed !== attachment.src) {
					attachment.src = fixed;
					migrated = true;
				}
			}
			if (record.role === 'assistant' && record.imageSrc && record.imageSrc.startsWith('data:image/')) {
				const saved = saveBase64Media(record.imageSrc, 'image/jpeg', 'image');
				if (saved) {
					record.imageSrc = saved;
					migrated = true;
				}
			}
		}
		lastSnapshot = migrated ? '' : JSON.stringify(cachedMessages);
		if (migrated) persistConversation();
		const context = cachedMessages
			.map(cachedToContext)
			.filter((item): item is PostMessage => !!item);
		if (context.length) loadContext(context);
		console.log(`[对话缓存] 已载入 ${context.length} 条近期记忆`);
	}
	catch (error) {
		console.warn('[对话缓存] 载入失败:', error);
	}
}
