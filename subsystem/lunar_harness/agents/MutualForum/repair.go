package MutualForum

// repair.go 向维修侧注册互助论坛智能体的只读诊断入口。
//
// 跨轮上下文由 chain/forum 维护并单独注册；这里只导出最近一次模型轮次诊断信息。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("mutual_forum_agent", roleLabel,
		"互助论坛智能体最近一次模型轮次诊断信息（只读）",
		func(int) (any, error) { return exportContext(false) })
}
