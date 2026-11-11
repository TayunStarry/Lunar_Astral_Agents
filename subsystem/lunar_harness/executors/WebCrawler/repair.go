package WebCrawler

// repair.go 向维修侧注册网页抓取的只读诊断入口。
//
// 网页抓取按次执行（每次重新打开浏览器、重新检索），不保留跨轮上下文；
// 失败通常来自搜索引擎结果页或目标站点本身。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("web_crawler", "网页抓取",
		"网页抓取的最近一次检索配置与工具清单（按次执行、不保留对话上下文，只读）",
		func(int) (any, error) { return exportContext(false) })
}
