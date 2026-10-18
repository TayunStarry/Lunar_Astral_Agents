import { ToolCall } from '../config/tool';
import { GlobalConfig } from '../config/global';
import { resolveImageRef, sanitizeImageDesc } from '../capabilities/image-tag';
import { describeResource } from '../capabilities/describe';
import { getResourceEntry, listResources } from '../capabilities/resource';

// ==== 图片工具（对话者可用） ====
//
// 统一资源句柄（@I.00x）是图片的**唯一**标识：持久、重启可复活、回收后降级为描述。
//   list_images      列出可引用的图片句柄
//   describe_images  按句柄取回图片内容描述（视觉模型解读，不注入图片本体）
//
// 清单只列句柄与描述，不标注「窗内/窗外」：可见性由上下文自身表达 ——
// 最近 20 张图片在消息里就是真实图片项，更早的只剩 `[@I.00x]` 标签文本。
// 历史 `图片.00x` 标签体系已废弃：该写法一律视为无效句柄，不再做任何兼容映射。
// 图片描述统一走 capabilities/describe 的 describeResource（它自己持有描述角色），
// 因此本模块不再需要单独注入描述角色。

/** 清单条数上限（防御模型索要过多条目） */
const LIST_LIMIT_MAX = 100;

/** 图片工具定义 */
export const imageRecallTools: ToolCall[] = [
    {
        type: "function",
        function: {
            name: "list_images",
            description: "列出当前可引用的图片句柄清单（句柄 + 内容描述），按登记时间倒序，只返回文字不含图片。当你不确定某张图片的句柄、或用户提到一张你已看不到的图片时，先用本工具查看清单，再用 describe_images 按句柄取内容描述；需要正文式的统一清单（含文档/视频/音频）请用 list_resources。",
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
            description: "按图片句柄获取图片的内容描述（视觉模型解读，不注入图片本体，上下文成本极低）。占位文本 [@I.00x]（…）括号内已是内容描述，无需再调用；需要了解描述之外的细节（如「图上写了什么字」、画面里第二个人的穿着）时可带 question 定向提问。句柄可从消息中的旁注占位、list_images 清单或合并转发文本中获得；交给绘图者参照创作时无需调用，直接在 dispatch_painter 的 reference_tag 里写句柄。",
            parameters: {
                type: "object",
                properties: {
                    tags: {
                        type: "array",
                        items: { type: "string" },
                        description: "图片句柄数组，如 [\"@I.003\", \"@I.007\"]（写作 [@I.003] 也可以）"
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

/**
 * 生成条目描述：资源目录摘要优先；未命中时交给统一描述能力生成并回写
 * （定向问题只回本次调用，不污染共用摘要）
 *
 * 旧标签体系废弃后，描述链路只剩资源目录一条：不再有「按缓存内部编号取描述」的分支。
 *
 * @param tag 规范句柄（@I.00x）
 * @param question 可选定向问题
 * @returns 规整后的描述（失败返回空串）
 */
function describeEntry(tag: string, question: string): string {
    const entry = getResourceEntry(tag);
    if (entry?.summary && !question) return sanitizeImageDesc(entry.summary, 200);
    const described = describeResource(tag, question);
    return described || sanitizeImageDesc(entry?.summary || '', 200);
}

/**
 * 处理句柄清单工具：返回可用图片句柄与内容描述
 *
 * 清单以**资源目录为唯一来源**（权威、含本体已回收的条目）。
 * 不标注「窗内/窗外」：模型自己看得到上下文里的图片项，看不到的只剩标签文本。
 */
async function handleListImages(args?: Record<string, any> | string): Promise<string[]> {
    const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
    /** 条目上限：默认 30 */
    const limit = Math.min(Math.max(Number(parsed.limit) || 30, 1), LIST_LIMIT_MAX);

    const lines: string[] = [];
    for (const entry of listResources('I', limit)) {
        const desc = entry.summary ? sanitizeImageDesc(entry.summary, 40) : '（暂无描述，可用 describe_images 获取）';
        const lost = entry.state === 'active' ? '' : ' [本体已回收]';
        lines.push(`[${entry.handle}] ${desc}${lost}`);
    }

    if (lines.length === 0) return ['当前没有可引用的图片：资源目录中还没有任何图片入库', ''];
    const text = `当前可引用的图片句柄（共 ${lines.length} 条，越靠前越新）：`
        + `\n${lines.join('\n')}`
        + '\n句柄形如 @I.003，来自统一资源目录：重启后依然有效，本体被回收时会降级为描述。';
    return [text, ''];
}

/** 处理图片描述工具：句柄数组规整去重 → 逐个取条目 → 返回描述文本（纯文本结果，不注入图片） */
async function handleDescribeImages(args?: Record<string, any> | string): Promise<string[]> {
    const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
    /** 句柄入参：兼容数组与单个句柄两种形态 */
    const rawTags: unknown[] = Array.isArray(parsed.tags) ? parsed.tags : (parsed.tag ? [parsed.tag] : []);
    const resolved = rawTags
        .map(item => resolveImageRef(String(item || '')))
        .filter((item): item is NonNullable<typeof item> => item !== null);
    if (resolved.length === 0) {
        return ['取图失败：请提供图片句柄数组，如 ["@I.003"]；句柄可用 list_images 或 list_resources 查看', ''];
    }
    const question = String(parsed.question || '').trim().slice(0, 100);

    const lines: string[] = [];
    for (const ref of resolved) {
        const desc = describeEntry(ref.handle, question);
        lines.push(`[${ref.handle}] ${desc || '（描述生成失败，源图可能已失效）'}`);
    }
    let text = `共取回 ${lines.length} 张图片的内容描述：\n${lines.join('\n')}`;
    if (resolved.length !== rawTags.length) {
        text += `\n（有 ${rawTags.length - resolved.length} 个句柄无法解析，已跳过；可用 list_images 查看当前可引用句柄）`;
    }
    console.log(`[图片描述] 已取回 ${lines.length} 张图片描述`);
    return [text, ''];
}

/** JSON 解析容错（模型偶发传入非 JSON 字符串） */
function safeParse(text: string): Record<string, any> {
    try {
        const parsed = JSON.parse(text);
        return parsed && typeof parsed === 'object' ? parsed : {};
    }
    catch {
        return {};
    }
}

// ==== 模块级注册 ====

GlobalConfig.LTPfunction.set('list_images', handleListImages);
GlobalConfig.LTPfunction.set('describe_images', handleDescribeImages);
