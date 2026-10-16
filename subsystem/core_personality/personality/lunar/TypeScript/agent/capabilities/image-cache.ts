import type { ImageCacheEntry } from '../../config/image';

// ===== 共享图片缓存能力封装 =====
// 缓存权威存储在 Go 侧（imagecache 包），QQ 合并转发与对话者上下文共用同一标签命名空间：
// 上下文中的每张静态图片在入口处（LiteImageFile）注册并打 [图片.00x] 标签旁注，
// 合并转发图片在 Go 侧直接注册并以占位标签内嵌文本；模型经 describe_images 按需取回内容描述

/** 缓存容量（与 Go 侧 imagecache.ImageCacheCapacity 保持一致，仅用于提示文案） */
export const IMAGE_CACHE_CAPACITY = 80;

/** 多模态图片窗口上限（与 DialogueRole.IMAGE_WINDOW 保持一致，仅用于提示文案） */
export const IMAGE_WINDOW_SIZE = 20;

/** 占位标签形态的正则（匹配裸标签与 [图片.00x] 两种写法，允许四位数以上序号） */
const TAG_ITEM_PATTERN = /^\[?(图片\.\d+)\]?$/;

/** 规整模型传入的图片标签：兼容 图片.003 / [图片.003] / 003 / 3 形态，无效输入返回空字符串 */
export function normalizeImageTag(raw: string): string {
    if (!raw) return '';
    const trimmed = String(raw).trim();
    const tagged = trimmed.match(/图片\.(\d+)/);
    // 序号后若仍跟字母/数字（如提示词里的元语言写法「图片.00x」），视为无效标签
    if (tagged && !/[\p{L}\p{N}]/u.test(trimmed.slice((tagged.index || 0) + tagged[0].length))) {
        return `图片.${tagged[1].padStart(3, '0')}`;
    }
    if (/^\d+$/.test(trimmed)) return `图片.${trimmed.padStart(3, '0')}`;
    return '';
}

/** 判断文本项是否为纯标签项（入注旁注或降级占位，两种形态同判） */
export function isTagItem(text: string): boolean {
    return TAG_ITEM_PATTERN.test((text || '').trim());
}

/** 判断同时含描述文本的标签项：[图片.003]（一只白猫） */
export function isTaggedPlaceholder(text: string): boolean {
    return /^\[?图片\.\d+\]?（/.test((text || '').trim());
}

/**
 * 规整图片内容描述，保证其可安全嵌入 [图片.00x]（描述）占位而不破坏标签形态：
 * 折叠空白、去首尾标点、剔除方括号/全角括号/换行，最后按字数硬截断。
 *
 * @param raw 视觉模型返回的原始描述
 * @param maxChars 最大保留字数（超出按字数截断并省略号收尾）
 * @returns 规整后的描述（全部内容被剔除时返回空字符串）
 */
