package Descriptor

// tool_catalog.go 描述者的工具目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/engine/agentinfo"

// ToolCatalog 返回描述者的文件查询工具清单。
func ToolCatalog() []agentinfo.Agent {
	defs := fileToolDefs()
	tools := make([]agentinfo.Tool, 0, len(defs))
	for _, def := range defs {
		tools = append(tools, agentinfo.Tool{Name: def.Function.Name, Description: def.Function.Description})
	}
	return []agentinfo.Agent{{
		ID: "descriptor", Name: "描述者", Source: "agents/Descriptor",
		Note:  "影像描述走视觉模型；文件探查使用下列查询工具",
		Tools: tools,
	}}
}
