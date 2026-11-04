package SearchSummarizer

// catalog.go 搜索者-摘要的阶段能力目录（供控制台「工具列表」屏展示）。

import (
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

func init() {
	toolreg.Register(AgentID, toolreg.Schema{
		Name:        "digest_pages",
		Description: "把首页与详情页正文拼接为长文，一次调用按原始问题做选择性摘要；模型退回否定占位句时改用正文兜底。",
	})
}

// ToolCatalog 返回搜索者-摘要的阶段能力。
//
// 摘要智能体不使用 function calling；这里展示它在搜索链中实际承担的一步提取。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: "搜索者-摘要", Source: "agents/SearchSummarizer",
		Note:  "搜索链第三步：把全部页面正文一次提取为针对性摘要。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}
