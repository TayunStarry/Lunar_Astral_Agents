package search

// repair.go 向维修侧注册搜索链的只读诊断入口。
//
// 搜索链按次执行（每次重新打开浏览器、重新规划页面），没有跨轮上下文；
// 可观察的是最近一次检索记录。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("web_search", "网络搜索",
		"网络搜索的最近一次检索记录（检索词 / 引擎 / 页数 / 报告，只读）",
		func(int) (any, error) { return exportContext(false) })
}
