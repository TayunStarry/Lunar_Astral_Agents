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
	"time"

	"LunarSubsystem/LoggerGeneral"
	"github.com/gorilla/websocket"
)

// wsPluginWriteWait 插件 WS 连接的单次写入超时：慢客户端不阻塞广播方与插件事件循环。
const wsPluginWriteWait = 10 * time.Second

// ltp9WSServer 单实例：负责宿主侧插件 WS 流的挂载/广播。
// paths: 路径 → 插件回调桥（onMessage，在插件事件循环内执行）。
var ltp9WSServer = &pluginWSServer{paths: map[string]func(string) string{}, clients: map[string]map[*wsConn]bool{}}

// wsConn 带写锁的连接包装：gorilla 连接不支持并发写，
// 宿主回显（读循环）与广播（Publish）必须经同一把锁串行，否则触发
// "concurrent write to websocket connection" panic（panic 发生在插件事件循环
// goroutine 内会直接击穿进程）。
type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// writeText 串行化写文本帧，带写超时。
func (c *wsConn) writeText(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(wsPluginWriteWait))
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

type pluginWSServer struct {
	mu       sync.RWMutex
	started  bool
	baseURL  string
	listener net.Listener
	paths    map[string]func(string) string
	clients  map[string]map[*wsConn]bool
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
	raw, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &wsConn{conn: raw}
	s.mu.Lock()
	s.registerClient(path, c)
	handler := s.paths[path]
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.removeClient(path, c)
		s.mu.Unlock()
		c.conn.Close()
	}()

	c.conn.SetReadLimit(10 * 1024 * 1024) // 10MB，兼容 base64 图片数据
	for {
		_, msg, rerr := c.conn.ReadMessage()
		if rerr != nil {
			break
		}
		if handler == nil {
			_ = c.writeText(msg)
			continue
		}
		_ = c.writeText([]byte(handler(string(msg))))
	}
}

func (s *pluginWSServer) registerClient(path string, c *wsConn) {
	set := s.clients[path]
	if set == nil {
		set = map[*wsConn]bool{}
		s.clients[path] = set
	}
	set[c] = true
}

func (s *pluginWSServer) removeClient(path string, c *wsConn) {
	if set := s.clients[path]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(s.clients, path)
		}
	}
}

// ltp9WSServe 挂载插件流路径并返回访问地址（engine.ws.expose 的宿主实现）。
// 后挂载者覆盖同路径时记一条告警：旧连接仍持有旧 handler，直到断开重连。
func ltp9WSServe(pluginID, path string, onMessage func(string) string) (string, error) {
	if err := ltp9WSServer.ensureStarted(); err != nil {
		return "", err
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	ltp9WSServer.mu.Lock()
	if _, exists := ltp9WSServer.paths[path]; exists {
		LoggerGeneral.Warn("StarLTP", "插件 %s 覆盖了已存在的 WS 流路径 %s（旧连接继续使用旧 handler）", pluginID, path)
	}
	ltp9WSServer.paths[path] = onMessage
	ltp9WSServer.mu.Unlock()
	addr := ltp9WSServer.baseURL + path
	LoggerGeneral.Info("StarLTP", "插件 %s 挂载 WS 流: %s", pluginID, addr)
	return addr, nil
}

// ltp9WSPublish 向指定路径的已连接客户端广播（engine.ws.publish 的宿主实现）。
// 客户端集合取快照后在锁外写：写操作经每连接写锁串行并带超时，
// 慢/失活客户端只拖慢自身，不会阻塞其他连接或持有读锁的发布方。
func ltp9WSPublish(pluginID, path, data string) error {
	if !ltp9WSServer.isStarted() {
		return fmt.Errorf("WS 服务未启动")
	}
	ltp9WSServer.mu.RLock()
	set := make([]*wsConn, 0, len(ltp9WSServer.clients[path]))
	for c := range ltp9WSServer.clients[path] {
		set = append(set, c)
	}
	ltp9WSServer.mu.RUnlock()

	var failed []*wsConn
	for _, c := range set {
		if err := c.writeText([]byte(data)); err != nil {
			failed = append(failed, c)
		}
	}
	if len(failed) > 0 {
		ltp9WSServer.mu.Lock()
		for _, c := range failed {
			ltp9WSServer.removeClientLocked(path, c)
		}
		ltp9WSServer.mu.Unlock()
	}
	return nil
}

// removeClientLocked 删除客户端并清理空路径集合（需已持有写锁）。
func (s *pluginWSServer) removeClientLocked(path string, c *wsConn) {
	if set := s.clients[path]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(s.clients, path)
		}
	}
	c.conn.Close()
}

func (s *pluginWSServer) isStarted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.started
}
