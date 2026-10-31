package agentchat

// type.go 共享智能体模型调用的类型定义（OpenAI v1 兼容）。
//
// 本包只承载「模型怎么调用」这一层基础设施：视觉理解与文献解析等智能体各自
// 持有自己的提示词、工具与处理管线，但不必再各写一份 SSE 解析与工具调用回填。

// Message OpenAI v1 协议中的单条对话消息
type Message struct {
	// Role 角色（system/user/assistant/tool）
	Role string `json:"role"`
	// Content 内容（文本或图文片段数组）
	Content any `json:"content"`
	// ToolCalls 模型请求的工具调用（assistant 消息）
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID 工具结果对应的调用 ID（tool 消息）
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ContentPart 图文混排内容片段
type ContentPart struct {
	// Type 内容类型（text/image_url）
	Type string `json:"type"`
	// Text 文本内容（type=text）
	Text string `json:"text,omitempty"`
	// ImageURL 图片地址（type=image_url）
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL 图片地址（data URI）
type ImageURL struct {
	// URL 图片地址
	URL string `json:"url"`
}

// ToolCall 模型返回的工具调用
type ToolCall struct {
	// ID 调用 ID
	ID string `json:"id"`
	// Type 调用类型（function）
	Type string `json:"type"`
	// Function 函数名与 JSON 参数串
	Function ToolFunction `json:"function"`
}

// ToolFunction 工具调用的函数体
type ToolFunction struct {
	// Name 函数名
	Name string `json:"name"`
	// Arguments JSON 参数串
	Arguments string `json:"arguments"`
}

// ToolDefinition 暴露给模型的工具定义
type ToolDefinition struct {
	// Type 工具类型（function）
	Type string `json:"type"`
	// Function 函数描述
	Function ToolFunctionDef `json:"function"`
}

// ToolFunctionDef 工具的函数描述体
type ToolFunctionDef struct {
	// Name 函数名
	Name string `json:"name"`
	// Description 函数说明
	Description string `json:"description"`
	// Parameters JSON Schema 参数结构
	Parameters map[string]any `json:"parameters"`
}

// ChatOptions 单次模型调用的可选项
type ChatOptions struct {
	// Model 覆盖模型名（空 = 主多模态模型）
	Model string
	// Temperature 采样温度（nil = 沿用服务端默认）
	Temperature *float64
	// MaxTokens 最大输出 token（0 = 不限制）
	MaxTokens int
	// HoldLease 调用方已持有模型切换租约（如轻量扫描通道），
	// 不再获取普通调用租约，避免写锁与读锁在同一协程上重入死锁。
	HoldLease bool
}

// Reply 一次模型调用的结果（助手消息 + 思考文本 + 服务端用量）
type Reply struct {
	// Message 助手消息（可能携带工具调用）
	Message Message
	// Reasoning 思考全文（仅流式响应有值）
	Reasoning string
	// Usage 服务端回传的 token 用量（未回传时为 nil）
	Usage *Usage
}

// Usage OpenAI 兼容响应中的 token 用量
type Usage struct {
	// Prompt 上下文 token 数
	Prompt int64 `json:"prompt_tokens"`
	// Completion 输出 token 数
	Completion int64 `json:"completion_tokens"`
	// PromptTokensDetails 输入 token 的细分（提示缓存命中数）
	PromptTokensDetails *PromptTokenDetails `json:"prompt_tokens_details,omitempty"`
}

// PromptTokenDetails 输入 token 的细分
type PromptTokenDetails struct {
	// CachedTokens 命中提示缓存的输入 token 数
	CachedTokens int64 `json:"cached_tokens"`
}

// chatResponse 非流式响应体
type chatResponse struct {
	// Choices 选择项
	Choices []struct {
		// Message 助手消息
		Message Message `json:"message"`
	} `json:"choices"`
	// Usage token 用量
	Usage *Usage `json:"usage"`
	// Error 错误信息
	Error *ChatError `json:"error"`
}

// ChatError 模型错误信息
type ChatError struct {
	// Message 错误文本
	Message string `json:"message"`
}

// streamChunk 流式响应分片
type streamChunk struct {
	// Choices 选择项
	Choices []struct {
		// Delta 增量
		Delta struct {
			// Content 正文增量
			Content string `json:"content"`
			// ReasoningContent 思考增量
			ReasoningContent string `json:"reasoning_content"`
			// ToolCalls 工具调用增量
			ToolCalls []struct {
				// Index 调用序号
				Index int `json:"index"`
				// ID 调用 ID（首个分片携带）
				ID string `json:"id"`
				// Type 调用类型
				Type string `json:"type"`
				// Function 函数增量
				Function struct {
					// Name 函数名
					Name string `json:"name"`
					// Arguments 参数串增量
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	// Usage token 用量（include_usage 时随最后一个分片回传）
	Usage *Usage `json:"usage"`
	// Error 错误信息
	Error *ChatError `json:"error"`
}
