// ═══════════════════════════════════════════════════════════
// 公共类型（被 eventAPI / agentAPI 等按需引用）
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

/** 事件负载 */
interface EventLoad<P = unknown> {
	/** 事件主题 */
	type: string
	/** 事件数据 */
	payload: P
}

/** 事件回传：`{ return }` 即把业务结果交回宿主 */
interface ReturnResult<R> {
	/** 业务结果，格式见 eventAPI 对应事件 */
	return: R
}

/** 放行 */
interface PassResult { }

/** 拦截：短路后续订阅器 */
interface InterceptResult {
	intercept: true
}

/** 改写：替换事件负载 */
interface ModifyResult {
	modifiedData: {
		content?: string
		senderName?: string
		[key: string]: unknown
	}
}

/** 撤回：事件不再派发 */
interface CancelResult {
	cancel: true
}

/** 订阅器可返回的结果（推荐 `{ return: 业务结果 }` 回传宿主） */
type HandlerResult = PassResult | InterceptResult | ModifyResult | CancelResult | Record<string, unknown>
