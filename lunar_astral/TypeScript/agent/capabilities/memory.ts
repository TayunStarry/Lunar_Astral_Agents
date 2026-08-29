import { GlobalConfig, PostMessage, SCHEDULE_TRIGGER_PREFIX, RandomFloor } from '../../index';

/** 表情包记忆库集合名（image 类型集合，由 memory.store 前端管理） */
const STICKER_COLLECTION = 'stickers';
/** 表情包集合是否已确认就绪（避免重复初始化） */
let stickerCollectionReady = false;

/** 基于查询文本从表情包记忆库检索表情包图片，返回 base64；无结果或失败时返回 null */
export async function queryEmotionSticker(query: string): Promise<string | null> {
    if (!query || !query.trim()) return null;
    try {
        // 首次使用时确保表情包集合存在（幂等：已存在则直接打开，不清空数据）
        if (!stickerCollectionReady) {
            const [ready] = memoryInit(STICKER_COLLECTION, 'image');
            if (!ready) return null;
            stickerCollectionReady = true;
        }
        /** 查询表情包记忆库 */
        const [results, error] = memoryQuery(STICKER_COLLECTION, query.trim(), 3);
        // 查询失败或无结果时返回 null
        if (error || !results || results.length === 0) return null;
        /** 在查询结果中随机选择一个结果 */
        const image = (results[RandomFloor(0, results.length - 1)] as { image?: string }).image;
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
        return message.content
            .filter(item => item.type === 'text')
            .map(item => item.text)
            .join(' ');
    }
    return '';
}

/** 初始化记忆库 */
function initMemory(): void {
    if (GlobalConfig.memoryReady) return;
    const [_, err] = memoryInit('lunar_messages', 'text');
    if (err) console.error('记忆库初始化失败:', err);
    else GlobalConfig.memoryReady = true;
}

/** 确保记忆库已初始化，返回是否就绪 */
export function ensureMemoryReady(): boolean {
    if (!GlobalConfig.memoryReady) initMemory();
    return GlobalConfig.memoryReady;
}

/** 记忆未读消息到记忆库 */
export function memorizeUnreadRecords(): void {
    // 缓冲池为空时跳过
    if (GlobalConfig.unreadRecords.length === 0) return;
    // 记忆库未就绪时保留缓冲消息，等待下次触发
    if (!ensureMemoryReady()) {
        console.warn('[记忆] 记忆库未就绪，保留缓冲消息待下次触发');
        return;
    }
    /** 成功写入的消息数量 */
    let written = 0;
    // 遍历未读消息缓冲池
    for (const message of GlobalConfig.unreadRecords) {
        // 过滤掉工具调用消息
        if (message.role === 'tool') continue;
        /** 提取文本内容并过滤空字符串 */
        const content = extractTextFromMessage(message).trim();
        // 过滤掉空字符串、长度小于等于5的消息
        if (!content || content.length <= 5) continue;
        // 过滤掉计划表触发的自动消息（统一前缀），避免写入长期记忆
        if (content.includes(SCHEDULE_TRIGGER_PREFIX)) continue;
        /** 写入记忆库 */
        const [, error] = memoryAdd('lunar_messages', message.role, content);
        // 记录写入失败的错误信息
        if (error) console.error('[记忆] 写入记忆库失败:', error);
        // 记录成功写入的消息数量
        else written++;
    }
    console.log(`[记忆] 已写入 ${written} 条消息到记忆库`);
    // 清空消息缓冲池
    GlobalConfig.unreadRecords = [];
}
