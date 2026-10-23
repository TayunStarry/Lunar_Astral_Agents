package ws

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"github.com/gorilla/websocket"
)

// NewHub 创建统一集线器实例
func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[*WSClient]bool),
		Bus:        make(chan []byte, BroadcastBuffer),
		Inbound:    make(chan []byte, InboundBuffer),
		Register:   make(chan *WSClient),
		Unregister: make(chan *WSClient),
	}
}

// Run 启动集线器主循环（在 goroutine 中运行）。
// 客户端注册/注销/原始广播/入站分发均在本循环内串行处理，保证顺序性。
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.clientsMu.Lock()
			h.Clients[client] = true
			count := len(h.Clients)
			h.clientsMu.Unlock()
			logger.Info("StudioHub", "客户端已连接，当前连接数: %d", count)

		case client := <-h.Unregister:
			// 幂等注销：慢客户端可能已被广播路径先行摘除
			h.clientsMu.Lock()
			_, exists := h.Clients[client]
			if exists {
				delete(h.Clients, client)
			}
			count := len(h.Clients)
			h.clientsMu.Unlock()
			if exists {
				client.shutdown()
				logger.Info("StudioHub", "客户端已断开，当前连接数: %d", count)
			}

		case message := <-h.Bus:
			h.BroadcastBus(DefaultBusChannel, message)

		case message := <-h.Inbound:
			h.dispatchInbound(message)
		}
	}
}

// ==================== 下行：引擎总线原始帧（仅订阅者） ====================

// BroadcastBus 向**已订阅引擎总线且频道匹配**的客户端投递原始帧；
// 未订阅者（消息终端等）与其它频道的订阅者均不收。
// 缓冲已满的订阅者视为慢客户端，直接被摘除（不阻塞其它客户端）。
//
// 为何必须按「订阅 + 频道」投递：/write/engine 承载引擎与工作室模块之间的高频原始帧
// （动画列表、位置遥测、面板指令等）。消息终端前端对无法识别的帧会以 JSON 文本兜底渲染，
// 若把总线帧投给它会持续往聊天记录里灌内容；而同一进程内的多个独立引擎实例
// （月华内嵌渲染引擎、琉璃 studio 编辑器引擎）若不按频道隔离，各自的遥测会互相串扰。
func (h *Hub) BroadcastBus(channel string, message []byte) {
	if channel == "" {
		channel = DefaultBusChannel
	}
	var slow []*WSClient

	h.clientsMu.RLock()
	for client := range h.Clients {
		if !client.bus || client.channel != channel {
			continue
		}
		select {
		case client.Send <- message:
		default:
			slow = append(slow, client)
		}
	}
	h.clientsMu.RUnlock()

	if len(slow) == 0 {
		return
	}
	// 摘除慢客户端：只做 map 操作与信号关闭，不向 Run 发送（当前可能正处于 Run 内）
	h.clientsMu.Lock()
	for _, client := range slow {
		if _, ok := h.Clients[client]; ok {
			delete(h.Clients, client)
			client.shutdown()
		}
	}
	h.clientsMu.Unlock()
	logger.Warn("StudioHub", "已摘除 %d 个慢总线客户端，当前连接数: %d", len(slow), len(h.Clients))
}

// ==================== 下行：结构化广播（月华语义） ====================

// BroadcastMessage 以 {"type":…,"data":…} 结构广播给所有客户端。
// 阻塞式投递（无 default 分支）：缓冲满时等待消费，绝不丢弃消息；
// 客户端断开经 Done 短路退出，避免死锁。
func (h *Hub) BroadcastMessage(msgType string, data any) {
	response := WSResponse{Type: msgType, Data: data}

	// 旁路回调（QQ 桥接等），在序列化前触发，保持原月华顺序
	if OnStructuredMessage != nil {
		OnStructuredMessage(response)
	}

	msgBytes, err := json.Marshal(response)
	if err != nil {
		logger.SubError("StudioHub", "WebSocket", "消息序列化失败: %v", err)
		return
	}

	h.clientsMu.RLock()
	clients := make([]*WSClient, 0, len(h.Clients))
	for c := range h.Clients {
		clients = append(clients, c)
	}
	h.clientsMu.RUnlock()

	for _, c := range clients {
		select {
		case c.Send <- msgBytes:
		case <-c.Done:
		}
	}
}

// ==================== 上行：入站分发 ====================

// SubscribeInbound 注册入站消息订阅者。
// 每个订阅者持有独立缓冲通道与消费协程：互不阻塞，也不阻塞集线器主循环；
// 通道满时丢弃该条入站消息（与旧的非阻塞入站语义一致）。
func (h *Hub) SubscribeInbound(name string, fn func([]byte)) {
	sub := &inboundSub{
		name: name,
		ch:   make(chan []byte, SubscriberBuffer),
		fn:   fn,
	}
	h.subsMu.Lock()
	h.subs = append(h.subs, sub)
	h.subsMu.Unlock()

	go func() {
		for msg := range sub.ch {
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("StudioHub", "入站订阅者 [%s] 处理异常: %v", sub.name, r)
					}
				}()
				sub.fn(msg)
			}()
		}
	}()
}

