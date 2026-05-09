package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 允许所有来源，生产环境应限制
		return true
	},
}

// WebSocketServer WebSocket 服务器
type WebSocketServer struct {
	interpreter *Interpreter
	clients     map[*websocket.Conn]bool
	clientsMu   sync.Mutex
	addr        string
	running     bool
}

// NewWebSocketServer 创建 WebSocket 服务器
func NewWebSocketServer(interpreter *Interpreter, addr string) *WebSocketServer {
	return &WebSocketServer{
		interpreter: interpreter,
		clients:     make(map[*websocket.Conn]bool),
		addr:        addr,
	}
}

// Start 启动 WebSocket 服务器
func (s *WebSocketServer) Start() error {
	if s.running {
		return fmt.Errorf("server already running")
	}

	http.HandleFunc("/ws", s.handleWebSocket)
	http.HandleFunc("/", s.handleIndex)

	s.running = true
	log.Printf("WebSocket server starting on %s", s.addr)
	go func() {
		if err := http.ListenAndServe(s.addr, nil); err != nil && s.running {
			log.Printf("WebSocket server error: %v", err)
		}
	}()

	// 启动消息广播 goroutine
	go s.broadcastMessages()

	return nil
}

// Stop 停止 WebSocket 服务器
func (s *WebSocketServer) Stop() {
	s.running = false
	s.clientsMu.Lock()
	for client := range s.clients {
		client.Close()
		delete(s.clients, client)
	}
	s.clientsMu.Unlock()
}

// handleIndex 处理根路径请求
func (s *WebSocketServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
    <title>LunarTick WebSocket Interface</title>
</head>
<body>
    <h1>LunarTick WebSocket Interface</h1>
    <p>Connect to /ws for WebSocket communication.</p>
</body>
</html>`))
}

// handleWebSocket 处理 WebSocket 连接
func (s *WebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	s.clientsMu.Lock()
	s.clients[conn] = true
	s.clientsMu.Unlock()

	log.Printf("New WebSocket client connected")

	// 接收消息
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			s.clientsMu.Lock()
			delete(s.clients, conn)
			s.clientsMu.Unlock()
			break
		}

		s.handleMessage(conn, message)
	}
}

// handleMessage 处理接收到的消息
func (s *WebSocketServer) handleMessage(conn *websocket.Conn, message []byte) {
	var req WSRequest
	if err := json.Unmarshal(message, &req); err != nil {
		s.sendError(conn, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	switch req.Type {
	case "inject":
		var injectData InjectData
		if err := json.Unmarshal(req.Data, &injectData); err != nil {
			s.sendError(conn, fmt.Sprintf("Invalid inject data: %v", err))
			return
		}
		s.interpreter.Inject(injectData.Lines)
		s.interpreter.Start()
		s.sendSuccess(conn, "Code injected and interpreter started")

	case "invoke":
		var invokeData InvokeData
		if err := json.Unmarshal(req.Data, &invokeData); err != nil {
			s.sendError(conn, fmt.Sprintf("Invalid invoke data: %v", err))
			return
		}
		s.interpreter.Invoke(invokeData.PointerName)
		s.sendSuccess(conn, fmt.Sprintf("Pointer %s invoked", invokeData.PointerName))

	case "start":
		s.interpreter.Start()
		s.sendSuccess(conn, "Interpreter started")

	case "stop":
		s.interpreter.Stop()
		s.sendSuccess(conn, "Interpreter stopped")

	default:
		s.sendError(conn, fmt.Sprintf("Unknown message type: %s", req.Type))
	}
}

// sendSuccess 发送成功消息
func (s *WebSocketServer) sendSuccess(conn *websocket.Conn, message string) {
	response := map[string]interface{}{
		"type":    "success",
		"message": message,
	}
	data, _ := json.Marshal(response)
	conn.WriteMessage(websocket.TextMessage, data)
}

// sendError 发送错误消息
func (s *WebSocketServer) sendError(conn *websocket.Conn, message string) {
	response := map[string]interface{}{
		"type":    "error",
		"message": message,
	}
	data, _ := json.Marshal(response)
	conn.WriteMessage(websocket.TextMessage, data)
}

// broadcastMessages 广播解释器的消息
func (s *WebSocketServer) broadcastMessages() {
	msgChan := s.interpreter.GetMessageChannel()
	for {
		select {
		case msg := <-msgChan:
			s.clientsMu.Lock()
			for client := range s.clients {
				data, _ := json.Marshal(msg)
				client.WriteMessage(websocket.TextMessage, data)
			}
			s.clientsMu.Unlock()
		}
	}
}
