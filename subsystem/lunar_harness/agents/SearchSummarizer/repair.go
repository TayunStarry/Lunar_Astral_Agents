package SearchSummarizer

// repair.go 向拯救者注册搜索者-摘要的修复能力（清空最近一次摘要记录）。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable(AgentID, "搜索者-摘要",
		"搜索者-摘要的最近一次摘要记录；reset 清空该记录",
		resetSummarizerState,
		func(int) (any, error) { return exportContext(false) })
}

// resetSummarizerState 清空最近一次摘要记录。
func resetSummarizerState() string {
	lastDigestMu.Lock()
	had := lastDigestItem != nil
	lastDigestItem = nil
	lastDigestMu.Unlock()
	if had {
		logger.Info("CrystalAstral", "搜索者-摘要最近摘要记录已清空")
		return "已清空搜索者-摘要的最近摘要记录"
	}
	return "搜索者-摘要当前没有摘要记录"
}
