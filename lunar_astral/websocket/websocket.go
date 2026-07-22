package websocket

import (
	"encoding/json"
	"logger"
	"lunar_astral/adapters"
	"lunar_astral/bridging/napcat"
	"net/http"

	"github.com/gorilla/websocket"
)

// shutdown 幂等地关闭客户端：关闭 done 信号、从连接池移除、关闭底层连接
// 任何一方（读泵/写泵）退出后均调用本方法，保证阻塞在 send 上的广播能通过 done 解除阻塞
func (c *WSClient) shutdown() {
	c.once.Do(func() {
		close(c.done)
		wsMutex.Lock()
		delete(wsClients, c)
		count := len(wsClients)
		wsMutex.Unlock()
		c.conn.Close()
		logger.SubInfo("LunarCore", "WebSocket", "客户端断开, 当前连接数: %d", count)
	})
}

// readPump 读泵：持续读取客户端上行消息并转发至广播通道
// 任意读取错误均触发 shutdown，由 done 信号通知广播方该客户端已不可用
func (c *WSClient) readPump() {
	defer c.shutdown()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.SubError("LunarCore", "WebSocket", "读取错误: %v", err)
			}
			return
		}

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			logger.SubError("LunarCore", "WebSocket", "消息解析错误: %v", err)
			continue
		}

		wsBroadcaster <- msg
	}
}

// writePump 写泵：消费 send 通道并写入底层连接
// 采用阻塞式 select（无 default 分支）：当 send 缓冲满时阻塞等待，绝不丢弃消息
// 同时监听 done 信号，客户端断开后及时退出，避免广播方死锁
func (c *WSClient) writePump() {
	defer c.shutdown()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				logger.SubError("LunarCore", "WebSocket", "写入错误: %v", err)
				return
			}
		case <-c.done:
			return
		}
	}
}

func WSHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.SubError("LunarCore", "WebSocket", "升级失败: %v", err)
		return
	}

	client := &WSClient{
		conn: conn,
		send: make(chan []byte, 256),
		done: make(chan struct{}),
	}

	wsMutex.Lock()
	wsClients[client] = true
	wsMutex.Unlock()

	logger.SubInfo("LunarCore", "WebSocket", "新客户端连接, 当前连接数: %d", len(wsClients))

	go client.writePump()
	go client.readPump()
}

func SetupWebSocketHandler(mux *http.ServeMux) {
	adapters.PushMessageFunc = BroadcastMessage
	mux.HandleFunc("/ws", WSHandler)
}

func CloseWebSocketServer() {
	// 拷贝客户端快照后逐一 shutdown，避免持锁期间触发 shutdown 的二次加锁
	wsMutex.Lock()
	clients := make([]*WSClient, 0, len(wsClients))
	for c := range wsClients {
		clients = append(clients, c)
	}
	wsMutex.Unlock()
	for _, c := range clients {
		c.shutdown()
	}
	logger.SubInfo("LunarCore", "WebSocket", "已关闭所有连接")
}

// BroadcastMessage 广播消息：拷贝当前客户端快照后逐一阻塞发送
// 阻塞式 select（无 default）保证所有在线客户端最终都能收到完整消息，允许任意延迟
// 慢客户端会阻塞广播直至其消费完成；客户端断开则通过 done 信号短路退出，避免死锁
// 注意：send 通道永不关闭，避免向已关闭通道发送引发 panic
func BroadcastMessage(msgType string, data any) {
	wsMutex.RLock()
	clients := make([]*WSClient, 0, len(wsClients))
	for c := range wsClients {
		clients = append(clients, c)
	}
	wsMutex.RUnlock()

	response := WSResponse{
		Type: msgType,
		Data: data,
	}

	msgBytes, err := json.Marshal(response)
	if err != nil {
		logger.SubError("LunarCore", "WebSocket", "消息序列化失败: %v", err)
		return
	}

	// 桥接适配器：将智能体响应转发到QQ群聊
	bridgeToQQ(response)

	for _, c := range clients {
		select {
		case c.send <- msgBytes:
		case <-c.done:
		}
	}
}

// bridgeToQQ 将智能体广播的响应消息转发到QQ群聊（如果桥接器已连接）
	func bridgeToQQ(response WSResponse) {
		if !napcat.IsBridgingEnabled() {
			return
		}

		switch response.Type {
		case "context":
			// Data 可能是任意结构体，通过 JSON 序列化/反序列化提取字段
			dataBytes, err := json.Marshal(response.Data)
			if err != nil {
				logger.SubError("LunarCore", "WebSocket", "桥接序列化数据失败: %v", err)
				return
			}
			var dataMap map[string]interface{}
			if err := json.Unmarshal(dataBytes, &dataMap); err != nil {
				logger.SubError("LunarCore", "WebSocket", "桥接解析数据失败: %v", err)
				return
			}
			msgType, _ := dataMap["type"].(string)
			content, _ := dataMap["content"].(string)
			if content != "" {
				go napcat.HandleAgentResponse(msgType, content)
			}
		case "image":
			// 图片消息：提取 base64 编码的图片列表并转发到QQ群
			dataBytes, err := json.Marshal(response.Data)
			if err != nil {
				logger.SubError("LunarCore", "WebSocket", "桥接序列化图片数据失败: %v", err)
				return
			}
			var dataMap map[string]interface{}
			if err := json.Unmarshal(dataBytes, &dataMap); err != nil {
				logger.SubError("LunarCore", "WebSocket", "桥接解析图片数据失败: %v", err)
				return
			}
			// 提取 images 数组
			if rawImages, ok := dataMap["images"].([]interface{}); ok {
				var images []string
				for _, img := range rawImages {
					if imgStr, ok := img.(string); ok {
						images = append(images, imgStr)
					}
				}
				if len(images) > 0 {
					go napcat.HandleAgentImageResponse(images)
				}
			}
		}
	}
