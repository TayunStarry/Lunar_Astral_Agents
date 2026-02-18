package utils

import "github.com/gorilla/websocket"

// WSAPIRequest WebSocket API 请求结构体
type WSAPIRequest struct {
	// Action 操作
	Action string `json:"action"`
	// Params 参数
	Params any `json:"params"`
	// Echo 回显
	Echo string `json:"echo,omitempty"`
}

// WSResponse WebSocket 响应结构体
type WSResponse struct {
	// Status 状态
	Status string `json:"status"`
	// Retcode 返回码
	Retcode int `json:"retcode"`
	// Data 数据
	Data any `json:"data"`
	// Message 消息
	Message string `json:"message"`
	// Echo 回显
	Echo string `json:"echo"`
	// Wording 描述
	Wording string `json:"wording"`
	// Stream 流
	Stream string `json:"stream"`
}

// Client WebSocket客户端
type Client struct {
	// conn WebSocket连接
	conn *websocket.Conn
	// serverURL WebSocket服务器URL
	serverURL string
	// token 认证令牌
	token string
}
