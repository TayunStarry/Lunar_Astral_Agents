package WebSearch

// tool_catalog.go 网络搜索的能力目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回网络搜索流程的阶段能力。
//
// 网络搜索是自主流水线智能体，不使用 function calling；这里展示的是它在一次检索中
// 实际执行的内部步骤，便于用户理解它会把任务拆成哪些动作。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: "web_search", Name: "网络搜索", Source: "agents/WebSearch",
		Note: "检索智能体：多引擎搜索并整理结构化报告。",
		Tools: []agentinfo.Tool{
			{Name: "extract_query", Description: "用小模型把自然语言需求提炼成检索词。"},
			{Name: "search_with_fallback", Description: "按必应 → 百度 → 搜狗的顺序尝试检索页并提取结果条目。"},
			{Name: "scroll_and_extract", Description: "「网页搜索:首页」从上到下完整滚动，提取结果条目与全部正文，随后关闭首页。"},
			{Name: "open_detail_pages", Description: "「网页搜索:详情」同时打开全部详情页，全部加载后统一滚动提取正文，随后统一关窗。"},
			{Name: "digest_all_pages", Description: "把全部页面正文拼接为长文，一次调用让模型按原始问题做针对性摘要提取，输出即最终结果。"},
		},
	}}
}
