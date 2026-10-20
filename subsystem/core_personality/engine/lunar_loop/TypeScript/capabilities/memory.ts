import { GlobalConfig } from '../config/global';
import { PostMessage } from '../config/model';
import { RandomFloor } from '../tools/math';
import { saveBase64Media } from '../io/conversation/codec';

/** 记忆类型：文本 / 图像 / 音频（查询只按类型过滤，不再按集合名过滤） */
export type MemoryMediaType = 'text' | 'image' | 'audio';

/** 动作记忆的来源标记：写入时打标，动作匹配时优先取这一类文本记忆 */
const ACTION_SOURCE = 'action';

/** 跨集合记忆检索命中 */
export interface MemorySearchHit {
    /** 记忆文档 ID */
    id: string;
    /** 记忆类型 */
    type: string;
    /** 记忆角色（user / assistant / image 等） */
    role: string;
    /** 记忆正文（文本记忆；图像/音频记忆通常为空） */
    content: string;
    /** 内容向量余弦相似度 */
    similarity: number;
    /** 入库时间 Unix 秒级时间戳（旧数据可能为空） */
    timestamp?: number;
    /** 内容自动归入的分组集合名 */
    collection?: string;
    /** 写入来源标记（如 action） */
    source?: string;
}

/** 记忆检索方式：向量语义 / 关键词 / 正则 */
export type MemorySearchMode = 'vector' | 'keyword' | 'regex';

/** 跨集合记忆检索选项 */
export interface MemorySearchOptions {
    /** 检索方式（默认 vector） */
    mode?: MemorySearchMode;
    /** 必须全部包含的过滤词（如说话人昵称） */
    filters?: string[];
    /** 命中任意一个就排除的过滤词 */
    exclude?: string[];
}

/** 规整过滤词：去空白、小写、去重 */
function normalizeTerms(terms?: string[]): string[] {
    return [...new Set((terms || []).map(term => String(term || '').trim().toLowerCase()).filter(Boolean))];
}

/** 正文是否包含全部过滤词（空列表视为不限制） */
function containsAll(content: string, terms: string[]): boolean {
    if (terms.length === 0) return true;
    const lower = content.toLowerCase();
    return terms.every(term => lower.includes(term));
}

/** 正文是否命中任意排除词（空列表视为不排除） */
function containsAny(content: string, terms: string[]): boolean {
    if (terms.length === 0) return false;
    const lower = content.toLowerCase();
    return terms.some(term => lower.includes(term));
}

/**
 * 全局检索：按指定方式检索全部集合中的**文本类型**记忆，过滤后返回前 N 条。
 *
 * 集合清单属于实现细节，不暴露给模型：调用方只说「查什么、怎么查、过滤什么、要几条」。
 * 月华思维链里的记忆引用固定为文本类型；表情包等图像记忆走 queryEmotionSticker。
 *
 * @param query 正向检索词（向量模式的语义查询 / 关键词模式的空格分词 / 正则模式的表达式）
 * @param limit 返回条数上限（默认 8，最大 30）
 * @param options 检索方式与过滤词
 * @returns 命中条目（向量模式按相似度降序，关键词/正则模式按时间倒序）
 */
export function searchMemoryGlobal(
    query: string,
    limit = 8,
    options: MemorySearchOptions = {}
): MemorySearchHit[] {
    const text = String(query || '').trim();
    if (!text) return [];
    const mode: MemorySearchMode = options.mode || 'vector';
    const safeLimit = Math.min(Math.max(Math.trunc(limit) || 8, 1), 30);

    const filters = normalizeTerms(options.filters);
    const exclude = normalizeTerms(options.exclude);
    /** 关键词模式的词项（全部命中）；正则模式整串作为表达式 */
    const terms = mode === 'regex' ? [text] : text.split(/\s+/).filter(Boolean);
    if (terms.length === 0) return [];

    try {
        const [results, error] = mode === 'vector'
            ? memoryQuery(text, 'text', safeLimit)
            : memoryKeywordSearch('text', terms, mode === 'regex', safeLimit);
        if (error || !results) {
            if (error) console.error('[记忆检索] 查询失败:', error?.message ?? String(error));
            return [];
        }

        const hits: MemorySearchHit[] = [];
        for (const item of results) {
            const content = String(item.content || '').trim();
            // 纯媒体记忆没有正文，跳过（文本类型查询理论上不会返回）
            if (!content) continue;
            // 动作记忆是内部执行数据，不进入面向用户的聊天记忆检索
            if (item.source === ACTION_SOURCE) continue;
            if (!containsAll(content, filters)) continue;
            if (containsAny(content, exclude)) continue;
            hits.push({
                id: String(item.id || ''),
                type: String(item.type || 'text'),
                role: String(item.role || 'unknown'),
                content,
                similarity: Number(item.similarity) || 0,
                timestamp: item.timestamp,
                collection: item.collection,
                source: item.source,
            });
        }

        // 同一段原文可能同时存在于多个集合，按内容去重
        const seen = new Set<string>();
        const unique = hits.filter(hit => {
            if (seen.has(hit.content)) return false;
            seen.add(hit.content);
            return true;
        });
        const top = unique.slice(0, safeLimit);
        if (top.length > 0) {
            console.log(`[记忆检索] 方式=${mode}，检索「${text.slice(0, 30)}」，返回 ${top.length} 条`);
        }
        return top;
    }
    catch (error) {
        console.error('[记忆检索] 查询异常:', error);
        return [];
    }
}

