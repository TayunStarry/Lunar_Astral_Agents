package model

import (
	"sync"
	"time"
)

// OpenAI V1 消息结构
type Message struct {
	// 消息角色，指示消息发送者的身份
	Role string `json:"role"` // system, user, assistant
	// 消息内容，包含用户输入或模型生成的回复
	Content any `json:"content"`
}

// OpenAI V1 请求结构
type AgentRequest struct {
	// 模型名称，指定要使用的OpenAI模型
	Model string `json:"model"`
	// 消息列表，包含请求中的所有消息
	Messages []Message `json:"messages"`
	// 工具列表，包含请求中定义的工具
	Tools []any `json:"tools,omitempty"`
}

// OpenAI V1 响应结构
type AgentResponse struct {
	// 响应ID，用于唯一标识每个响应
	ID string `json:"id"`
	// 对象类型，固定为"chat.completion"
	Object string `json:"object"`
	// 创建时间，Unix时间戳格式
	Created int64 `json:"created"`
	// 使用的模型名称
	Model string `json:"model"`
	// 选择列表，包含模型生成的回复
	Choices []Choice `json:"choices"`
	// 使用信息，包含令牌使用统计
	Usage UsageInfo `json:"usage"`
}

// OpenAI V1 选择结构
type Choice struct {
	// 选择索引，从0开始
	Index int `json:"index"`
	// 消息内容，包含模型生成的回复
	Message Message `json:"message"`
	// 完成原因，指示模型生成回复的结束条件
	FinishReason string `json:"finish_reason"`
}

// OpenAI V1 使用信息结构
type UsageInfo struct {
	// 提示令牌数
	PromptTokens int `json:"prompt_tokens"`
	// 完成令牌数
	CompletionTokens int `json:"completion_tokens"`
	// 总令牌数
	TotalTokens int `json:"total_tokens"`
}

// OpenAI V1 模型结构
type AgentModels struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// 请求上下文结构
type RequestContext struct {
	// 请求ID，用于唯一标识每个请求
	ID string
	// 消息列表，包含请求中的所有消息
	Messages []Message
	// 工具列表，包含请求中定义的工具
	Tools []any
	// 响应通道，用于将处理后的响应发送回客户端
	ResponseChannel chan AgentResponse
	// 请求创建时间，用于过期请求清理
	CreatedAt time.Time
}

// embedding OpenAI V1 嵌入响应结构
type embeddingResp struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// 最大队列长度
var maxQueueLength = 3

// 当前正在处理的请求数
var currentProcessing int

// 请求队列
var requestQueue []chan struct{}

// 队列锁
var queueMutex sync.Mutex
