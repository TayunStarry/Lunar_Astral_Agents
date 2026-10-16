import { ToolCall } from '../config/tool';
import { GlobalConfig } from '../config/global';
import type { ModelBuilder } from '../agent/base/builder';
import {
    IMAGE_CACHE_CAPACITY,
    IMAGE_WINDOW_SIZE,
    getCacheEntry,
    listCacheEntries,
    normalizeImageTag,
    sanitizeImageDesc,
    setCacheDesc
} from '../agent/capabilities/image-cache';

// ==== 图片标签工具（对话者可用） ====
// 对话者经 list_images 查看当前存在的图片标签，经 describe_images 按标签获取图片内容描述
// （多模态模型解读，不注入图片本体）：
//   适用于已降级的占位标签图片（[图片.00x]（描述））、QQ 合并转发内的图片、
//   以及工具（截图等）返回的图片。
// 描述与共享缓存的 desc 字段共用（命中零视觉调用，未命中生成一次并回写）；
// 需要把图片交给绘图者参照创作时无需调用——直接在 dispatch_painter 的 reference_tag 里写标签。

/** 描述角色（视觉模型），由 roles.ts 注入（避免 tool → roles 静态循环引用） */
let describeRole: ModelBuilder | null = null;

/** 注入描述角色（roles.ts 模块初始化时调用一次） */
export function registerImageDescribeRole(role: ModelBuilder): void {
    describeRole = role;
}

/** 图片标签工具定义 */
export const imageRecallTools: ToolCall[] = [
    {
        type: "function",
        function: {
            name: "list_images",
            description: "列出当前缓存中可用的图片标签清单（标签 + 内容描述 + 是否仍在多模态窗口内），按注册时间倒序，只返回文字不含图片。当你不确定某张图片的标签是否还有效、或用户提到一张你已看不到的图片时，先用本工具查看清单，再用 describe_images 按标签取内容描述。",
            parameters: {
                type: "object",
                properties: {
                    limit: {
                        type: "number",
                        description: "返回条目数量上限，默认 30"
                    }
                },
                required: []
            }
        }
    },
    {
        type: "function",
        function: {
            name: "describe_images",
            description: "按图片标签获取图片的内容描述（视觉模型解读，不注入图片本体，上下文成本极低）。占位文本 [图片.00x]（…）括号内已是内容描述，无需再调用；需要了解描述之外的细节（如「图上写了什么字」、画面里第二个人的穿着）时可带 question 定向提问。标签可从消息中的旁注占位、list_images 清单或合并转发文本中获得；交给绘图者参照创作时无需调用，直接在 dispatch_painter 的 reference_tag 里写标签。",
            parameters: {
                type: "object",
                properties: {
                    tags: {
                        type: "array",
                        items: { type: "string" },
                        description: "图片标签数组，如 [\"图片.003\", \"图片.007\"]（写作 [图片.003] 形式也可以）"
                    },
                    question: {
                        type: "string",
                        description: "可选：用户当前关心的具体问题（如'图上写了什么字'、'第二个人穿什么'），描述将围绕该问题定向生成"
                    }
                },
                required: ["tags"]
            }
        }
    }
];

/** 生成条目描述：缓存命中直接返回；未命中时调视觉模型生成并回写缓存（定向问题不污染共用描述） */
function describeEntry(tag: string, source: string, question: string): string {
    const entry = getCacheEntry(tag);
    if (entry?.desc && !question) return sanitizeImageDesc(entry.desc, 160);
    if (!describeRole || !source) return sanitizeImageDesc(entry?.desc || '', 160);
    try {
        /** 描述指令：通用一句话描述，或围绕用户问题定向描述 */
        const instruction = question
            ? `用户想知道：${question}。请观察这张图片，用不超过100字的中文描述其中与该问题相关的关键信息，直接输出描述本身。`
            : '用一句不超过20字的中文描述这张图片的主体与场景，只输出描述本身。';
        describeRole.coverContext({
            role: 'user',
            content: [
                { type: 'text', text: instruction },
                { type: 'image_url', image_url: { url: source } }
            ]
        });
        const response = describeRole.run([], []);
        const desc = sanitizeImageDesc(response.body?.choices?.[0]?.message?.content || '', 160);
        // 定向回答只回本次调用（不写共用 desc），避免窄问题答案覆盖通用描述、
        // 导致后续降级占位与 list_images 清单显示的内容漂移
        if (desc && !question) setCacheDesc(tag, desc);
        return desc || sanitizeImageDesc(entry?.desc || '', 160);
    }
    catch (error) {
        console.error(`[图片描述] ${tag} 描述生成失败:`, error);
        return sanitizeImageDesc(entry?.desc || '', 160);
    }
}

