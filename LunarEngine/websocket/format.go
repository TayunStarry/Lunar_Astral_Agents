package websocket

import (
	"sync" // 用于同步操作，如互斥锁
	"time" // 用于时间操作，如时间戳

	"golang.org/x/net/websocket" // 用于WebSocket连接
)

// OpenAI V1 请求结构
type AgentRequest struct {
	// 模型名称，指定要使用的OpenAI模型
	Model string `json:"model"`
	// 消息列表，包含请求中的所有消息
	Messages []Message `json:"messages"`
	// 工具列表，包含请求中定义的工具
	Tools []interface{} `json:"tools,omitempty"`
}

// OpenAI V1 消息结构
type Message struct {
	// 消息角色，指示消息发送者的身份
	Role string `json:"role"` // system, user, assistant
	// 消息内容，包含用户输入或模型生成的回复
	Content interface{} `json:"content"`
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

// 请求上下文结构
type RequestContext struct {
	// 请求ID，用于唯一标识每个请求
	ID string
	// 消息列表，包含请求中的所有消息
	Messages []Message
	// 工具列表，包含请求中定义的工具
	Tools []interface{}
	// 响应通道，用于将处理后的响应发送回客户端
	ResponseChannel chan AgentResponse
	// 请求创建时间，用于过期请求清理
	CreatedAt time.Time
}

// 服务器状态
type ServerState struct {
	// 互斥锁，用于保护请求映射的并发访问
	mutex sync.RWMutex
	// 请求映射，键为请求ID，值为请求上下文
	requests map[string]*RequestContext
	// 服务器配置
	config ServerConfig
	// WebSocket连接
	websocketConn *websocket.Conn
	// WebSocket连接互斥锁
	wsMutex sync.Mutex
}

// 服务器配置
type ServerConfig struct {
	// 服务器监听端口
	Port string
	// 允许的CORS来源
	CORSAllowedOrigins []string
	// 请求超时时间
	RequestTimeout time.Duration
	// 最大并发请求数
	MaxRequests int
	// 过期请求清理间隔
	CleanupInterval time.Duration
}

// WebSocket消息结构
type WSMessage struct {
	// 消息类型，指示消息的目的或内容
	Type string `json:"type"`
	// 消息数据，包含具体的消息内容
	Data any `json:"data"`
	// 请求ID，可选字段，用于关联请求和响应
	RequestID string `json:"request_id,omitempty"`
}
