package PlanManager

// catalog.go 计划管理智能体的工具清单（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回计划管理智能体的工具清单。
func ToolCatalog() []agentinfo.Agent {
	tools := make([]agentinfo.Tool, 0, 4)
	for _, def := range toolDefinitions() {
		tools = append(tools, agentinfo.Tool{Name: def.Function.Name, Description: def.Function.Description})
	}
	return []agentinfo.Agent{{
		ID: "plan_manager", Name: roleLabel,
		Note:  "计划管理智能体：按自然语言新增、删除、改写与查询计划表。",
		Tools: tools,
	}}
}
