package execute

import "sync"

// Message 消息结构
type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// AgentRequest 请求结构
type AgentRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []any     `json:"tools,omitempty"`
}

// AgentModels 模型结构
type AgentModels struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// embeddingResp 嵌入响应结构
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
