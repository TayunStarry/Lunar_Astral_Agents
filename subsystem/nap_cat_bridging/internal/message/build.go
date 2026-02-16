package message

import (
	status "nap_cat_bridging/internal/config"
	"nap_cat_bridging/internal/websocket"
)

// Processor 消息处理器
type Processor struct {
	// 配置信息
	config *status.Config
	// 群信息列表
	groupInfos []status.GroupInfo
	// WebSocket 客户端
	wsClient *websocket.Client
	// 群成员映射，键为群ID，值为用户ID到昵称的映射
	groupMembers map[int64]map[int64]string
}

// SendGroupMsgParams 发送群消息请求参数
type SendGroupMsgParams struct {
	// 群ID
	GroupID int64 `json:"group_id"`
	// 消息内容
	Message []status.MsgItem `json:"message"`
}

// NewProcessor 创建消息处理器
func NewProcessor(config *status.Config, wsClient *websocket.Client) *Processor {
	return &Processor{
		config:       config,
		wsClient:     wsClient,
		groupInfos:   make([]status.GroupInfo, 0),
		groupMembers: make(map[int64]map[int64]string),
	}
}
