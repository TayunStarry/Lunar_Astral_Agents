// ═══════════════════════════════════════════════════════════
// 公共类型（被 agentAPI 按需引用）
// ═══════════════════════════════════════════════════════════

/** OpenAI 兼容消息 */
interface PostMessage {
	/** 消息角色 */
	role: 'user' | 'assistant' | 'system' | 'tool'
	/** 消息内容 */
	content: string | Array<TextItem | ImageItem>
	/** assistant 消息携带的函数调用（OpenAI 兼容 tool_calls） */
	tool_calls?: ToolCall[]
}

/** 文本项 */
interface TextItem {
	type: 'text'
	/** 文本内容 */
	text: string
}

/** 图片项 */
interface ImageItem {
	type: 'image_url'
	/** 图片地址 */
	image_url: {
		url: string
	}
}

/** OpenAI 兼容函数工具定义（agent.chat 的 opts.tools 项） */
interface ChatTool {
	type: 'function'
	/** 函数定义体 */
	function: {
		/** 函数名 */
		name: string
		/** 函数描述 */
		description?: string
		/** JSON Schema 形式的参数定义 */
		parameters?: Record<string, unknown>
	}
}

/** OpenAI 兼容模型函数调用（agent.chat 返回的 tool_calls 项） */
interface ToolCall {
	/** 调用 id（回传 tool 消息时填写 tool_call_id） */
	id: string
	type: 'function'
	/** 调用详情 */
	function: {
		/** 函数名 */
		name: string
		/** JSON 字符串形式的实参 */
		arguments: string
	}
}

/** 工具结果消息（role='tool'，回传某次函数调用的执行结果） */
interface ToolMessage {
	role: 'tool'
	/** 对应 ToolCall.id */
	tool_call_id: string
	/** 执行结果文本 */
	content: string
}
