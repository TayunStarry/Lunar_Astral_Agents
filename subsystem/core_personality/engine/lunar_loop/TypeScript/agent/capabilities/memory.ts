import { GlobalConfig } from '../../config/global';
import { PostMessage } from '../../config/model';
import { RandomFloor } from '../../tool/math';

/** 表情包记忆库集合名（image 类型集合，由 memory.store 前端管理） */
const STICKER_COLLECTION = 'stickers';
/** 表情包集合是否已确认就绪（避免重复初始化） */
let stickerCollectionReady = false;

/** 动作组记忆集合名（text 类型集合，动作名逐条入库） */
const ACTION_COLLECTION = 'actions';
/** 动作集合是否已确认就绪（避免重复初始化） */
let actionCollectionReady = false;

/** 长期对话记忆集合名 */
export const LUNAR_MESSAGES_COLLECTION = 'lunar_messages';

/** 跨集合检索时跳过的非对话集合（图片库 / 动作库） */
const NON_CONVERSATION_COLLECTIONS = new Set([STICKER_COLLECTION, ACTION_COLLECTION]);

/** 跨集合记忆检索命中 */
export interface MemorySearchHit {
    /** 记忆文档 ID */
    id: string;
    /** 命中的集合名 */
    collection: string;
    /** 记忆角色（user / assistant / image 等） */
    role: string;
    /** 记忆正文（图片记忆为空，已跳过） */
    content: string;
    /** 内容向量余弦相似度 */
    similarity: number;
    /** 入库时间 Unix 秒级时间戳（旧数据可能为空） */
    timestamp?: number;
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

/** 列出可检索的对话类集合（过滤图片库与动作库） */
export function listMemoryCollections(): string[] {
    try {
        // 确保记忆库实例与默认集合已初始化（幂等）
        if (!ensureMemoryReady()) return [];
        const [names, error] = memoryListCollections();
        if (error || !names) {
            console.error('[记忆检索] 列出集合失败:', error?.message ?? String(error ?? '返回为空'));
            return [];
        }
        return names.filter(name =>
            !!name
            && !NON_CONVERSATION_COLLECTIONS.has(name)
        );
    }
    catch (error) {
        console.error('[记忆检索] 列出集合抛异常:', error);
        return [];
    }
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
 * 跨集合全局检索：按指定方式（向量 / 关键词 / 正则）检索每个对话类集合，
 * 合并、过滤、去重后取前 N 条。
 *
 * 集合清单属于实现细节，不暴露给模型：调用方只说「查什么」，本函数自行枚举
 * 全部对话类集合（跳过表情包与动作库）。
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
    const collections = listMemoryCollections();
    if (collections.length === 0) return [];

    const filters = normalizeTerms(options.filters);
    const exclude = normalizeTerms(options.exclude);
    /** 每个集合的检索条数：至少覆盖请求条数，避免合并后不足 */
    const perCollection = Math.min(Math.max(safeLimit, 4), 30);
    /** 关键词模式的词项（全部命中）；正则模式整串作为表达式 */
    const terms = mode === 'regex' ? [text] : text.split(/\s+/).filter(Boolean);
    if (terms.length === 0) return [];

    const hits: MemorySearchHit[] = [];
    for (const collection of collections) {
        try {
            const [results, error] = mode === 'vector'
                ? memoryQuery(collection, text, perCollection)
                : memoryKeywordSearch(collection, terms, mode === 'regex', perCollection);
            if (error || !results) continue;
            for (const item of results as Array<{ id?: string; role?: string; content?: string; similarity?: number; timestamp?: number }>) {
                const content = String(item.content || '').trim();
                // 图片记忆没有正文，跳过（跨集合检索只取可读文本）
                if (!content) continue;
                if (!containsAll(content, filters)) continue;
                if (containsAny(content, exclude)) continue;
                hits.push({
                    id: String(item.id || ''),
                    collection,
                    role: String(item.role || 'unknown'),
                    content,
                    similarity: Number(item.similarity) || 0,
                    timestamp: item.timestamp,
                });
            }
        }
        catch (error) {
            console.error(`[记忆检索] 集合 ${collection} 查询异常:`, error);
        }
    }

    // 向量模式按相似度降序；关键词/正则模式按入库时间倒序
    if (mode === 'vector') hits.sort((a, b) => b.similarity - a.similarity);
    else hits.sort((a, b) => (b.timestamp || 0) - (a.timestamp || 0));

    // 同一段原文可能同时出现在多个集合，按内容去重
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

/** 将动作组逐个写入动作记忆库（幂等：相同/相近文本内容被记忆库去重机制自动忽略），返回是否就绪 */
export function syncActionsToMemory(actionNames: string[]): boolean {
    if (!actionNames || actionNames.length === 0) return false;
    try {
        // 首次使用时确保动作集合存在（幂等：已存在则直接打开，不清空数据）
        if (!actionCollectionReady) {
            const [ready] = memoryInit(ACTION_COLLECTION);
            if (!ready) return false;
            actionCollectionReady = true;
        }
        // 逐个动作名入库：以动作名作为显式标签（跳过 LLM 标签生成），重复写入被去重机制忽略
        for (const name of actionNames) {
            const trimmed = (name || '').trim();
            if (!trimmed) continue;
            memoryAddWithTags(ACTION_COLLECTION, 'assistant', trimmed, [trimmed]);
        }
        return true;
    }
    catch (error) {
        console.error('[动作库] 动作组入库失败:', error);
        return false;
    }
}

/** 将文本与动作记忆库执行一次匹配，返回最高匹配度的动作名（无结果或失败时返回 null） */
export function queryBestAction(text: string): string | null {
    if (!text || !text.trim()) return null;
    try {
        /** 查询动作记忆库，取相似度最高的首条结果 */
        const [results, error] = memoryQuery(ACTION_COLLECTION, text.trim(), 1);
        // 查询失败或无结果时返回 null
        if (error || !results || results.length === 0) return null;
        return (results[0] as { content?: string }).content || null;
    }
    catch (error) {
        console.error('[动作库] 匹配失败:', error);
        return null;
    }
}

/** 基于查询文本从表情包记忆库检索表情包图片，返回 base64；无结果或失败时返回 null */
export async function queryEmotionSticker(query: string): Promise<string | null> {
    if (!query || !query.trim()) return null;
    try {
        // 首次使用时确保表情包集合存在（幂等：已存在则直接打开，不清空数据）
        if (!stickerCollectionReady) {
            const [ready] = memoryInit(STICKER_COLLECTION);
            if (!ready) return null;
            stickerCollectionReady = true;
        }
        /** 查询表情包记忆库 */
        const [results, error] = memoryQuery(STICKER_COLLECTION, query.trim(), 3);
        // 查询失败或无结果时返回 null
        if (error || !results || results.length === 0) return null;
        /** 在查询结果中随机选择一个结果（图片记忆 base64 字段） */
        const image = (results[RandomFloor(0, results.length - 1)] as { base64?: string }).base64;
        // 返回随机选择的图片或 null
        return image || null;
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

/** 初始化记忆库 */
function initMemory(): void {
    if (GlobalConfig.memoryReady) return;
    const [_, err] = memoryInit('lunar_messages');
    if (err) console.error('记忆库初始化失败:', err);
    else GlobalConfig.memoryReady = true;
}

/** 确保记忆库已初始化，返回是否就绪 */
export function ensureMemoryReady(): boolean {
    if (!GlobalConfig.memoryReady) initMemory();
    return GlobalConfig.memoryReady;
}
