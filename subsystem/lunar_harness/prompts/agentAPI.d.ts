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
	 * 计算文本的嵌入向量（本地固定文本嵌入模型）
	 *
	 * @param input 单条字符串或字符串数组
	 * @param opts 入参为空时的兜底：{ text? } 或 { texts? }
	 * @returns 单条返回 { success:true, embedding: number[] }；多条返回 { success:true, embeddings: number[][] }；失败返回 failure
	 */
	embed(input: string | string[], opts?: { text?: string; texts?: string[] }): { success: boolean; embedding?: number[]; embeddings?: number[][]; error?: string }
	/**
	 * 调用前端智能体（Mini-LTP / Node-LTP），以自然语言驱动目标智能体（同步阻塞）
	 *
	 * @param pkgId 目标包目录名（包 ID）
	 * @param text 要交给智能体的自然语言输入
	 * @returns { success, text: 执行结果文本, error? }；目标包不存在/拒绝响应时 success=false
	 */
	synergy(pkgId: string, text: string): { success: boolean; text?: string; error?: string }
	/**
	 * 调用进程内网络搜索，返回自然语言搜索报告
	 *
	 * @param instruction 自然语言搜索指令（可含「前N页/个」等数量要求）
	 * @returns { success, text: 搜索报告, error? }
	 */
	search(instruction: string): { success: boolean; text?: string; error?: string }
}
