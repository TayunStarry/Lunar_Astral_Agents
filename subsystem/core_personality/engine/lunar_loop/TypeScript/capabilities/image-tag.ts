import { admitImageDataURI, deleteResource, getResourceEntry } from './resource';
import { formatHandle, parseHandle } from './handle';
import { GlobalConfig } from '../config/global';
import type { PostMessage } from '../config/model';

// ===== 图片标签机制（对话者侧唯一实现） =====
//
// 机制只有两条规矩（二段走）：
//   1. 入队赋签：多模态消息推入未读数组时，为其中每个图片项入库（内容哈希去重），
//      并把图片项的来源**改写为视觉读取 URL** —— URL 自带 tag 参数（如 ?tag=@I.003），
//      端点返回「约束尺寸 + 右上角叠印句柄」的派生副本（原图不动，参考图始终干净）。
//      标签与图片文件的绑定记录在资源目录数据库。
//   2. 窗口降级：对话者上下文只保留最新 20 张真实图片；更早的图片项
//      置换为一条文本 `[@I.00x]（内容描述）`（句柄从图片项 URL 的 tag 参数解析）。
//
// 句柄随图走（烙在副本右上角 + 编码在 URL 里），不再使用「图片项后紧跟旁注文本」的
// 旧排布 —— 连续多图场景下相邻文本可能被模型对应到错误的图片上。

/** 多模态窗口上限：对话者上下文中允许保留的真实图片数量 */
export const IMAGE_WINDOW_SIZE = 20;

/** 视觉读取端点路径（与 server/variable.go 的路由声明一致） */
const VISION_READ_PATH = '/file/miniature/';

/** 合法句柄形态（tag 参数校验，与 Go 侧 visionTagPattern 一致） */
const TAG_PATTERN = /^@[A-Z]\.\d{3,}$/;

/** 按来源推断图片扩展名（data URI 的 MIME / URL 路径后缀；无法推断时回落 .png） */
export function guessImageExt(source: string): string {
    const lower = (source || '').toLowerCase();
    const dataMatch = /^data:image\/(\w+)[;,]/.exec(lower);
    if (dataMatch) return `.${dataMatch[1] === 'jpeg' ? 'jpg' : dataMatch[1]}`;
    const path = lower.split('?')[0].split('#')[0];
    const ext = path.slice(path.lastIndexOf('.'));
    return /^\.[a-z0-9]{2,5}$/.test(ext) ? ext : '.png';
}

/**
 * 从视觉读取 URL 中解析句柄（tag 查询参数）
 *
 * @param url 图片项来源（视觉读取 URL / data URI / 原始来源）
 * @returns 规范句柄（@I.003）；无法解析返回空串
 */
export function handleFromVisionURL(url: unknown): string {
    if (typeof url !== 'string') return '';
    const match = /[?&]tag=([^&]+)/.exec(url);
    if (!match) return '';
    let tag = match[1];
    try {
        tag = decodeURIComponent(tag);
    }
    catch {
        // 保留原样继续校验
    }
    return TAG_PATTERN.test(tag) ? tag : '';
}

/**
 * 入队赋签：为消息中的每个图片项入库，并把来源改写为视觉读取形态
 *
 * 在消息推入未读数组时调用（loop.ts 的 writeMessage）。url 形态（默认）下思维链中
 * 流转的是视觉读取端点 URL（服务端缩放 + 叠印句柄 + 盘上缓存）；base64 形态下经
 * 同一变换管线直出 data URI（供无法回取本机 URL 的云端多模态服务）。
 * 入库按内容哈希去重：同一张图反复发送复用同一句柄。
 *
 * 入库失败时该图片项被**替换为文本占位**，绝不原样放行：原图一旦进入多模态窗口，
 * 视觉 token 会撑爆 llama-server 的非因果注意力批（进程断言退出），
 * 而且那正是 docs/adr/0008 明令禁止的「原图流转」。
 *
 * @param message 目标消息（内容为数组时原位修改；纯文本消息跳过）
 */
