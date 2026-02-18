package processor

import (
	"transponder/internal/setup"
	"transponder/internal/utils"
)

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

// ImageObjectParameter 图片对象参数
type ImageObjectParameter struct {
	// 图片URL
	URL string `json:"url"`
	// 文件URL
	File string `json:"file"`
}

// ImageURL 图片URL结构
type ImageURL struct {
	// 图片URL
	URL string `json:"url"`
	// 图片详情
	Detail string `json:"detail"`
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

// MessageContent 消息内容接口
type MessageContent interface {
	// 检查消息内容是否有效
	ProcessValid() bool
}

// ProcessResult Process函数的返回类型
type ProcessResult []MessageContent

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
	baseURL string
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
