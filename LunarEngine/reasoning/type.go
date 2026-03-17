package execute

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
