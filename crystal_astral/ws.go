package main

import (
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ==== WebSocket 升级器 ====

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		// 无 Origin 头：非浏览器客户端（月华等本地进程直连），放行
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		// 浏览器页面：仅放行同源请求（页面从本服务加载后连回自身 /ws）。
		// 旧实现恒真，任意网页都能跨站连上 /ws 驱动 LTP9 调试面（文件/数据库/指令/网络探针）。
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return u.Host == r.Host
	},
}

// ==== WebSocket 心跳参数 ====
// 此前读协程对每次读取都设固定 60s 读超时，页面闲置（不发消息）时连接会被超时断开，
// 导致前端每隔一会儿就重连。现改为服务端定期发 ping、浏览器自动回 pong，
// 收到 pong 即刷新读超时，闲置连接可长期维持。
const (
	wsWriteWait  = 10 * time.Second // 单次写入超时
	wsPongWait   = 90 * time.Second // 最长等待一次 pong（服务端每 wsPingPeriod 发一次 ping）
	wsPingPeriod = 30 * time.Second // 服务端发送 ping 的间隔
	wsMaxMessage = 10 * 1024 * 1024 // 单条消息上限（支持 base64 图片数据）
)

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

	// 写协程：将 Send 通道中的消息写入 WebSocket 连接，并定时发心跳 ping 维持连接
	go func() {
		defer wg.Done()
		defer conn.Close()
		ticker := time.NewTicker(wsPingPeriod)
		defer ticker.Stop()
		for {
			select {
			case message, ok := <-client.Send:
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if !ok {
					return // Send 通道已关闭（集线器注销），退出
				}
				if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
					return
				}
			case <-ticker.C: // 心跳：浏览器自动回 pong → 读协程刷新读超时
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// 读协程：从 WebSocket 连接读取消息并广播给所有客户端；收到 pong 刷新读超时
	go func() {
		defer wg.Done()
		defer func() {
			h.Unregister <- client
			conn.Close()
		}()
		conn.SetReadLimit(wsMaxMessage)
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(wsPongWait))
			return nil
		})
		conn.SetReadDeadline(time.Now().Add(wsPongWait))
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					LoggerGeneral.Error("StudioHub", "WebSocket 读取错误: %v", err)
				}
				break
			}
			// 控制帧（ping/pong）由 gorilla 内部处理，不会走到这里；仅数据帧继续广播
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
