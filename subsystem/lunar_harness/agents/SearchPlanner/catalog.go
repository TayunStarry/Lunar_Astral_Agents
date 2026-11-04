package SearchPlanner

// catalog.go 搜索者-规划的阶段能力目录（供控制台「工具列表」屏展示）。

import (
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

func init() {
	toolreg.Register(AgentID, toolreg.Schema{
		Name:        "plan_search",
		Description: "一次调用同时输出 pipeline / query / args / confidence；未知管线或把握不足时回落联网检索。",
	})
}

// ToolCatalog 返回搜索者-规划的阶段能力。
//
// 规划智能体不使用 function calling；这里展示它在搜索链中实际承担的一步决策。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: "搜索者-规划", Source: "agents/SearchPlanner",
		Note:  "搜索链第一步：把自然语言需求决策为管线、检索词与参数。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}
