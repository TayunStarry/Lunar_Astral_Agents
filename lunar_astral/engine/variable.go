package engine

import (
	"LunarSubsystem/LoggerGeneral"
	"sync"
)

// ==== JavaScript 运行时上下文保护锁 ====

// runtimeMutex 保护 JavaScript 运行时上下文的创建与检查
// （运行时实例本身由通用模块 lunar_goja 持有）
var runtimeMutex sync.Mutex

// ==== LTPX 远程（琉璃）工具链全局变量 ====

var (
	// ltpRemoteMutex 保护琉璃联络 URL 与工具链的并发读写
	ltpRemoteMutex sync.RWMutex
	// ltpRemoteURL 琉璃的唯一联络 URL（兼容多开：以最新注册的琉璃进程为准，只记录一个）
	ltpRemoteURL string
	// ltpRemoteTool 最近一次从琉璃拉取的聚合工具（v5 起恒为单一 use_program，琉璃动态增删 LTPX 插件时随心跳更新）
	ltpRemoteTool *LTPXRemoteToolDef
)

// ==== 消息推送全局变量 ====

// PushMessageFunc 消息推送函数，由 websocket 包初始化时设置
var PushMessageFunc func(msgType string, data any)

// GetAnimCacheFunc 获取动画缓存函数，由 websocket 包初始化时设置
// 用于 getAvailableActions 读取缓存的动画列表
var GetAnimCacheFunc func() any

func init() {
	PushMessageFunc = func(msgType string, data any) {
		LoggerGeneral.Error("LunarCore", "PushMessageFunc 未初始化, 消息类型: %s", msgType)
	}
	GetAnimCacheFunc = func() any {
		return nil
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
