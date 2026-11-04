package CreatorPlanner

// repair.go 向拯救者注册创作规划者的修复能力。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable(AgentID, "创作规划者",
		"创作规划者按次执行、不保留跨轮对话；reset 清空最近一次推理记录",
		resetLastRun)
}
