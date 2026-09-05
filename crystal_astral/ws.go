package main

import (
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ==== WebSocket 升级器 ====

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源（本地开发环境）
	},
}

// ==== 工作室集线器 ====

// NewStudioHub 创建工作室集线器实例
func NewStudioHub() *StudioHub {
	return &StudioHub{
		Clients:    make(map[*StudioClient]bool),
		Broadcast:  make(chan []byte, 256),
		Inbound:    make(chan []byte, 1024),
		Register:   make(chan *StudioClient),
		Unregister: make(chan *StudioClient),
	}
}

// Run 启动集线器主循环（在 goroutine 中运行）
// 处理客户端注册、注销和消息广播
func (h *StudioHub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Clients[client] = true
			LoggerGeneral.Info("StudioHub", "客户端已连接，当前连接数: %d", len(h.Clients))

		case client := <-h.Unregister:
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
				LoggerGeneral.Info("StudioHub", "客户端已断开，当前连接数: %d", len(h.Clients))
			}

		case message := <-h.Broadcast:
			for client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					// 客户端发送缓冲区已满，视为慢客户端，断开连接
					close(client.Send)
					delete(h.Clients, client)
				}
			}
		}
	}
}

// HandleWebSocket 处理 WebSocket 升级请求
// 升级 HTTP 连接为 WebSocket，启动读写协程
func (h *StudioHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		LoggerGeneral.Error("StudioHub", "WebSocket 升级失败: %v", err)
		return
	}

	client := &StudioClient{
		Conn: conn,
		Send: make(chan []byte, 256),
	}
	h.Register <- client

	// 启动读写协程
	var wg sync.WaitGroup
	wg.Add(2)

	// 写协程：将 Send 通道中的消息写入 WebSocket 连接
	go func() {
		defer wg.Done()
		defer conn.Close()
		for message := range client.Send {
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		}
	}()

	// 读协程：从 WebSocket 连接读取消息并广播给所有客户端
	go func() {
		defer wg.Done()
		defer func() {
			h.Unregister <- client
			conn.Close()
		}()
		conn.SetReadLimit(10 * 1024 * 1024) // 10MB 最大消息大小（支持 base64 图片数据）
		for {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					LoggerGeneral.Error("StudioHub", "WebSocket 读取错误: %v", err)
				}
				break
			}
			// 转发给所有客户端 + 非阻塞送入引擎入站通道（LTP3 引擎等内部消费者）
			h.Broadcast <- message
			select {
			case h.Inbound <- message:
			default:
			}
		}
	}()

	wg.Wait()
}

// StudioEngineHandler 接收引擎/工作室消息（POST /write/engine，格式与 /write/message 同构）
// 职责：将原始消息广播给所有本地 /ws 客户端（模块间互通，客户端自行过滤）
// 注：crystal_astral 与 lunar_astral 为独立实现，此处不做跨后端转发
func StudioEngineHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if StudioHubInstance != nil {
		StudioHubInstance.Broadcast <- body
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"length":  len(body),
	})
}
