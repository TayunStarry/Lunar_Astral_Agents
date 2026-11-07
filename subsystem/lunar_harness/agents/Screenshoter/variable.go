package Screenshoter

// variable.go 截图者的常量与运行时状态。

import (
	"sync"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// module 日志模块标识
	module = "Screenshoter"
	// agentID 上报、数据导出与智能修复共用的智能体标识
	agentID = "screenshoter"
	// roleLabel 实况与模型路由使用的角色名（保持既有契约）
	roleLabel = agentinfo.NameScreenshoter
	// promptPath 系统提示词路径
	promptPath = "prompts/screenshoterRole.md"
	// maxIterations 单次任务的推理轮上限
	maxIterations = 4
	// contextLimit 单次任务的上下文上限
	contextLimit = 40
	// retryAttempts 模型输出陷入重复时的总尝试次数（含首次）
	retryAttempts = 2
)

var (
	// stateMu 串行化截图任务并保护跨任务上下文：截图会切分多帧并推送，任务之间不应交叠
	stateMu sync.Mutex
	// state 跨任务保留的会话上下文（与原截图者角色一致）
	state runtimeState
)
