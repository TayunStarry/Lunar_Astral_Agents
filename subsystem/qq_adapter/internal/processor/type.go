package processor

import (
	"QQAdapter/internal/setup"
	"QQAdapter/internal/utils"
)

// EmbeddingRequestBody 嵌入请求体结构
type EmbeddingRequestBody struct {
	// 模型名称
	Model string `json:"model"`
	// 输入文本列表
	Input []string `json:"input"`
	// 任务类型
	TaskType string `json:"task_type"`
	// 期望的嵌入向量维度
	Dimensionality int `json:"dimensionality"`
}

// EmbeddingResponse 嵌入响应结构
type EmbeddingResponse struct {
	// 响应对象类型
	Object string `json:"object"`
	// 响应数据列表
	Data []EmbeddingResponseData `json:"data"`
	// 模型名称
	Model string `json:"model"`
	// 响应使用统计
	Usage EmbeddingResponseUsage `json:"usage"`
}

// EmbeddingResponseData 嵌入响应数据结构
type EmbeddingResponseData struct {
	// 响应对象类型
	Object string `json:"object"`
	// 嵌入向量
	Embedding []float64 `json:"embedding"`
	// 索引位置
	Index int `json:"index"`
}

// EmbeddingResponseUsage 嵌入响应使用统计结构
type EmbeddingResponseUsage struct {
	// 提示令牌数
	PromptTokens int `json:"prompt_tokens"`
	// 总令牌数
	TotalTokens int `json:"total_tokens"`
}

// FusionMessage 消息内容接口
type FusionMessage interface {
	// 检查消息内容是否有效
	MessageValid() bool
}

// KnowledgeMessage 知识库消息结构
type KnowledgeMessage struct {
	// 消息角色，固定为 "assistant"
	Role string `json:"role"`
	// 消息内容
	Content string `json:"content"`
	// 图片URL
	ImageUrl string `json:"imageUrl"`
	// 消息UUID
	UUID string `json:"uuid"`
}

// MultimodalMessage 多模态消息结构
type MultimodalMessage struct {
	// 消息角色
	Role string `json:"role"`
	// 消息内容
	Content ProcessResult `json:"content"`
	// 工具调用
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// 工具调用ID
	ToolCallID string `json:"tool_call_id,omitempty"`
	// 工具名称
	Name string `json:"name,omitempty"`
}

// BaseMessage 基础消息结构
type BaseMessage struct {
	// 消息角色
	Role string `json:"role"`
	// 消息内容
	Content string `json:"content"`
	// 工具调用
	ToolCalls any `json:"tool_calls,omitempty"`
	// 工具调用ID
	ToolCallID string `json:"tool_call_id,omitempty"`
	// 工具名称
	Name string `json:"name,omitempty"`
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

// ImageObjectParameter 图片对象参数
type ImageObjectParameter struct {
	// 图片URL
	URL string `json:"url"`
	// 文件URL
	File string `json:"file"`
}

// TextMessage 文本消息
type TextMessage struct {
	// 消息类型
	Type string `json:"type"`
	// 文本内容
	Text string `json:"text"`
}

// ImageMessage 图片消息
type ImageMessage struct {
	// 消息类型
	Type string `json:"type"`
	// 图片URL
	ImageURL ImageURL `json:"image_url"`
}

// ImageURL 图片URL结构
type ImageURL struct {
	// 图片URL
	URL string `json:"url"`
	// 图片详情
	Detail string `json:"detail"`
}

// MessageContent 消息内容接口
type MessageContent interface {
	// 检查消息内容是否有效
	ProcessValid() bool
}

// ProcessResult Process函数的返回类型
type ProcessResult []FusionMessage

// Handle 消息处理器
type Handle struct {
	// 配置信息
	Config *setup.Config
	// 群信息列表
	groupInfos []setup.GroupInfo
	// WebSocket 客户端
	wsClient *utils.Client
	// 群成员映射，键为群ID，值为用户ID到昵称的映射
	groupMembers map[int64]map[int64]string
	// OpenAI API 基础URL
	BaseURL string
	// 当前处理的群ID
	currentGroupID int64
}

// SendGroupMsgParams 发送群消息请求参数
type SendGroupMsgParams struct {
	// 群ID
	GroupID int64 `json:"group_id"`
	// 消息内容
	Message []setup.MessageItem `json:"message"`
}
