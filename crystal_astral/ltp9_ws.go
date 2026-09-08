package main

// ==== LTP9 插件 WebSocket 服务端桥（engine.ws 真实传输） ====
// 独立监听回环地址的随机端口，为插件挂载流路径（Serve）并广播（Publish）。
// 不经主 /ws 集线器，避免把插件流二次广播给全部客户端。

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"LunarSubsystem/LoggerGeneral"
	"github.com/gorilla/websocket"
)

// ltp9WSServer 单实例：负责宿主侧插件 WS 流的挂载/广播。
// paths: 路径 → 插件回调桥（onMessage，在插件事件循环内执行）。
var ltp9WSServer = &pluginWSServer{paths: map[string]func(string) string{}, clients: map[string]map[*websocket.Conn]bool{}}

type pluginWSServer struct {
	mu       sync.RWMutex
	started  bool
	baseURL  string
	listener net.Listener
	paths    map[string]func(string) string
	clients  map[string]map[*websocket.Conn]bool
}

// ensureStarted 惰性启动独立 WS 服务（仅启动一次）。
func (s *pluginWSServer) ensureStarted() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.listener = ln
	s.baseURL = fmt.Sprintf("ws://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)
	s.started = true
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", s.handleConn)
		srv := &http.Server{Handler: mux}
		if serr := srv.Serve(ln); serr != nil && !strings.Contains(serr.Error(), "closed") {
			LoggerGeneral.Error("StarLTP", "LTP9 插件 WS 服务异常: %v", serr)
		}
	}()
	LoggerGeneral.Info("StarLTP", "LTP9 插件 WebSocket 服务已启动: %s", s.baseURL)
	return nil
}

// handleConn 处理一次 WS 连接：按 path 找到插件回调桥，逐条回显/转发。
func (s *pluginWSServer) handleConn(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.registerClient(path, conn)
	handler := s.paths[path]
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.removeClient(path, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	conn.SetReadLimit(10 * 1024 * 1024) // 10MB，兼容 base64 图片数据
	for {
		_, msg, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		if handler == nil {
			_ = conn.WriteMessage(websocket.TextMessage, msg)
			continue
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(handler(string(msg))))
	}
}

func (s *pluginWSServer) registerClient(path string, conn *websocket.Conn) {
	set := s.clients[path]
	if set == nil {
		set = map[*websocket.Conn]bool{}
		s.clients[path] = set
	}
	set[conn] = true
}

func (s *pluginWSServer) removeClient(path string, conn *websocket.Conn) {
	if set := s.clients[path]; set != nil {
		delete(set, conn)
		if len(set) == 0 {
			delete(s.clients, path)
		}
	}
}

// ltp9WSServe 挂载插件流路径并返回访问地址（engine.ws.expose 的宿主实现）。
func ltp9WSServe(pluginID, path string, onMessage func(string) string) (string, error) {
	if err := ltp9WSServer.ensureStarted(); err != nil {
		return "", err
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	ltp9WSServer.mu.Lock()
	ltp9WSServer.paths[path] = onMessage
	ltp9WSServer.mu.Unlock()
	addr := ltp9WSServer.baseURL + path
	LoggerGeneral.Info("StarLTP", "插件 %s 挂载 WS 流: %s", pluginID, addr)
	return addr, nil
}

// ltp9WSPublish 向指定路径的已连接客户端广播（engine.ws.publish 的宿主实现）。
func ltp9WSPublish(pluginID, path, data string) error {
	if !ltp9WSServer.isStarted() {
		return fmt.Errorf("WS 服务未启动")
	}
	ltp9WSServer.mu.RLock()
	defer ltp9WSServer.mu.RUnlock()
	if set := ltp9WSServer.clients[path]; set != nil {
		for c := range set {
			_ = c.WriteMessage(websocket.TextMessage, []byte(data))
		}
	}
	return nil
}

func (s *pluginWSServer) isStarted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.started
}