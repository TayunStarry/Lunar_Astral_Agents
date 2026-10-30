package Descriptor

// tool_catalog.go 通用描述的工具目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回通用描述的文件查询工具清单。
func ToolCatalog() []agentinfo.Agent {
	defs := fileToolDefs()
	tools := make([]agentinfo.Tool, 0, len(defs))
	for _, def := range defs {
		tools = append(tools, agentinfo.Tool{Name: def.Function.Name, Description: def.Function.Description})
	}
	return []agentinfo.Agent{{
		ID: "descriptor", Name: "通用描述", Source: "agents/Descriptor",
		Note:  "通用描述智能体：按需读取影像与文件并生成结构化描述。",
		Tools: tools,
	}}
}
