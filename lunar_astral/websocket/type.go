package websocket

import (
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocket 客户端结构
type WSClient struct {
	// WebSocket 连接
	conn *websocket.Conn
	// 发送消息通道（阻塞式，由 writePump 消费）
	send chan []byte
	// 关闭信号通道：客户端任一侧断开后关闭，用于解除广播方的阻塞
	done chan struct{}
	// 确保 shutdown 仅执行一次
	once sync.Once
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
