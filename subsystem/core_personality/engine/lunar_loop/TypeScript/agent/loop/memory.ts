import { GlobalConfig } from '../../config/global';
import { extractTextFromMessage } from '../capabilities/memory';
import { dialogueRole, memorizerRole } from '../roles/roles';

/** 更新远期记忆摘要 */
export function updatePreviousMemories(): void {
	// 消息缓冲池非空时，触发记忆者智能体：将缓冲消息逐个写入记忆库后清空
	if (GlobalConfig.unreadRecords.length >= 1) memorizerRole.persistUnreadRecords();
	/** 提取未读消息中的用户消息文本（作为记忆库查询条件） */
	const userMessages = GlobalConfig.unreadContext.filter(message => message.role === 'user').map(message => extractTextFromMessage(message).trim()).filter(text => text.length > 0);
	/** 清理RAG消息并返回 */
	const clear = () => { dialogueRole.ragMessages = []; };
	// 如果没有用户消息，清理RAG消息并返回
	if (userMessages.length === 0) return clear();
	/** 记忆库返回的时间线文本（自动背景，不做 LLM 摘要） */
	const digest = memorizerRole.queryRagDigest(userMessages);
	// 记忆库返回为空时，清理RAG消息并返回（跳过拼接）
	if (!digest) return clear();
	// 将记忆时间线作为一条用户消息注入 ragMessages（替换碎片式上下文）
	dialogueRole.ragMessages = [{ role: 'user', content: `【长期记忆摘要】\n${digest}` }];
}
