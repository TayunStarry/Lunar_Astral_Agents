package ws

import (
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// ==== 心跳与限制参数 ====
// 服务端定期发 ping、浏览器自动回 pong，收到 pong 即刷新读超时，
// 闲置连接可长期维持（取代「每次读取都设固定读超时」的旧实现）。
const (
	// WriteWait 单次写入超时
	WriteWait = 10 * time.Second
	// PongWait 最长等待一次 pong（服务端每 PingPeriod 发一次 ping）
	PongWait = 90 * time.Second
	// PingPeriod 服务端发送 ping 的间隔
	PingPeriod = 30 * time.Second
	// MaxMessage 单条消息上限（支持 base64 图片数据）
	MaxMessage = 10 * 1024 * 1024
	// SendBuffer 单客户端下行缓冲深度
	SendBuffer = 256
	// BroadcastBuffer 原始广播通道缓冲深度
	BroadcastBuffer = 256
	// InboundBuffer 入站通道缓冲深度
	InboundBuffer = 1024
	// SubscriberBuffer 单个入站订阅者缓冲深度
	SubscriberBuffer = 256
)

// Instance 全局集线器实例（装配期赋值）
var Instance *Hub

// BusParam 引擎总线订阅声明参数。
// 连接 URL 携带 bus=1 表示该连接是引擎/工作室总线参与者，接收原始帧；
// 缺省或 bus=0 表示仅接收结构化消息（月华消息终端、手机端即属此类）。
const BusParam = "bus"

// ChannelParam 总线频道声明参数（连接与 /write/engine 请求共用）。
//
// 为何需要频道：同一进程内可能存在多个彼此独立的引擎实例
// （月华消息终端内嵌的渲染引擎、琉璃 studio 的编辑器引擎），
// 它们各自的遥测/面板指令只能在**本实例的参与者之间**流动。
// 合并为单进程后若只按「是否订阅」投递，两个实例的帧会互相串扰
// （如 studio 信息栏同时收到两路遥测而高频刷动）。
const ChannelParam = "channel"

// DefaultBusChannel 未声明频道时的归属频道。
// 面向宿主的广播（ltpx_call、LTP9 调试回执、服务端主动 Emit）走此频道，
// 由琉璃桌面与蓝图页等订阅者接收。
const DefaultBusChannel = "lunar-engine-bus"

// Upgrader WebSocket 升级器：
//   - 无 Origin 头（非浏览器客户端，如本地直连进程）放行
//   - 浏览器页面仅放行同源请求，避免任意网页跨站连上 /ws 驱动调试面
var Upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return u.Host == r.Host
	},
}

// OnStructuredMessage 结构化消息的旁路回调（由装配层注入，例如转发到 QQ 桥接）。
// 仅对 BroadcastMessage 生效，原始字节广播不触发。
var OnStructuredMessage func(WSResponse)
