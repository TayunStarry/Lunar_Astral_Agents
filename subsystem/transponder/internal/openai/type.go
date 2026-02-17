package openai

// Message OpenAI消息结构体
type Message struct {
	// 消息角色
	Role string `json:"role"`
	// 消息内容
	Content any `json:"content,omitempty"`
	// 工具调用
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// 工具调用ID
	ToolCallID string `json:"tool_call_id,omitempty"`
	// 工具名称
	Name string `json:"name,omitempty"`
}

// ImageContent OpenAI图片消息结构体
type ImageContent struct {
	// 消息类型
	Type string `json:"type"`
	// 图片URL
	ImageURL map[string]string `json:"image_url"`
}

// Request OpenAI请求结构体
type Request struct {
	// 模型名称
	Model string `json:"model"`
	// 消息列表
	Messages []Message `json:"messages"`
	// 工具列表
	Tools []Tool `json:"tools,omitempty"`
	// 工具选择
	ToolChoice string `json:"tool_choice,omitempty"`
}

// Response OpenAI响应结构体
type Response struct {
	// 回复列表
	Choices []struct {
		// 回复消息
		Message Message `json:"message"`
	} `json:"choices"`
}

// Client OpenAI客户端
type Client struct {
	// API URL
	apiURL string
	// API Token
	token string
	// 模型名称
	model string
	// 最大上下文长度
	maxContext int
}
