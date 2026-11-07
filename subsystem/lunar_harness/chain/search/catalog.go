package search

// catalog.go 搜索链的能力目录（供控制台「工具列表」屏展示）。
//
// 目录内容不再在链侧硬编码：由两个搜索者智能体在 protocol/toolreg 登记的阶段能力派生。

import (
	"LunarSubsystem/LunarHarness/agents/SearchPlanner"
	"LunarSubsystem/LunarHarness/agents/SearchSummarizer"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// ToolCatalog 返回搜索链的阶段能力。
//
// 搜索链不使用 function calling；这里展示的是它在一次检索中调度、且由智能体承担的模型步骤，
// 抓取与天气查询由执行体承担，不在智能体声明里。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: "web_search", Name: "网络搜索", Kind: agentinfo.KindChain,
		Note: "搜索链：规划 → 抓取 → 摘要三段式调度，多引擎搜索并整理结构化报告。",
		Tools: agentinfo.ToolsFromRegistry(
			SearchPlanner.AgentID,
			SearchSummarizer.AgentID,
		),
	}}
}
