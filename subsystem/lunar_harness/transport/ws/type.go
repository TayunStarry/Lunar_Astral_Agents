package ws

import (
	"sync"

	"github.com/gorilla/websocket"
)

// WSClient 单个 WebSocket 客户端连接。
// Send 通道**永不关闭**（关闭会引发向已关闭通道发送的 panic），
// 断开一律经 Done 通道广播，写协程同时监听 Send 与 Done 退出。
type WSClient struct {
	// Conn 底层连接
	Conn *websocket.Conn
	// Send 下行消息通道
	Send chan []byte
	// Done 断开信号（shutdown 时关闭一次）
	Done chan struct{}
	// bus 是否为引擎总线订阅者（由连接 URL 的 bus 参数声明）
	// 仅订阅者接收原始字节总线帧；未订阅者只接收结构化消息
	bus bool
	// channel 总线频道名（由连接 URL 的 channel 参数声明，缺省为 DefaultBusChannel）。
	// 频道用于隔离彼此独立的引擎实例：总线帧只投递给**同频道**的订阅者。
	channel string
	// once 保证 Done 只关闭一次
	once sync.Once
}

// WSMessage 结构化上行消息（月华语义：{"type":…,"data":…}）
type WSMessage struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// WSResponse 结构化下行消息（月华语义）
type WSResponse struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Hub 统一 WebSocket 集线器。
// 同时承载两条互不相同的投递通道：
//   - BroadcastMessage 结构化下行（{"type":…,"data":…}）→ 投递给全部客户端
//   - Bus              引擎总线原始帧 → 仅投递给显式订阅的客户端（bus 参数）
//
// 两通道必须分离：引擎/工作室类模块之间经 /write/engine 高频互通原始帧，
// 而消息终端前端把无法识别的帧当作内容渲染，若混投会污染其消息记录。
type Hub struct {
	// Clients 在线客户端集合
	Clients map[*WSClient]bool
	// Bus 引擎总线原始帧通道（仅订阅者接收）
	Bus chan []byte
	// Inbound 入站消息通道（另经订阅者分发）
	Inbound chan []byte
	// Register 注册通道
	Register chan *WSClient
	// Unregister 注销通道
	Unregister chan *WSClient
	// clientsMu 保护 Clients 的并发快照读取
	clientsMu sync.RWMutex
	// subsMu 保护入站订阅者列表
	subsMu sync.RWMutex
	// subs 入站订阅者（每个订阅者独立缓冲与消费协程，互不阻塞）
	subs []*inboundSub
}

// inboundSub 入站订阅者
type inboundSub struct {
	// name 订阅者名称（日志用）
	name string
	// ch 独立缓冲通道
	ch chan []byte
	// fn 消费函数
	fn func([]byte)
}