export function sanitizeImageDesc(raw: unknown, maxChars: number): string {
    if (typeof raw !== 'string') return '';
    let text = raw.replace(/\s+/g, ' ').trim();
    if (!text) return '';
    // 模型偶发的整体包裹（如「「一只白猫」」）：先剥一层首尾引号类字符
    text = text.replace(/^[「『"'“”‘’]+/, '').replace(/[」』"'“”‘’]+$/, '').trim();
    // 结构保留字符：方括号与全角括号一律剔除，避免破坏 [标签]（描述）形态
    text = text.replace(/[[\]【】〔〕（）()]/g, '').trim();
    // 去首尾标点，避免出现 [图片.003]（，一只白猫）
    text = text.replace(/^[，。、；：,.!?！？~～…\-\s]+/, '').replace(/[，。、；：,.!?！？~～…\-\s]+$/, '').trim();
    if (!text) return '';
    if (text.length > maxChars) text = text.slice(0, maxChars).trim() + '…';
    return text;
}

/** 组装降级占位文本：[图片.003]（描述）；无描述时保留纯标签（不写「无描述」以免占上下文） */
export function composeImagePlaceholder(tag: string, desc: string): string {
    const clean = sanitizeImageDesc(desc, 40);
    return clean ? `[${tag}]（${clean}）` : `[${tag}]`;
}

/** 图片来源类型标签（提示文案用，避免把 data URI 正文塞进上下文） */
export function sourceKind(source: string): string {
    if (/^data:image\//i.test(source)) return '内联图片';
    if (/^https?:\/\//i.test(source)) return '网络图片';
    return '本地图片';
}

/** 注册图片来源并分配标签（pinned: 多模态窗口内图片钉住防淘汰），失败时返回空字符串 */
export function putCacheEntry(source: string, pinned: boolean): string {
    try {
        const [tag, error] = imageCachePut(source, pinned);
        return (error || !tag) ? '' : tag;
    }
    catch (error) {
        console.error('[图片缓存] 注册失败:', error);
        return '';
    }
}

/** 按标签查询缓存条目，不存在或失败时返回 null */
export function getCacheEntry(tag: string): ImageCacheEntry | null {
    try {
        const [entry, error] = imageCacheGet(tag);
        return (error || !entry) ? null : entry;
    }
    catch (error) {
        console.error('[图片缓存] 查询失败:', error);
        return null;
    }
}

/** 按注册序号倒序返回最近 topK 个条目，失败时返回空数组 */
export function listCacheEntries(topK: number): ImageCacheEntry[] {
    try {
        const [entries, error] = imageCacheList(topK);
        return (error || !entries) ? [] : entries;
    }
    catch (error) {
        console.error('[图片缓存] 清单查询失败:', error);
        return [];
    }
}

/** 写入条目内容描述（失败静默，描述仅用于展示） */
export function setCacheDesc(tag: string, desc: string): void {
    try {
        imageCacheSetDesc(tag, desc);
    }
    catch (error) {
        console.error('[图片缓存] 描述写入失败:', error);
    }
}

/** 解除条目钉住（图片降级为占位标签后调用） */
export function unpinCacheTag(tag: string): void {
    try {
        imageCacheUnpin(tag);
    }
    catch (error) {
        console.error('[图片缓存] 解钉失败:', error);
    }
}

// ===== 本轮入注标签的暂存区 =====
// 降级（摘图留签）发生在同一轮推理内、消息尚未写入上下文时：优先级为
//   ①本轮暂存标签（图片刚入注，标签必然在同一消息内）
//   ②消息内已写入的裸标签旁注（历史消息降级路径）
//   ③现注册兜底（工具结果内嵌图片等无旁注场景）
// 每条消息降级完成后清空暂存区，避免跨消息错配与跨轮泄漏。

/** 本轮入注标签：消息对象 → （图片来源 → 标签） */
const pendingTags = new Map<object, Map<string, string>>();

/** 暂存本轮入注标签（registerAndPushImage 调用，供同轮降级取用） */
export function stashPendingTag(message: object, source: string, tag: string): void {
    if (!message || !source || !tag) return;
    let bucket = pendingTags.get(message);
    if (!bucket) {
        bucket = new Map<string, string>();
        pendingTags.set(message, bucket);
    }
    bucket.set(source, tag);
}

/** 按来源取出暂存的入注标签（取走即删除，保证一图一签） */
export function takePendingTag(message: object, source: string): string {
    const bucket = pendingTags.get(message);
    if (!bucket) return '';
    const tag = bucket.get(source) || '';
    if (tag) bucket.delete(source);
    if (bucket.size === 0) pendingTags.delete(message);
    return tag;
}

/** 清理某条消息的暂存标签（降级完成或消息被裁剪时调用） */
export function clearPendingTags(message: object): void {
    pendingTags.delete(message);
}

/**
 * 降级一条消息中的全部图片为占位标签，返回重建后的内容项数组（纯函数，便于单测验证无残留）。
 *
 * 图片与标签旁注按出现顺序一一对应，且两种排布都要兼容：
 *   - 标签在图片之后（媒体入注 registerAndPushImage 与工具结果图片的实际排布）
 *   - 标签在图片之前（历史消息的既有形态）
 * 配对时只消费「该图片之前最近的一个未被占用的标签项」或「其后最近的一个未被占用的标签项」；
 * 用户正文里的标签字面量只要不与图片相邻就直接保留，不被吞掉。消息内多余的标签项一律原样保留。
 *
 * @param content 原内容项数组
 * @param tagFor 取标签回调（返回空字符串时丢弃该图片项，不留悬空句柄）
 * @param descFor 取描述回调（返回空字符串时占位退化为纯标签）
 */
export function demoteContentImages(
    content: any[],
    tagFor: (source: string) => string,
    descFor: (tag: string, source: string) => string
): any[] {
    const items = content || [];
    /** 各内容项降级后的替代文本（null 表示按原样保留；空字符串表示该项被丢弃） */
    const replacements: (string | null)[] = items.map(() => null);
    /** 标签项下标（旁注兼作该图片的标签来源） */
    const tagIndexes: number[] = [];
    /** 图片项下标 */
    const imageIndexes: number[] = [];
    items.forEach((item, index) => {
        const itemType = item ? (item as { type?: string }).type : undefined;
        if (itemType === 'text' && isTagItem(String((item as { text?: unknown }).text ?? ''))) tagIndexes.push(index);
        else if (itemType === 'image_url') imageIndexes.push(index);
    });
    const usedTags = new Set<number>();

    /** 取该图片配套的标签旁注下标：优先其前最近未占用者，其次其后最近未占用者 */
    const takeAnnotation = (imageIndex: number): number => {
        for (let k = tagIndexes.length - 1; k >= 0; k--) {
            if (!usedTags.has(tagIndexes[k]) && tagIndexes[k] < imageIndex) {
                usedTags.add(tagIndexes[k]);
                return tagIndexes[k];
            }
        }
        for (let k = 0; k < tagIndexes.length; k++) {
            if (!usedTags.has(tagIndexes[k]) && tagIndexes[k] > imageIndex) {
                usedTags.add(tagIndexes[k]);
                return tagIndexes[k];
            }
        }
        return -1;
    };

    for (const imageIndex of imageIndexes) {
        const item = items[imageIndex];
        const source = (item as { image_url?: { url?: string } }).image_url?.url || '';
        /** 该图片的最终标签 */
        const tag = tagFor(source);
        // 标签来源无效（注册失败）：丢弃图片项，避免留下无法寻址的裸图
        if (!tag) {
            replacements[imageIndex] = '';
            continue;
        }
        const annotationIndex = takeAnnotation(imageIndex);
        // 被消费的旁注由其替代文本取代（占位落在旁注原位），图片项本身丢弃不再出现
        if (annotationIndex >= 0) {
            replacements[annotationIndex] = composeImagePlaceholder(tag, descFor(tag, source));
            replacements[imageIndex] = '';
        }
        else replacements[imageIndex] = composeImagePlaceholder(tag, descFor(tag, source));
    }

    const rebuilt: any[] = [];
    items.forEach((item, index) => {
        const replacement = replacements[index];
        if (replacement === '') return;
        if (typeof replacement === 'string') rebuilt.push({ type: 'text', text: replacement });
        else rebuilt.push(item);
    });
    return rebuilt;
}
