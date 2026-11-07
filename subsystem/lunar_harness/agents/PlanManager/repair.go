package PlanManager

// repair.go 向维修侧注册计划管理的只读诊断入口（最近一次执行记录）。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly(agentID, roleLabel,
		"计划管理按次执行、不保留跨轮对话；可查看最近一次计划操作记录（只读）",
		func(int) (any, error) { return exportContext(false) })
}
