package LifeMemorizer

// repair.go 向拯救者注册生活记忆整理者的修复能力。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable(AgentID, "生活记忆整理者",
		"生活记忆整理者按批执行、不保留跨批对话；reset 清空最近一次整理记录",
		resetLastRun,
		func(int) (any, error) { return exportContext(false) })
}
