import { GlobalConfig } from '../config/global';
import { PostMessage } from '../config/model';
import { ModelBuilder } from './base/builder';

// ==== 引擎重载的上下文抢救（TUI 手动重载 / 致命错误自动重载共用）====
//
// 为什么需要它：重载 goja 运行时意味着把 agentSystem.js 放进一个全新的虚拟机重新执行——
// 旧运行时里的上下文（对话历史、未读消息、思考中标记）都会随旧 VM 一起消失。
// 用户已明确取舍：**维持思维链 > 保住上下文**，所以这里只做"尽力抢救"：
//   - 必救：未读消息队列（丢了就等于用户那句话没说过）；
//   - 尽量救：对话者上下文（对话记忆），但**图片一律换成占位**——
//     20 张图各几百 KB 的 base64 会把快照撑到几十 MB，重载一次要卡好几秒，
//     而重载本身已经是"出事了"的兜底动作，不值得为图片付这个代价。
//
// 边界：解析失败、版本不符、角色未注册时一律**返回说明而不是抛异常**——
// 这条链在故障恢复路径上，它自己再抛错就等于把最后一道兜底也砸了。

/** 快照版本（结构变更时递增；恢复侧只接受同版本） */
const RECOVERY_VERSION = 1;

/** 图片项被省略后的占位文本 */
const IMAGE_PLACEHOLDER = '[图片：引擎重载时已省略]';

/** 参与抢救的智能体角色（由 roles.ts 注入，避免 recovery → roles 的循环依赖） */
export interface RecoveryRoles {
	/** 对话者（月华主上下文） */
	dialogue: ModelBuilder;
}

let recoveryRoles: RecoveryRoles | null = null;

/** 注入参与抢救的角色（roles.ts 在模块初始化时调用一次） */
export function registerRecoveryRoles(roles: RecoveryRoles): void {
	recoveryRoles = roles;
}

/** 把消息内容里的图片 data URI 换成占位（保留文本与工具调用结构） */
function shrinkContent(content: unknown): unknown {
	if (typeof content === 'string') return content;
	if (!Array.isArray(content)) return content;
	return content.map(item => {
		if (!item || typeof item !== 'object') return item;
		const anyItem = item as { type?: string; image_url?: { url?: string } };
		if (anyItem.type !== 'image_url' || !anyItem.image_url) return item;
		return { type: 'image_url', image_url: { url: IMAGE_PLACEHOLDER } };
	});
}

/** 消息数组 → 可序列化快照（保留 role / content / tool_call_id / tool_calls） */
function exportMessages(messages: PostMessage[]): unknown[] {
	return messages.map(message => ({
		role: message.role,
		content: shrinkContent((message as { content?: unknown }).content),
		tool_call_id: (message as { tool_call_id?: string }).tool_call_id || undefined,
		tool_calls: (message as { tool_calls?: unknown }).tool_calls || undefined,
	}));
}

/** 恢复侧：把快照消息还原为 PostMessage（丢弃不认识的行，不抛异常） */
function importMessages(raw: unknown): PostMessage[] {
	if (!Array.isArray(raw)) return [];
	const out: PostMessage[] = [];
	for (const item of raw) {
		if (!item || typeof item !== 'object') continue;
		const row = item as { role?: string; content?: unknown; tool_call_id?: string; tool_calls?: unknown };
		if (!row.role) continue;
		const message = {
			role: row.role,
			content: row.content === undefined ? '' : row.content,
			tool_call_id: row.tool_call_id,
			tool_calls: row.tool_calls,
		} as PostMessage;
		out.push(message);
	}
	return out;
}

/**
 * 生成引擎重载前的上下文快照（JSON 文本）
 *
 * 由宿主在关闭运行时**之前**调用（goja 层经 `agentSystem.snapshotContext()` 暴露）。
 */
export function snapshotAgentContext(): string {
	try {
		const dialogue = recoveryRoles ? exportMessages(recoveryRoles.dialogue.messages) : [];
		const payload = {
			kind: 'lunar-agent-recovery',
			version: RECOVERY_VERSION,
			at: new Date().toISOString(),
			unread: exportMessages(GlobalConfig.unreadContext),
			dialogue: dialogue,
		};
		return JSON.stringify(payload);
	}
	catch (error) {
		console.error('[引擎恢复] 生成上下文快照失败:', (error as Error)?.message ?? error);
		return '';
	}
}

/**
 * 恢复引擎重载前的上下文（在**新运行时**里执行）
 *
 * @param jsonText 快照文本（空串表示没有可恢复内容）
 * @returns 人类可读的恢复结果（写进宿主日志）
 */
export function restoreAgentContext(jsonText: string): string {
	// 重载后重新进入"空闲"状态：思考中标记绝不能从快照里恢复成 true，
	// 否则新运行时一启动就被自己锁死（那正是"思维链停摆"的形态之一）
	GlobalConfig.reasoningInProgress = false;
	GlobalConfig.lastFailure = '';
	if (!jsonText || !jsonText.trim()) return '没有可恢复的上下文（快照为空）';
	let payload: { kind?: string; version?: number; unread?: unknown; dialogue?: unknown };
	try {
		payload = JSON.parse(jsonText);
	}
	catch (error) {
		return `快照解析失败，未恢复上下文: ${(error as Error)?.message ?? error}`;
	}
	if (payload?.kind !== 'lunar-agent-recovery' || payload?.version !== RECOVERY_VERSION) {
		return `快照版本不符（期望 v${RECOVERY_VERSION}），未恢复上下文`;
	}
	const unread = importMessages(payload.unread);
	const dialogue = importMessages(payload.dialogue);
	// 未读队列按"追加"恢复：重载期间宿主可能又收进了新消息，不能覆盖它们
	if (unread.length > 0) GlobalConfig.unreadContext = [...unread, ...GlobalConfig.unreadContext];
	if (dialogue.length > 0 && recoveryRoles) recoveryRoles.dialogue.coverContext(dialogue);
	return `已恢复 ${dialogue.length} 条对话上下文、${unread.length} 条未读消息`;
}
