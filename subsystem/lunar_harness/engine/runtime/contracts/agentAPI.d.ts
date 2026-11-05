// ═══════════════════════════════════════════════════════════
// 模型能力（agent.*）
//
// 消息与工具类型见 commonAPI.d.ts（PostMessage / ToolMessage / ChatTool / ToolCall）。
// 模型路由为本地固定值，无需配置模型地址或密钥。
// ═══════════════════════════════════════════════════════════

/** 模型能力 */
export const agent: {
	/**
	 * 调用 v1/chat/completions 生成回复文本（同步阻塞）
	 *
	 * @param messages OpenAI 消息数组（PostMessage / ToolMessage）
	 * @param opts 可选参数 { temperature?, max_tokens?, system?, tools? }
	 *             system 为系统提示词字符串（自动前置为一条 system 消息）
	 *             tools  为给模型提供的函数工具定义（OpenAI 兼容 ChatTool[]）
	 * @returns 成功返回 { success:true, text: 回复内容, tool_calls?: ToolCall[] }；
	 *          模型走函数调用时 content 为空，tool_calls 原样透传（供 AtoA 接头）；失败返回 failure
	 */
	chat(messages: Array<PostMessage | ToolMessage>, opts?: { temperature?: number; max_tokens?: number; system?: string; tools?: ChatTool[] }): { success: boolean; text?: string; tool_calls?: ToolCall[]; error?: string }
	/**
	 * 调用进程内网络搜索，返回自然语言搜索报告
	 *
	 * @param instruction 自然语言搜索指令（可含「前N页/个」等数量要求）
	 * @returns { success, text: 搜索报告, error? }
	 */
	search(instruction: string): { success: boolean; text?: string; error?: string }
}
