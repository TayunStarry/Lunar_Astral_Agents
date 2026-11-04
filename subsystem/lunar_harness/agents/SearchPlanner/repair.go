package SearchPlanner

// repair.go 向拯救者注册搜索者-规划的修复能力（清空最近一次规划记录）。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable(AgentID, "搜索者-规划",
		"搜索者-规划的最近一次结构化决策；reset 清空该记录",
		resetPlannerState)
}

// resetPlannerState 清空最近一次规划记录。
func resetPlannerState() string {
	lastPlanMu.Lock()
	had := lastPlanItem != nil
	lastPlanItem = nil
	lastPlanMu.Unlock()
	if had {
		logger.Info("CrystalAstral", "搜索者-规划最近规划记录已清空")
		return "已清空搜索者-规划的最近规划记录"
	}
	return "搜索者-规划当前没有规划记录"
}
