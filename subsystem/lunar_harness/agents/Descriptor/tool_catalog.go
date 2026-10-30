package Descriptor

// tool_catalog.go 描述者的工具目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回描述者的文件查询工具清单。
func ToolCatalog() []agentinfo.Agent {
	defs := fileToolDefs()
	tools := make([]agentinfo.Tool, 0, len(defs))
	for _, def := range defs {
		tools = append(tools, agentinfo.Tool{Name: def.Function.Name, Description: def.Function.Description})
	}
	return []agentinfo.Agent{{
		ID: "descriptor", Name: "描述者", Source: "agents/Descriptor",
		Note:  "描述智能体：按需读取影像与文件并生成结构化描述。",
		Tools: tools,
	}}
}
