package adapters

import (
	"LunarSubsystem/LoggerGeneral"
	"context"
	"sync"

	"github.com/dop251/goja_nodejs/eventloop"
)

// ==== JavaScript 运行时全局变量 ====

// 全局变量，存储JavaScript运行时实例
var (
	runtimeMutex  sync.Mutex
	runtimeCtx    context.Context
	runtimeCancel context.CancelFunc
	runtime       *eventloop.EventLoop
)

// ==== LTPX 工具管理全局变量 ====

var (
	ltpMutex       sync.RWMutex
	loadedTools    = make(map[string]*LTPXToolInfo)
	pendingLoads   []*LTPXToolInfo
	pendingUnloads []string
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
