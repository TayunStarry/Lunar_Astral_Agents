package server

import (
	"LunarCore/adapters"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSClient struct {
	conn   *websocket.Conn
	send   chan []byte
	client *WSClient
}

type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

type WSResponse struct {
	Type    string      `json:"type"`
	Data    interface{} `json:"data,omitempty"`
	Context interface{} `json:"context,omitempty"`
	Image   interface{} `json:"image,omitempty"`
}

var (
	wsClients     = make(map[*WSClient]bool)
	wsMutex       sync.RWMutex
	wsBroadcaster = make(chan WSMessage, 256)
)

func WSHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}

	client := &WSClient{
		conn: conn,
		send: make(chan []byte, 256),
	}

	wsMutex.Lock()
	wsClients[client] = true
	wsMutex.Unlock()

	log.Printf("Lunar模块[WebSocket] -> 新客户端连接, 当前连接数: %d", len(wsClients))

	go client.writePump()
	go client.readPump()
}

func (c *WSClient) readPump() {
	defer func() {
		wsMutex.Lock()
		delete(wsClients, c)
		wsMutex.Unlock()
		c.conn.Close()
		log.Printf("Lunar模块[WebSocket] -> 客户端断开, 当前连接数: %d", len(wsClients))
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket读取错误: %v", err)
			}
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("WebSocket消息解析错误: %v", err)
			continue
		}

		wsBroadcaster <- msg
	}
}

func (c *WSClient) writePump() {
	defer c.conn.Close()

	for message := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Printf("WebSocket写入错误: %v", err)
			return
		}
	}
}

func SetupWebSocketHandler(mux *http.ServeMux) {
	adapters.PushMessageFunc = BroadcastMessage
	mux.HandleFunc("/ws", WSHandler)
}

func CloseWebSocketServer() {
	wsMutex.Lock()
	defer wsMutex.Unlock()
	for client := range wsClients {
		client.conn.Close()
		close(client.send)
		delete(wsClients, client)
	}
	log.Printf("Lunar模块[WebSocket] -> 已关闭所有连接")
}

func BroadcastMessage(msgType string, data interface{}) {
	wsMutex.RLock()
	defer wsMutex.RUnlock()

	response := WSResponse{
		Type: msgType,
		Data: data,
	}

	msgBytes, err := json.Marshal(response)
	if err != nil {
		log.Printf("WebSocket消息序列化失败: %v", err)
		return
	}

	for client := range wsClients {
		select {
		case client.send <- msgBytes:
		default:
			close(client.send)
			delete(wsClients, client)
		}
	}
}
