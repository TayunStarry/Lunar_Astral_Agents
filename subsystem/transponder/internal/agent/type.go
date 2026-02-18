package agent

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

// ImageContent Agent图片消息结构体
type ImageContent struct {
	// 消息类型
	Type string `json:"type"`
	// 图片URL
	ImageURL map[string]string `json:"image_url"`
}

// Request Agent请求结构体
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

// Response Agent响应结构体
type Response struct {
	// 回复列表
	Choices []ResponseItem `json:"choices"`
}

// ResponseItem 回复消息结构体
type ResponseItem struct {
	// 回复消息
	Message Message `json:"message"`
}

// Client Agent客户端
type Client struct {
	// API URL
	agentURL string
	// API Token
	token string
	// 模型名称
	model string
	// 最大上下文长度
	maxContext int
}

// Tool 工具结构体
type Tool struct {
	// 工具类型，固定为 "function"
	Type string `json:"type"`
	// 函数定义
	Function FunctionDef `json:"function"`
}

// FunctionDef 函数定义结构体
type FunctionDef struct {
	// 函数名称
	Name string `json:"name"`
	// 函数描述
	Description string `json:"description"`
	// 参数定义
	Parameters ParameterDef `json:"parameters"`
}

// ParameterDef 参数定义结构体
type ParameterDef struct {
	// 参数类型，固定为 "object"
	Type string `json:"type"`
	// 参数属性定义
	Properties map[string]PropertyDef `json:"properties"`
	// 必填参数列表，必须包含在调用参数中
	Required []string `json:"required"`
}

// PropertyDef 属性定义结构体
type PropertyDef struct {
	// 属性类型，如 "string", "number", "boolean" 等
	Type string `json:"type"`
	// 属性描述，用于解释属性的作用
	Description string `json:"description"`
}

// ToolCall 工具调用结构体
type ToolCall struct {
	// 工具调用类型，固定为 "function"
	Type string `json:"type"`
	// 工具调用 ID，用于关联响应
	ID string `json:"id"`
	// 工具调用函数定义
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction 工具调用函数定义结构体
type ToolCallFunction struct {
	// 调用的函数名称
	Name string `json:"name"`
	// 函数调用参数，JSON 字符串格式，符合 Parameters 定义
	Arguments string `json:"arguments"`
}

// ToolCallResponse 工具调用响应结构体
type ToolCallResponse struct {
	// 响应角色，固定为 "assistant"
	Role string `json:"role"`
	// 响应内容，通常为空字符串
	Content string `json:"content"`
	// 包含的工具调用列表，每个调用都有 ID、名称和参数
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolResponse 工具响应结构体
type ToolResponse struct {
	// 响应角色，固定为 "assistant"
	Role string `json:"role"`
	// 响应内容，通常为空字符串
	Content string `json:"content,omitempty"`
	// 关联的工具调用 ID，用于匹配 ToolCall
	ToolCallID string `json:"tool_call_id"`
	// 工具名称，与 ToolCall 中的 Name 一致
	Name string `json:"name"`
}

// RequestGeneration 图片生成请求结构体
type RequestGeneration struct {
	// 生成提示
	Prompt string `json:"prompt"`
	// 负提示
	NegativePrompt string `json:"negative_prompt"`
	// 是否使用参考图片
	UseReference bool `json:"use_reference"`
	// 强度，范围0-1，默认0.65
	Strength *float64 `json:"strength"`
	// CFG缩放，范围0-100，默认1.0
	CfgScale *float64 `json:"cfg_scale"`
}

// RequestContent 知识保存请求结构体
type RequestContent struct {
	// 内容
	Content string `json:"content"`
}