/** 将动作组逐个写入动作记忆（源标记 action；相同/相近文本由记忆库去重），返回是否就绪 */
export function syncActionsToMemory(actionNames: string[]): boolean {
    if (!actionNames || actionNames.length === 0) return false;
    try {
        if (!ensureMemoryReady()) return false;
        // 逐个动作名入库：动作名与「动作」标签共同决定嵌入与归组，重复写入被去重机制忽略
        for (const name of actionNames) {
            const trimmed = (name || '').trim();
            if (!trimmed) continue;
            memoryAddWithTags('assistant', trimmed, [trimmed, '动作'], ACTION_SOURCE);
        }
        return true;
    }
    catch (error) {
        console.error('[动作库] 动作组入库失败:', error);
        return false;
    }
}

/** 将文本与动作记忆执行一次匹配，返回最高匹配度的动作名（无结果或失败时返回 null） */
export function queryBestAction(text: string): string | null {
    if (!text || !text.trim()) return null;
    try {
        // 全局文本检索：只在 source=action 的条目里取最佳匹配，避免把普通聊天文本当作动作名
        const [results, error] = memoryQuery(text.trim(), 'text', 30);
        if (error || !results || results.length === 0) return null;
        const matched = results.find(item => item.source === ACTION_SOURCE && String(item.content || '').trim());
        return matched ? String(matched.content).trim() : null;
    }
    catch (error) {
        console.error('[动作库] 匹配失败:', error);
        return null;
    }
}

/** 基于查询文本从图像类型记忆中检索表情包，返回图片 URL；无结果或失败时返回 null */
export async function queryEmotionSticker(query: string): Promise<string | null> {
    if (!query || !query.trim()) return null;
    try {
        if (!ensureMemoryReady()) return null;
        /** 查询图像类型记忆（跨全部集合） */
        const [results, error] = memoryQuery(query.trim(), 'image', 10);
        if (error || !results || results.length === 0) return null;
        /** 在查询结果中随机选择一个结果（图片记忆 url 字段） */
        const item = results[RandomFloor(0, results.length - 1)];
        if (item.url) return item.url;
        // 历史记忆兼容：旧数据仍为内联 base64 时先落盘为内容仓库实体，再按 URL 返回
        if (item.base64) return saveBase64Media(item.base64, 'image/jpeg', 'sticker') || null;
        return null;
    }
    catch (error) {
        console.error('[表情包] 检索失败:', error);
        return null;
    }
}

/** 从消息中提取全部文本内容（多模态消息剔除图片，仅保留文本项；无文本时返回空字符串） */
export function extractTextFromMessage(message: PostMessage): string {
    // 纯文本消息和工具响应消息直接返回
    if (typeof message.content === 'string') return message.content;
    // 多模态消息：提取所有文本内容并拼接，剔除图片等非文本项
    if (Array.isArray(message.content)) {
        return message.content.filter(item => item.type === 'text').map(item => item.text).join(' ');
    }
    return '';
}

/**
 * 从消息中提取用于长期记忆的文本
 *
 * 统一资源句柄（@I.007 等）是持久化标识：序号写库后永不复用，内容仓库回收后
 * 句柄依然可解析（降级为描述），因此**写入记忆前无需抹除或改写句柄**。
 * 历史 `图片.00x` 标签体系已废弃，不再做正向映射（旧记忆里的该形态若被模型照抄，
 * 会被当作无效句柄处理，这与"该写法已不存在"的事实一致）。
 *
 * @param message 待提取的消息
 * @returns 用于长期记忆的文本
 */
export function extractMemoryText(message: PostMessage): string {
    return extractTextFromMessage(message);
}

/** 按 ID 读取一条记忆（跨全部集合；不存在时返回 null） */
export function getMemoryEntry(id: string): MemorySearchHit | null {
    try {
        const [entry, error] = memoryGet(String(id || '').trim());
        if (error || !entry) return null;
        return {
            id: String(entry.id || ''),
            type: String(entry.type || 'text'),
            role: String(entry.role || 'unknown'),
            content: String(entry.content || ''),
            similarity: Number(entry.similarity) || 0,
            timestamp: entry.timestamp,
            collection: entry.collection,
            source: entry.source,
        };
    }
    catch (error) {
        console.error('[记忆] 按 ID 读取失败:', error);
        return null;
    }
}

/** 逐条列举记忆（跨全部集合；mediaType 为空表示全部类型，limit<=0 返回全部） */
export function listMemoryEntries(mediaType?: MemoryMediaType, limit = 0, offset = 0): MemorySearchHit[] {
    try {
        const [entries, error] = memoryList(mediaType || '', limit, offset);
        if (error || !entries) return [];
        return entries.map(entry => ({
            id: String(entry.id || ''),
            type: String(entry.type || 'text'),
            role: String(entry.role || 'unknown'),
            content: String(entry.content || ''),
            similarity: Number(entry.similarity) || 0,
            timestamp: entry.timestamp,
            collection: entry.collection,
            source: entry.source,
        }));
    }
    catch (error) {
        console.error('[记忆] 列举失败:', error);
        return [];
    }
}

/** 初始化记忆库（启动时一次性加载全部集合与默认分组） */
function initMemory(): void {
    if (GlobalConfig.memoryReady) return;
    const [ok, err] = memoryInit();
    if (err || !ok) console.error('记忆库初始化失败:', err ?? '返回失败');
    else GlobalConfig.memoryReady = true;
}

/** 确保记忆库已初始化，返回是否就绪 */
export function ensureMemoryReady(): boolean {
    if (!GlobalConfig.memoryReady) initMemory();
    return GlobalConfig.memoryReady;
}
