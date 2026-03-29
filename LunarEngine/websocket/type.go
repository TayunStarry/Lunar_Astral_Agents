package websocket

import (
	agent "Lunar-Astral-Agents/reasoning" // 引入推理模块，用于获取模型路径等配置
	"sync"                                // 用于同步操作，如互斥锁
	"time"                                // 用于时间操作，如时间戳

	"golang.org/x/net/websocket" // 用于WebSocket连接
)

// 服务器状态
type ServerState struct {
	// 互斥锁，用于保护请求映射的并发访问
	mutex sync.RWMutex
	// 请求映射，键为请求ID，值为请求上下文
	requests map[string]*agent.RequestContext
	// 服务器配置
	config ServerConfig
	// WebSocket连接
	websocketConn *websocket.Conn
	// WebSocket连接互斥锁
	wsMutex sync.Mutex
}

// 服务器配置
type ServerConfig struct {
	// 服务器监听端口
	Port string
	// 允许的CORS来源
	CORSAllowedOrigins []string
	// 请求超时时间
	RequestTimeout time.Duration
	// 最大并发请求数
	MaxRequests int
	// 过期请求清理间隔
	CleanupInterval time.Duration
}
