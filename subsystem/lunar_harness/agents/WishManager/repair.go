package WishManager

// repair.go 向维修侧注册愿望管理的只读诊断入口（最近一次执行记录）。

import "LunarSubsystem/LunarHarness/protocol/agentrepair"

func init() {
	agentrepair.RegisterReadonly(AgentID, roleLabel,
		"愿望管理按次执行、不保留跨轮对话；可查看最近一次愿望操作记录与当前愿望清单（只读）",
		func(int) (any, error) { return exportContext(false) })
}
