package WishManager

// catalog.go 愿望管理智能体的工具清单（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回愿望管理智能体的工具清单。
func ToolCatalog() []agentinfo.Agent {
	tools := make([]agentinfo.Tool, 0, 6)
	for _, def := range toolDefinitions() {
		tools = append(tools, agentinfo.Tool{Name: def.Function.Name, Description: def.Function.Description})
	}
	return []agentinfo.Agent{{
		ID: AgentID, Name: roleLabel,
		Note:  "愿望管理智能体：按自然语言新增、改写、完成、删除、调整优先级与查询愿望。",
		Tools: tools,
	}}
}
