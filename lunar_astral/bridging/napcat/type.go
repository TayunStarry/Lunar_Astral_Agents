package napcat

import (
	"encoding/json"
	"sync"
)

// ==================== 桥接器配置类型 ====================

// BridgingConfig 桥接器配置，从 lunar_config.json 的 server 段读取
type BridgingConfig struct {
	BridgingType     string   `json:"bridging_type"`
	BridgingPath     string   `json:"bridging_path"`
	BridgingToken    string   `json:"bridging_token"`
	BridgingTarget   int64    `json:"bridging_target"`
	BridgingKeywords []string `json:"bridging_keywords"`
}

// ==================== Napcat 消息类型 ====================

// NapcatMessage Napcat 上行消息结构
type NapcatMessage struct {
	SelfID      int64            `json:"self_id"`
	UserID      int64            `json:"user_id"`
	MessageID   int64            `json:"message_id"`
	Sender      Sender           `json:"sender"`
	GroupID     int64            `json:"group_id"`
	Message     []MessageSegment `json:"message"`
	PostType    string           `json:"post_type"`
	MessageType string           `json:"message_type"`
	RawMessage  string           `json:"raw_message"`
}

// Sender 发送者信息
type Sender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
}

// MessageSegment 消息段
type MessageSegment struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// TextData 文本消息数据
type TextData struct {
	Text string `json:"text"`
}

// AtData @消息数据
type AtData struct {
	QQ string `json:"qq"`
}

// ReplyData 回复消息数据
type ReplyData struct {
	ID string `json:"id"`
}

// ImageData 图片消息数据
type ImageData struct {
	Summary  string `json:"summary"`
	File     string `json:"file"`
	SubType  int    `json:"sub_type"`
	URL      string `json:"url"`
	FileSize string `json:"file_size"`
}

// ForwardData 转发消息数据
type ForwardData struct {
	ID string `json:"id"`
}

// FileData 文件消息数据
type FileData struct {
	File     string `json:"file"`
	FileID   string `json:"file_id"`
	FileSize string `json:"file_size"`
	URL      string `json:"url"`
}

// NapcatWSResponse Napcat HTTP API 响应结构
type NapcatWSResponse struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Wording string          `json:"wording"`
	Echo    string          `json:"echo"`
}

// ==================== 缓存类型 ====================

// CachedMessage 缓存的单条消息
type CachedMessage struct {
	GroupID   int64  // 来源群号
	UserID    int64  // 发送者ID
	Nickname  string // 发送者昵称
	Content   string // 文本内容
	HasImages bool   // 是否包含图片
}

// MessageCache 消息缓存容器
type MessageCache struct {
	Messages []CachedMessage
	mu       sync.RWMutex
}

// ==================== 桥接器状态类型 ====================

// BridgeState 桥接器连接状态
type BridgeState int

const (
	BridgeDisconnected BridgeState = iota // 未连接
	BridgeConnecting                      // 连接中
	BridgeConnected                       // 已连接
	BridgeFailed                          // 连接失败（达到最大重试次数）
)
