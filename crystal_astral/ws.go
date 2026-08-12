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
			// 检查是否为 animation_list 消息，缓存动作定义供智能体查询
			cacheAnimationList(message)
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
			// 纯转发，不解析消息内容（哑中继原则）
			h.Broadcast <- message
		}
	}()

	wg.Wait()
}

// HandleEngineCommand 接收来自 lunar_astral 后端的引擎命令，直接转发到 StudioHub
// 实现 Agent → StudioHub → Engine 的直接通信路径，绕过前端
func HandleEngineCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	if StudioHubInstance == nil {
		http.Error(w, "StudioHub not initialized", http.StatusServiceUnavailable)
		return
	}
	StudioHubInstance.Broadcast <- body
	w.WriteHeader(http.StatusOK)
}

// ==== 动画列表缓存（从引擎 animation_list 消息中提取） ====

// animationListMessage 引擎广播的 animation_list 消息结构（仅解析需要的字段）
type animationListMessage struct {
	Type    string `json:"type"`
	Payload struct {
		ActionDefinitions []ActionDefinition `json:"actionDefinitions"`
	} `json:"payload"`
}

// cacheAnimationList 尝试从消息中提取 animation_list 的动作定义并缓存
// 非 animation_list 类型的消息会被静默忽略
func cacheAnimationList(msg []byte) {
	var parsed animationListMessage
	if err := json.Unmarshal(msg, &parsed); err != nil {
		return
	}
	if parsed.Type != "animation_list" {
		return
	}
	if len(parsed.Payload.ActionDefinitions) == 0 {
		return
	}

	animCache.Lock()
	defer animCache.Unlock()
	animCache.Actions = parsed.Payload.ActionDefinitions
	animCache.UpdatedAt = time.Now().UnixMilli()
	LoggerGeneral.Info("StudioHub", "动画列表缓存已更新: %d 个动作", len(animCache.Actions))
}

// HandleGetAnimations 返回当前缓存的可用动作列表
// 供 lunar_astral 后端查询，用于智能体动态构建工具定义
func HandleGetAnimations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	animCache.RLock()
	defer animCache.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(animCache)
}
