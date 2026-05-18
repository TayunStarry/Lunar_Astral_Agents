package websocket

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocket 升级器，用于将HTTP连接升级为WebSocket连接
var upgrader = websocket.Upgrader{
	// 读取缓冲区大小，用于接收客户端发送的消息
	ReadBufferSize: 1024,
	// 写入缓冲区大小，用于发送消息给客户端
	WriteBufferSize: 1024,
	// 检查请求来源是否在允许列表中
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WebSocket 客户端映射，键为客户端连接，值为true
var wsClients = make(map[*WSClient]bool)

// WebSocket 客户端互斥锁，用于保护客户端映射的并发访问
var wsMutex sync.RWMutex

// WebSocket 广播通道，用于发送消息给所有客户端
var wsBroadcaster = make(chan WSMessage, 256)
