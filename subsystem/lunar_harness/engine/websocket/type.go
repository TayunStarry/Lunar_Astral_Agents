package websocket

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

// ClientMessage 客户端上行结构化命令。
//
// 只有**非总线订阅者**（bus=0，如资源库前端）的上行消息会进入这里：
// 总线参与者（引擎 iframe、工作室面板）的上行帧仍走 Hub.Bus 回流。
// Send 用于把回执定向发回发起命令的那一条连接，避免全频道广播。
type ClientMessage struct {
	// Channel 连接声明的频道（URL 的 channel 参数）
	Channel string
	// Data 上行消息原始字节（已拷贝，处理方无需关心读取缓冲复用）
	Data []byte
	// Send 向该连接投递一条下行消息；连接已断开时静默丢弃
	Send func([]byte)
}

// Hub 统一 WebSocket 集线器。
// 同时承载两条互不相同的投递通道：
//   - BroadcastMessage 结构化下行（{"type":…,"data":…}）→ 投递给全部客户端
//   - Bus              引擎总线原始帧 → 仅投递给同频道且显式订阅的客户端（bus 参数）
//
// 两通道必须分离：引擎/工作室类模块之间经 /ws 上行高频互通原始帧，
// 而消息终端前端把无法识别的帧当作内容渲染，若混投会污染其消息记录。
type Hub struct {
	// Clients 在线客户端集合
	Clients map[*WSClient]bool
	// Bus 引擎总线原始帧通道（带频道，仅订阅者接收）
	Bus chan BusFrame
	// Register 注册通道
	Register chan *WSClient
	// Unregister 注销通道
	Unregister chan *WSClient
	// clientsMu 保护 Clients 的并发快照读取
	clientsMu sync.RWMutex
}

// BusFrame 一条待投递的引擎总线帧（频道 + 原始字节）
type BusFrame struct {
	// Channel 发送方声明的频道（连接 URL 的 channel 参数）
	Channel string
	// Data 原始帧字节（发什么转什么，不做改写）
	Data []byte
}
