package engine

import (
	"LunarSubsystem/LoggerGeneral"
	"sync"
	"sync/atomic"
)

// ==== JavaScript 运行时上下文保护锁 ====

// runtimeMutex 保护 JavaScript 运行时上下文的创建与检查
// （运行时实例本身由通用模块 lunar_goja 持有）
var runtimeMutex sync.Mutex

// agentReady 标记 agentSystem.js 是否已成功加载（数据导出等宿主侧入口据此判断可用性）
var agentReady atomic.Bool

// 月华 JS 层日志路由标签（与 TuiSystem 管线路由及 container 包 console 绑定的约定一致）
const (
	lunarCoreModule    = "LunarCore"
	lunarJavaScriptSub = "JavaScript"
)

// ==== LTPX 工具链全局变量（进程内协调，无联络 URL） ====

var (
	// ltpRemoteMutex 保护聚合工具缓存的并发读写
	ltpRemoteMutex sync.RWMutex
	// ltpRemoteTool 最近一次同步到的聚合工具（恒为单一 use_the_program，随工具链变化更新）
	ltpRemoteTool *LTPXRemoteToolDef
	// ltpLoggedSubTools 上次打日志的子工具清单（清单未变化时不重复刷屏）
	ltpLoggedSubTools []string
)

// ==== 消息推送全局变量 ====

// PushMessageFunc 消息推送函数（结构化广播），由装配层注入为统一集线器的 BroadcastMessage
var PushMessageFunc func(msgType string, data any)

func init() {
	PushMessageFunc = func(msgType string, data any) {
		LoggerGeneral.Error("LunarCore", "PushMessageFunc 未初始化, 消息类型: %s", msgType)
	}
}

// UnreadContext 统一未读消息队列：文本消息与视频/音频URL内容项（MediaUrlContent）按到达时序混排
// 唯一数据源，从入队层面保证文本与媒体的相对顺序不会错乱；并发写入由 unreadMutex 保护
var UnreadContext = make([]PostMessage, 0)

// unreadMutex 保护统一未读队列的并发读写（napcat 回调与 HTTP handler 跨 goroutine 入队）
var unreadMutex sync.Mutex

// ==== 智能体 3D 位置全局变量 ====

// agentPosition 缓存的智能体最新3D位置（由前端遥测数据更新）
var agentPosition AgentPositionData

// agentPositionMutex 保护 agentPosition 的并发访问
var agentPositionMutex sync.RWMutex
