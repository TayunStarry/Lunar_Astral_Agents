package agent

import "subsystem/component/processor"

// Request Agent请求结构体
type Request struct {
	// 模型名称
	Model string `json:"model"`
	// 消息列表
	Messages []processor.FusionMessage `json:"messages"`
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
	Message map[string]any `json:"message"`
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

// ToolReturn 工具返回结构体
type ToolReturn struct {
	// 工具返回ID
	ID string `json:"id"`
	// 工具类型，固定为 "function"
	Type string `json:"type"`
	// 函数调用参数
	Function ToolReturnData `json:"function"`
}

// ToolReturnData 工具返回数据结构体
type ToolReturnData struct {
	// 函数名称
	Name string `json:"name"`
	// 函数调用参数
	Arguments string `json:"arguments"`
}

// Tool 工具结构体
type Tool struct {
	// 工具类型，固定为 "function"
	Type string `json:"type"`
	// 函数定义
	Function FunctionData `json:"function"`
}

// FunctionData 函数定义结构体
type FunctionData struct {
	// 函数名称
	Name string `json:"name"`
	// 函数描述
	Description string `json:"description"`
	// 参数定义
	Parameters ParameterData `json:"parameters"`
}

// ParameterData 参数定义结构体
type ParameterData struct {
	// 参数类型，固定为 "object"
	Type string `json:"type"`
	// 参数属性定义
	Properties map[string]PropertyData `json:"properties"`
	// 必填参数列表，必须包含在调用参数中
	Required []string `json:"required"`
}

// PropertyData 属性定义结构体
type PropertyData struct {
	// 属性类型，如 "string", "number", "boolean" 等
	Type string `json:"type"`
	// 属性描述，用于解释属性的作用
	Description string `json:"description"`
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
