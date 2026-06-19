package adapters

import (
	"context"
	"logger"
	"sync"

	"websearch"

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

func init() {
	PushMessageFunc = func(msgType string, data interface{}) {
		logger.Error("LunarCore", "PushMessageFunc 未初始化, 消息类型: %s", msgType)
	}
}

// UnreadContext 未处理的上下文消息
var UnreadContext = make([]PostMessage, 0)

// UnreadVideoUrl 未处理的视频URL
var UnreadVideoUrl = make([]string, 0)

// ==== 网络检索全局变量 ====

// webSearchSystem 网络检索子系统实例
var webSearchSystem *websearch.System
