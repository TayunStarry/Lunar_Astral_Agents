package PlanManager

// repair.go 向拯救者注册计划管理的修复能力（清空最近一次执行记录）。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable(agentID, "计划管理",
		"计划管理按次执行、不保留跨轮对话；reset 清空最近一次计划操作记录",
		resetRun,
		func(int) (any, error) { return exportContext(false) })
}
