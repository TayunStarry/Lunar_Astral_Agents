package SearchPlanner

// catalog.go 搜索者-规划的阶段能力目录（供控制台「工具列表」屏展示）。

import (
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

func init() {
	toolreg.Register(AgentID, toolreg.Schema{
		Name:        decisionToolName,
		Description: "通过 function calling 提交 pipeline / query / args / confidence；未知管线或把握不足时回落联网检索。",
	})
}

// ToolCatalog 返回搜索者-规划的阶段能力。
//
// 规划智能体通过 decide_search_plan 工具提交决策，不再从正文解析参数。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: modelRole,
		Note:  "搜索链第一步：把自然语言需求决策为管线、检索词与参数。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}
