package SearchSummarizer

// repair.go 向维修侧注册搜索者-摘要的只读诊断入口（最近一次摘要记录）。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly(AgentID, modelRole,
		modelRole+"的最近一次摘要记录（只读）",
		func(int) (any, error) { return exportContext(false) })
}
