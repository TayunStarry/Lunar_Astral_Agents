package main

import (
	"encoding/json"
	"fmt"
	"logger"
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

type WebSocketServer struct {
	interpreter *Interpreter
	clients     map[*websocket.Conn]bool
	clientsMu   sync.Mutex
	addr        string
	running     bool
}

func NewWebSocketServer(interpreter *Interpreter, addr string) *WebSocketServer {
	return &WebSocketServer{
		interpreter: interpreter,
		clients:     make(map[*websocket.Conn]bool),
		addr:        addr,
	}
}

func (s *WebSocketServer) Start() error {
	if s.running {
		return fmt.Errorf("server already running")
	}

	http.HandleFunc("/ws", s.handleWebSocket)
	http.HandleFunc("/", s.handleIndex)

	s.running = true
	logger.Info("LunarTick", "WebSocket server starting on %s", s.addr)
	go func() {
		if err := http.ListenAndServe(s.addr, nil); err != nil && s.running {
			logger.Error("LunarTick", "WebSocket server error: %v", err)
		}
	}()
	go func() {
		if err := http.ListenAndServe(s.addr, nil); err != nil && s.running {
			logger.Error("LunarTick", "WebSocket server error: %v", err)
		}
	}()

	go s.broadcastMessages()

	return nil
}

func (s *WebSocketServer) Stop() {
	s.running = false
	s.clientsMu.Lock()
	for client := range s.clients {
		client.Close()
		delete(s.clients, client)
	}
	s.clientsMu.Unlock()
}

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

func (s *WebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("LunarTick", "WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	s.clientsMu.Lock()
	s.clients[conn] = true
	s.clientsMu.Unlock()

	logger.Info("LunarTick", "New WebSocket client connected")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			logger.Error("LunarTick", "WebSocket read error: %v", err)
			s.clientsMu.Lock()
			delete(s.clients, conn)
			s.clientsMu.Unlock()
			break
		}

		s.handleMessage(conn, message)
	}
}

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
		s.sendError(conn, fmt.Sprintf("Unknown message type: %s", req.Type))
	}
}

func (s *WebSocketServer) sendSuccess(conn *websocket.Conn, message string) {
	response := map[string]interface{}{
		"type":    "success",
		"message": message,
	}
	data, _ := json.Marshal(response)
	conn.WriteMessage(websocket.TextMessage, data)
}

func (s *WebSocketServer) sendError(conn *websocket.Conn, message string) {
	response := map[string]interface{}{
		"type":    "error",
		"message": message,
	}
	data, _ := json.Marshal(response)
	conn.WriteMessage(websocket.TextMessage, data)
}

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