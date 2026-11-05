package Responder

// repair.go 向拯救者注册通用应答的修复能力。
//
// 通用应答按请求即时推理、不保存会话上下文；可修的是排队状态：
// 外部请求卡住时，reset 会丢弃尚未开始的排队请求。

import (
	"fmt"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("responder", "通用应答",
		"通用应答的排队状态（不保留上下文）；reset 丢弃尚未开始的排队请求",
		resetQueueState,
		func(int) (any, error) { return exportContext(false) })
}

// resetQueueState 清空待处理队列，正在执行的请求不受影响。
func resetQueueState() string {
	dropped := ResetQueue()
	logger.Warn(roleLabel, "[拯救] 已丢弃 %d 条排队中的应答请求", dropped)
	return fmt.Sprintf("已丢弃 %d 条排队中的应答请求", dropped)
}
