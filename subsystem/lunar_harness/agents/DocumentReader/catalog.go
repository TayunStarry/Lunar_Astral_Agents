package DocumentReader

// catalog.go 文献解析智能体的工具目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回文献解析智能体的工具清单。
func ToolCatalog() []agentinfo.Agent {
	defs := fileToolDefs()
	tools := make([]agentinfo.Tool, 0, len(defs))
	for _, def := range defs {
		tools = append(tools, agentinfo.Tool{Name: def.Function.Name, Description: def.Function.Description})
	}
	return []agentinfo.Agent{{
		ID: "document_reader", Name: "文献解析", Source: "agents/DocumentReader",
		Note:  "文献解析智能体：自动识别格式，按行 / 分批解析小说、文献、文档、DTS、源码等超大规模文本。",
		Tools: tools,
	}}
}