// dispatchInbound 把入站消息扇出到所有订阅者（非阻塞）
func (h *Hub) dispatchInbound(message []byte) {
	h.subsMu.RLock()
	subs := make([]*inboundSub, len(h.subs))
	copy(subs, h.subs)
	h.subsMu.RUnlock()

	for _, sub := range subs {
		select {
		case sub.ch <- message:
		default:
		}
	}
}

// ==================== 连接生命周期 ====================

// HandleWebSocket 处理 WebSocket 升级请求，升级后启动读写协程
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("StudioHub", "WebSocket 升级失败: %v", err)
		return
	}

	client := &WSClient{
		Conn:    conn,
		Send:    make(chan []byte, SendBuffer),
		Done:    make(chan struct{}),
		bus:     subscribeBus(r),
		channel: requestChannel(r),
	}
	h.Register <- client

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); client.writePump() }()
	go func() { defer wg.Done(); client.readPump(h) }()
	wg.Wait()
}

// subscribeBus 解析连接的引擎总线订阅声明。
// 默认**不订阅**：只有显式声明 bus=1/true/yes/on 的连接才接收原始总线帧。
// 默认不订阅是安全取向——消息终端前端把无法识别的帧当内容渲染，
// 必须由总线参与者（引擎 iframe、工作室面板、琉璃桌面与蓝图页）自行声明订阅。
func subscribeBus(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get(BusParam))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// RequestChannel 导出频道解析（供 /write/engine 等宿主端点复用，缺省为 DefaultBusChannel）
func RequestChannel(r *http.Request) string {
	return requestChannel(r)
}

// requestChannel 解析总线频道声明（连接与 /write/engine 共用），缺省为 DefaultBusChannel
func requestChannel(r *http.Request) string {
	if channel := strings.TrimSpace(r.URL.Query().Get(ChannelParam)); channel != "" {
		return channel
	}
	return DefaultBusChannel
}

// shutdown 幂等关闭客户端：关闭 Done 信号并断开底层连接
func (c *WSClient) shutdown() {
	c.once.Do(func() {
		close(c.Done)
		c.Conn.Close()
	})
}

// writePump 写协程：消费 Send 通道写入连接，并定时发心跳 ping
func (c *WSClient) writePump() {
	ticker := time.NewTicker(PingPeriod)
	defer ticker.Stop()
	for {
		select {
		case message := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(WriteWait))
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				c.shutdown()
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(WriteWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.shutdown()
				return
			}
		case <-c.Done:
			return
		}
	}
}

// readPump 读协程：读取上行消息 → 原始广播 + 入站分发；收到 pong 刷新读超时
func (c *WSClient) readPump(h *Hub) {
	defer func() {
		h.Unregister <- c
		c.shutdown()
	}()

	c.Conn.SetReadLimit(MaxMessage)
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(PongWait))
		return nil
	})
	c.Conn.SetReadDeadline(time.Now().Add(PongWait))

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.Error("StudioHub", "WebSocket 读取错误: %v", err)
			}
			return
		}
		// 控制帧（ping/pong）由 gorilla 内部处理，不会走到这里；仅数据帧继续进总线与入站
		h.Bus <- message
		select {
		case h.Inbound <- message:
		default:
		}
	}
}

// ClientCount 当前在线客户端数
func (h *Hub) ClientCount() int {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()
	return len(h.Clients)
}

// BusCount 当前引擎总线订阅者数量
func (h *Hub) BusCount() int {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()
	count := 0
	for client := range h.Clients {
		if client.bus {
			count++
		}
	}
	return count
}

// ChannelBusCount 指定频道上的引擎总线订阅者数量。
// 琉璃桌面以 /ws?bus=1（缺省频道）接入，月华侧引擎 iframe 已显式 bus=0 退订，
// 因此「缺省频道订阅数 > 0」可作为琉璃前端在线的判据（LTPX 前端守护使用）。
func (h *Hub) ChannelBusCount(channel string) int {
	if channel == "" {
		channel = DefaultBusChannel
	}
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()
	count := 0
	for client := range h.Clients {
		if client.bus && client.channel == channel {
			count++
		}
	}
	return count
}

// ChannelClientCount 指定频道上的连接数（不区分是否订阅引擎总线）。
// 月华消息终端以 /ws?channel=message-terminal 接入且不订阅总线，
// 故其在线判定须按频道连接数统计（ChannelBusCount 会把这类连接计为 0）。
func (h *Hub) ChannelClientCount(channel string) int {
	if channel == "" {
		channel = DefaultBusChannel
	}
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()
	count := 0
	for client := range h.Clients {
		if client.channel == channel {
			count++
		}
	}
	return count
}

// CloseAll 关闭所有在线连接（进程退出前调用）
func (h *Hub) CloseAll() {
	h.clientsMu.Lock()
	clients := make([]*WSClient, 0, len(h.Clients))
	for c := range h.Clients {
		clients = append(clients, c)
	}
	h.clientsMu.Unlock()

	for _, c := range clients {
		c.shutdown()
	}
	logger.SubInfo("StudioHub", "WebSocket", "已关闭所有连接")
}
