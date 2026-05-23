package server

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// SystemEndpoint 定义系统端点的结构
type SystemEndpoint struct {
	// HTTP 访问路径
	Path string `json:"path"`
	// HTTP 方法处理器
	Handler http.HandlerFunc `json:"handler"`
	// HTTP 方法类型
	Method string `json:"method"`
	// 处理器功能描述
	Description string `json:"description"`
}

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
