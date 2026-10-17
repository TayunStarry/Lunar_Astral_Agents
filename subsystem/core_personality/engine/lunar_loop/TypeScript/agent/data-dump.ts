import { GlobalConfig } from '../config/global';
import { PostMessage } from '../config/model';
import { ModelBuilder } from './base/builder';

// ==== 数据导出（TUI 检修用） ====
//
// 宿主（Go 侧）经运行时全局函数 dumpForJS(agent) 触发，本模块把「月华主上下文」或
// 「某个子智能体的上下文」序列化为 JSON 文本回传，由宿主落盘为 .json 文件。
//
// 为什么由 JS 侧产出：智能体上下文（messages）只在 goja 运行时内，Go 侧不可见；
// 由此处统一导出，保证「看到的上下文」就是模型真正收到的那份（含工具调用与图片项）。
//
// 设计取舍：
//   - 不引入新的导出编排：导出即序列化 + 交宿主写盘，不做过滤与裁剪（除体积护栏），
//     以便排查「模型到底看到了什么」这类问题时信息不丢失；
//   - 图片 base64 体积巨大（单张可达数百 KB），默认截断为占位摘要，避免导出文件失控；
//     需要原始数据时用 includeImages 开关；
//   - 角色注册表由 roles.ts 注入（roles.ts 是本模块的调用方，反向 import 会形成循环）。

/** 单张图片 base64 保留的最大字符数（超出截断；0 表示不截断） */
const IMAGE_KEEP_CHARS = 0;

/** 导出体积上限（字符）：超出时截断末尾并标注，防止把内存写爆 */
const DUMP_MAX_CHARS = 32 * 1024 * 1024;

/** 可导出角色：agent 名 → 实例与中文标签 */
export type DumpRoleEntry = { role: ModelBuilder; label: string };

/** 角色注册表（由 roles.ts 在模块初始化时注入，避免 data-dump → roles → data-dump 循环） */
let roleRegistry: Record<string, DumpRoleEntry> = {};

/** 注入可导出角色注册表（roles.ts 调用一次） */
export function registerDumpRoles(entries: Record<string, DumpRoleEntry>): void {
    roleRegistry = entries;
}

/** 可导出的目标名（TUI 列表用）：lunar 为月华主上下文，其余为子智能体 */
export function dumpTargets(): string[] {
    return ['lunar', ...Object.keys(roleRegistry)];
}

/** 目标是否可导出 */
export function hasDumpTarget(agent: string): boolean {
    return agent === 'lunar' || !!roleRegistry[agent];
}

/**
 * 压缩消息内容项中的图片体积：仅保留 data URI 头部与长度摘要。
 *
 * @param content 消息内容（字符串或内容项数组）
 * @param includeImages 是否保留原始 base64
 */
function shrinkContent(content: unknown, includeImages: boolean): unknown {
    if (typeof content === 'string') return content;
    if (!Array.isArray(content)) return content;
    return content.map(item => {
        if (!item || typeof item !== 'object') return item;
        const anyItem = item as { type?: string; image_url?: { url?: string } };
        if (anyItem.type !== 'image_url' || !anyItem.image_url) return item;
        const url = anyItem.image_url.url || '';
        if (includeImages || url.length <= IMAGE_KEEP_CHARS) return item;
        const head = url.slice(0, 64);
        return {
            type: 'image_url',
            image_url: { url: `${head}…（base64 已省略 ${url.length} 字符；需要原图请用 includeImages）` },
        };
    });
}

/** 消息数组 → 导出结构（保留角色、工具调用、内容项类型） */
function exportMessages(messages: PostMessage[], includeImages: boolean): unknown[] {
    return messages.map(message => ({
        role: message.role,
        content: shrinkContent((message as { content?: unknown }).content, includeImages),
        tool_call_id: (message as { tool_call_id?: string }).tool_call_id || undefined,
        tool_calls: (message as { tool_calls?: unknown }).tool_calls || undefined,
    }));
}

/** 单个智能体的上下文快照 */
function snapshotAgent(agent: string, includeImages: boolean): { target: string; label: string; messages: unknown[]; systemPrompt?: string } {
    if (agent === 'lunar') {
        return {
            target: 'lunar',
            label: '月华主上下文（未读队列 + 长期记忆摘要）',
            messages: exportMessages(GlobalConfig.unreadContext, includeImages),
        };
    }
    const entry = roleRegistry[agent];
    return {
        target: agent,
        label: entry.label,
        messages: exportMessages(entry.role.messages, includeImages),
        systemPrompt: (entry.role as unknown as { systemPrompt?: string }).systemPrompt,
    };
}

/**
 * 导出上下文为 JSON 文本（宿主 dumpForJS 的调用入口）。
 *
 * @param argsJSON `{agent, includeImages?}`；agent 为 'lunar' 或子智能体名
 * @returns JSON 文本（附带 generated_at / 各目标消息条数，便于快速判读）
 */
export function buildContextDump(argsJSON: string): string {
    let args: { agent?: string; includeImages?: boolean } = {};
    try {
        args = argsJSON ? JSON.parse(argsJSON) : {};
    }
    catch (e) {
        return JSON.stringify({ error: '导出参数解析失败: ' + String(e) });
    }
    const agent = String(args.agent || '').trim() || 'lunar';
    const includeImages = args.includeImages === true;
    if (!hasDumpTarget(agent)) {
        return JSON.stringify({
            error: `未知导出目标: ${agent}`,
            available: dumpTargets(),
        });
    }

    const snap = snapshotAgent(agent, includeImages);
    /** 各智能体的消息条数概览（一眼看出谁在积累上下文） */
    const overview: Record<string, number> = { lunar: GlobalConfig.unreadContext.length };
    for (const [name, entry] of Object.entries(roleRegistry)) {
        overview[name] = entry.role.messages.length;
    }

    const payload = {
        kind: 'lunar-agent-context',
        generated_at: new Date().toISOString(),
        target: snap.target,
        label: snap.label,
        include_images: includeImages,
        message_count: snap.messages.length,
        context_overview: overview,
        system_prompt: snap.systemPrompt,
        messages: snap.messages,
    };
    let text = JSON.stringify(payload, null, 2);
    if (text.length > DUMP_MAX_CHARS) {
        text = text.slice(0, DUMP_MAX_CHARS) + `\n…（导出超过 ${DUMP_MAX_CHARS} 字符，已截断）`;
    }
    return text;
}

/** 全局导出函数自检（宿主未注册 dumpForJS 时给出可读错误，而不是静默失败） */
export function ensureDumpBridge(): boolean {
    return typeof dumpForJS === 'function';
}

/**
 * 向宿主注册上下文导出入口：把 buildContextDump 挂到运行时全局，
 * 供 Go 侧（TUI / HTTP 端点）经 agent 事件循环调用。
 *
 * 宿主桥缺失时打一条 warn——数据导出是检修能力，缺失不应影响智能体主流程。
 */
export function bindDumpBridge(): void {
    if (!ensureDumpBridge()) {
        console.warn('[数据导出] 宿主未注册 dumpForJS，TUI 数据导出不可用');
        return;
    }
    (globalThis as unknown as Record<string, unknown>).buildContextDump = buildContextDump;
    console.log('[数据导出] 上下文导出入口已注册（目标：' + dumpTargets().join('、') + '）');
}