export function attachImageTags(message: PostMessage): void {
    if (!message || typeof message.content === 'string' || !Array.isArray(message.content)) return;
    const content = message.content as Array<{ type?: string; image_url?: { url?: string }; text?: string }>;
    for (const item of content) {
        if (item?.type !== 'image_url') continue;
        const source = item.image_url?.url || '';
        if (!source) continue;
        // 视频块（伪装成 image_url 的 data:video URI 或视频扩展名）不走图片标签：
        // 它们由 LiteImageFile 以「句柄 + 理解文本」原位置换，不进入多模态窗口
        const lower = source.toLowerCase();
        if (lower.startsWith('data:video/') || GlobalConfig.videoFormatsExtensions.some(ext => lower.endsWith(ext))) continue;
        // 已经是视觉读取端点形态：不要再入库一次（避免把副本当原图重复登记）
        if (lower.includes(VISION_READ_PATH)) continue;
        // 入库（内容哈希去重在 Go 侧完成，JS 侧不触碰图片二进制）。
        // 建议名按来源推断真实扩展名：blob 落盘沿用 <内容哈希>.<扩展> 命名，
        // 硬编码 image.png 会让前端已按 <哈希>.jpg 落盘的同内容图片被再写一份 .png
        const admitted = admitImageDataURI(source, `image${guessImageExt(source)}`);
        if (!admitted?.handle || !admitted.blob_path) {
            // 阻断降级：宁可少一张图，也不让原图进入思维链
            console.error('[图片标签] 入库失败，该图片已从思维链剔除（禁止原图流转）:', source.substring(0, 60));
            item.type = 'text';
            item.text = '[一张图片入库失败，已忽略]';
            delete item.image_url;
            continue;
        }
        // 来源改写为视觉读取形态：约束尺寸 + 句柄叠印，模型看到的每张图都自带身份
        item.image_url.url = visionReference(admitted.handle, admitted.blob_path);
    }
}

/**
 * 生成图片的思维链消费引用（两种形态，均由视觉读取管线产出）
 *
 * - url（默认）：`/file/miniature/` 端点 URL，服务端按需缩放、叠印句柄并缓存；
 * - base64：`visionRead` 走同一套 VisionCopy 管线直出 data URI，供无法回取本机 URL 的云端模型。
 *
 * 两种形态都是 ≤ vision_max_side 的副本，模型永远拿不到原图，也没有自己缩放的能力。
 *
 * @param handle 规范句柄（@I.00x）
 * @param blobPath 源图相对 LocalDir 路径（内容仓库位置）
 * @returns 视觉读取端点 URL / 视觉副本 data URI；无法构造时返回空串
 */
export function visionReference(handle: string, blobPath: string): string {
    try {
        if (GlobalConfig.ImageInputMode === 'base64') {
            // 云端模型：同一变换管线直出 data URI（缩放缓存命中时秒回）
            const [dataURI, error] = visionRead(blobPath, handle);
            if (!error && dataURI) return dataURI;
            console.error('[图片标签] 视觉读取失败，退化为端点 URL:', error?.message ?? String(error));
        }
        return `${url()[0]}${VISION_READ_PATH}${blobPath}?tag=${encodeURIComponent(handle)}`;
    }
    catch (error) {
        console.error('[图片标签] 生成视觉引用失败:', error);
        return '';
    }
}

/**
 * 规整图片内容描述，保证其可安全嵌入 `[@I.00x]（描述）` 占位而不破坏句柄形态：
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
    // 去首尾标点，避免出现 [@I.003]（，一只白猫）
    text = text.replace(/^[，。、；：,.!?！？~～…\-\s]+/, '').replace(/[，。、；：,.!?！？~～…\-\s]+$/, '').trim();
    if (!text) return '';
    if (text.length > maxChars) text = text.slice(0, maxChars).trim() + '…';
    return text;
}

/**
 * 组装降级占位文本：`[@I.003]（描述）`
 *
 * 无描述时保留纯句柄 —— 句柄本身就是有效引用，写「无描述」只会白占上下文。
 */
export function composeImagePlaceholder(tag: string, desc: string): string {
    const clean = sanitizeImageDesc(desc, 40);
    return clean ? `[${tag}]（${clean}）` : `[${tag}]`;
}

// ===== 句柄 → 来源解析（工具侧统一入口） =====

/**
 * 把图片句柄解析为可取用的来源（资源目录为唯一权威源）
 *
 * @param raw 句柄文本（可带方括号）
 * @returns 来源信息（规范句柄 + 相对 LocalDir 的实体路径 + 描述）；无法解析返回 null
 */
export function resolveImageRef(raw: string): { handle: string; source: string; desc: string } | null {
    const trimmed = String(raw || '').trim();
    if (!trimmed) return null;
    const handle = parseHandle(trimmed);
    if (!handle || handle.kind !== 'I') return null;
    const normalized = formatHandle(handle);
    const entry = getResourceEntry(normalized);
    if (!entry) return null;
    // 本体已丢失/损坏：移除失效句柄并返回 null（句柄已从目录清除，等同不存在）
    if (entry.state === 'lost') {
        deleteResource(normalized);
        return null;
    }
    // blob 在位时给出相对 LocalDir 的路径，交给管线按路径解析；已回收时来源为空（仅描述可用）
    const source = entry.state === 'active' && entry.blob_path ? entry.blob_path : '';
    return { handle: normalized, source, desc: entry.summary || '' };
}
