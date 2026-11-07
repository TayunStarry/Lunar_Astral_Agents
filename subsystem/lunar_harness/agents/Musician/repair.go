package Musician

// repair.go 向维修侧注册演奏者的只读诊断入口（最近一次演奏记录）。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly(agentID, roleLabel,
		roleLabel+"按次执行、不保留跨轮对话；可查看最近一次演奏记录（只读）",
		func(int) (any, error) { return exportContext(false) })
}
