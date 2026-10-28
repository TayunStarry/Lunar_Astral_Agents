package MutualForum

// tool_catalog.go Mutual Forum 的论坛工具目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回 Mutual Forum 智能体的全部论坛工具。
func ToolCatalog() []agentinfo.Agent {
	defs := forumTools()
	tools := make([]agentinfo.Tool, 0, len(defs))
	for _, def := range defs {
		tools = append(tools, agentinfo.Tool{Name: def.Function.Name, Description: def.Function.Description})
	}
	return []agentinfo.Agent{{
		ID: "mutual_forum", Name: "互助论坛", Source: "agents/MutualForum",
		Note:  "模块：Mutual Forum；工具是否可见取决于独立配置与论坛健康检查",
		Tools: tools,
	}}
}
