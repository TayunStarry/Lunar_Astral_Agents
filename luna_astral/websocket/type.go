package websocket

import "github.com/gorilla/websocket"

// WebSocket 客户端结构
type WSClient struct {
	// WebSocket 连接
	conn *websocket.Conn
	// 发送消息通道
	send chan []byte
	// 客户端引用
	client *WSClient
}

// WebSocket 消息结构
type WSMessage struct {
	// 消息类型
	Type string `json:"type"`
	// 消息数据
	Data any `json:"data,omitempty"`
}

// WebSocket 响应结构
type WSResponse struct {
	// 响应类型
	Type string `json:"type"`
	// 响应数据
	Data any `json:"data,omitempty"`
	// 响应上下文
	Context any `json:"context,omitempty"`
	// 响应图片
	Image any `json:"image,omitempty"`
}
