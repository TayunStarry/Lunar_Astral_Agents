package search

// repair.go 向拯救者注册搜索链的修复能力。
//
// 搜索链按次执行（每次重新打开浏览器、重新规划页面），没有跨轮上下文；
// 可修的是「最近一次检索记录」等运行期痕迹。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("web_search", "网络搜索",
		"网络搜索的最近一次检索记录（检索词 / 引擎 / 页数 / 报告）；reset 清空该记录",
		resetSearchState)
}

// resetSearchState 清空最近一次检索记录，下一次搜索从空白状态开始。
func resetSearchState() string {
	lastSearchMu.Lock()
	had := lastSearch != nil
	lastSearch = nil
	lastSearchMu.Unlock()
	if had {
		logger.Info("CrystalAstral", "网络搜索最近检索记录已清空")
		return "已清空网络搜索的最近检索记录"
	}
	return "网络搜索当前没有检索记录"
}
