package adapters

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
	// ltpRemoteTools 最近一次从琉璃拉取的工具链（琉璃可能动态增删 LTPX 插件）
	ltpRemoteTools []LTPXRemoteToolDef
)

// ==== 消息推送全局变量 ====

// PushMessageFunc 消息推送函数，由 websocket 包初始化时设置
var PushMessageFunc func(msgType string, data interface{})

// GetAnimCacheFunc 获取动画缓存函数，由 websocket 包初始化时设置
// 用于 getAvailableActions 读取缓存的动画列表
var GetAnimCacheFunc func() interface{}

func init() {
	PushMessageFunc = func(msgType string, data interface{}) {
		LoggerGeneral.Error("LunarCore", "PushMessageFunc 未初始化, 消息类型: %s", msgType)
	}
	GetAnimCacheFunc = func() interface{} {
		return nil
	}
}

// UnreadContext 未处理的上下文消息
var UnreadContext = make([]PostMessage, 0)

// UnreadVideoUrl 未处理的视频URL
var UnreadVideoUrl = make([]string, 0)

// ==== 智能体 3D 位置全局变量 ====

// agentPosition 缓存的智能体最新3D位置（由前端遥测数据更新）
var agentPosition AgentPositionData

// agentPositionMutex 保护 agentPosition 的并发访问
var agentPositionMutex sync.RWMutex