/** 处理标签清单工具：返回可用标签、内容描述与窗内标记 */
async function handleListImages(args?: Record<string, any> | string): Promise<string[]> {
    const parsed = typeof args === 'string' ? JSON.parse(args) : (args || {});
    /** 条目上限：默认 30，硬上限为缓存容量 */
    const limit = Math.min(Math.max(Number(parsed.limit) || 30, 1), IMAGE_CACHE_CAPACITY);
    const entries = listCacheEntries(limit);
    if (entries.length === 0) return ['当前图片缓存为空：没有任何可用的图片标签', ''];
    const lines = entries.map(entry =>
        `[${entry.tag}] ${entry.desc ? sanitizeImageDesc(entry.desc, 40) : '（暂无描述，可用 describe_images 获取）'}${entry.pinned ? ' [窗内]' : ''}`
    );
    const text = `当前可用的图片标签（共 ${entries.length} 条，按注册时间倒序，越靠前越新；[窗内] 表示图片仍在你的多模态窗口中，能直接看到）：`
        + `\n${lines.join('\n')}`
        + `\n标签只在本次运行内有效（重启后编号会重新分配），且缓存仅保留最近的图片；标签取不到时说明该图已过期，可参考上表中仍然存在的其它标签。`;
    return [text, ''];
}

/** 处理图片描述工具：标签数组规整去重 → 逐个取缓存条目 → 返回描述文本（纯文本结果，不注入图片） */
async function handleDescribeImages(args?: Record<string, any> | string): Promise<string[]> {
    const parsed = typeof args === 'string' ? JSON.parse(args) : (args || {});
    /** 标签入参：兼容数组与单个标签两种形态 */
    const rawTags: unknown[] = Array.isArray(parsed.tags) ? parsed.tags : (parsed.tag ? [parsed.tag] : []);
    const tags = [...new Set(rawTags.map(item => normalizeImageTag(String(item || ''))).filter(Boolean))];
    if (tags.length === 0) return ['取图失败：请提供图片标签数组，如 ["图片.003"]；标签形如 图片.003，可用 list_images 查看当前可用标签', ''];
    const question = String(parsed.question || '').trim().slice(0, 100);

    const lines: string[] = [];
    const missing: string[] = [];
    for (const tag of tags) {
        const entry = getCacheEntry(tag);
        if (!entry) {
            missing.push(tag);
            continue;
        }
        const desc = describeEntry(tag, entry.source, question);
        lines.push(`[${tag}] ${desc || '（描述生成失败，源图可能已失效）'}`);
    }
    if (lines.length === 0) {
        return [`取图失败：${missing.join('、')} 均已过期或不存在（缓存仅保留最近 ${IMAGE_CACHE_CAPACITY} 张未钉住的图片，当前窗口上限 ${IMAGE_WINDOW_SIZE} 张）。请调用 list_images 查看当前可用标签`, ''];
    }
    let text = `共取回 ${lines.length} 张图片的内容描述：\n${lines.join('\n')}`;
    if (missing.length > 0) text += `\n以下标签已过期，无法取回：${missing.join('、')}（可调用 list_images 查看当前可用标签）`;
    console.log(`[图片描述] 已取回 ${lines.length} 张图片描述${missing.length ? `，${missing.length} 个标签过期` : ''}`);
    return [text, ''];
}

// ==== 模块级注册 ====

GlobalConfig.LTPfunction.set('list_images', handleListImages);
GlobalConfig.LTPfunction.set('describe_images', handleDescribeImages);
