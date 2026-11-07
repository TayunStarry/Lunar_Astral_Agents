package SearchPlanner

// repair.go 向维修侧注册搜索者-规划的只读诊断入口（最近一次规划记录）。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly(AgentID, modelRole,
		modelRole+"的最近一次结构化决策（只读）",
		func(int) (any, error) { return exportContext(false) })
}
