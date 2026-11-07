package Responder

// repair.go 向维修侧注册通用应答的只读诊断入口。
//
// 通用应答按请求即时推理、不保存会话上下文；可观察的是排队状态。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("responder", roleLabel,
		"通用应答的排队状态（不保留上下文，只读）",
		func(int) (any, error) { return exportContext(false) })
}
