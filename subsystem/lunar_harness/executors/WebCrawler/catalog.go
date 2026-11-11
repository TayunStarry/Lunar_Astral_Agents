package WebCrawler

// catalog.go 网页抓取执行体的工具目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回网页抓取执行体的检索与页面读取能力。
func ToolCatalog() []agentinfo.Agent {
	tools := []agentinfo.Tool{
		{Name: ToolCrawl, Description: "按检索词执行多引擎降级检索（bing → baidu → sogou），过滤字典站与工具站兜底页后并发采集详情页正文。"},
		{Name: ToolReadURL, Description: "打开指定页面并滚动走查提取正文（单页按配额截断，不发起模型调用）。"},
	}
	return []agentinfo.Agent{{
		ID: "web_crawler", Name: "网页抓取", Kind: agentinfo.KindExecutor,
		Note:  "网页抓取执行体：基于 Lunar Harness 原生浏览器完成多引擎检索、详情页并发采集与近期访问缓存，由搜索链调度；不发起任何模型调用。",
		Tools: tools,
	}}
}
