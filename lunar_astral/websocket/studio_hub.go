package websocket

import (
	"encoding/json"
	"LunarSubsystem/general_logger"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ==== StudioHub：引擎通信中枢（/ws/studio 端点） ====
// 独立于 /ws 端点，专供引擎（simple_physics）通过 WsBridge 连接。
// 消息广播给所有已连接的引擎客户端，同时从 animation_list 消息中缓存动作定义。

// upgraderStudio 引擎 WebSocket 升级器
var upgraderStudio = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// NewStudioHub 创建引擎集线器实例
func NewStudioHub() *StudioHub {
	return &StudioHub{
		Clients:    make(map[*StudioClient]bool),
		Broadcast:  make(chan []byte, 256),
		Register:   make(chan *StudioClient),
		Unregister: make(chan *StudioClient),
	}
}

// StartStudioHub 启动引擎集线器并注册 /ws/studio 端点
func StartStudioHub(mux *http.ServeMux) {
	StudioHubInstance = NewStudioHub()
	go StudioHubInstance.Run()
	mux.HandleFunc("/ws/studio", StudioHubInstance.HandleWebSocket)
	logger.SubInfo("LunarCore", "StudioHub", "引擎通信中枢已启动: /ws/studio")
}

// Run 启动集线器主循环
func (h *StudioHub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Clients[client] = true
			logger.SubInfo("LunarCore", "StudioHub", "引擎客户端已连接，当前连接数: %d", len(h.Clients))

		case client := <-h.Unregister:
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
				logger.SubInfo("LunarCore", "StudioHub", "引擎客户端已断开，当前连接数: %d", len(h.Clients))
			}

		case message := <-h.Broadcast:
			// 从消息中提取动画列表并缓存
			cacheAnimationList(message)
			for client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.Clients, client)
				}
			}
		}
	}
}

// HandleWebSocket 处理引擎 WebSocket 升级请求
func (h *StudioHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgraderStudio.Upgrade(w, r, nil)
	if err != nil {
		logger.SubError("LunarCore", "StudioHub", "WebSocket 升级失败: %v", err)
		return
	}

	client := &StudioClient{
		Conn: conn,
		Send: make(chan []byte, 256),
	}
	h.Register <- client

	var wg sync.WaitGroup
	wg.Add(2)

	// 写协程
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

	// 读协程
	go func() {
		defer wg.Done()
		defer func() {
			h.Unregister <- client
			conn.Close()
		}()
		conn.SetReadLimit(10 * 1024 * 1024)
		for {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					logger.SubError("LunarCore", "StudioHub", "WebSocket 读取错误: %v", err)
				}
				break
			}
			h.Broadcast <- message
		}
	}()

	wg.Wait()
}

// StudioBroadcast 向所有引擎客户端广播消息（供外部模块调用）
func StudioBroadcast(msg []byte) {
	if StudioHubInstance != nil {
		StudioHubInstance.Broadcast <- msg
	}
}

// ==== 动画列表缓存 ====

// animationListMessage 引擎广播的 animation_list 消息结构
type animationListMessage struct {
	Type    string `json:"type"`
	Payload struct {
		ActionDefinitions []ActionDefinition `json:"actionDefinitions"`
	} `json:"payload"`
}

// cacheAnimationList 尝试从消息中提取 animation_list 并缓存
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
	logger.SubInfo("LunarCore", "StudioHub", "动画列表缓存已更新: %d 个动作", len(animCache.Actions))
}

// HandleGetAnimations 返回当前缓存的可用动作列表
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