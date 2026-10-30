//go:build windows

package desktop

// tool_catalog.go 桌面操作的工具目录（供控制台「工具列表」屏展示）。

import (
	"sort"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// ToolCatalog 返回桌面操作的 function calling 工具清单（按名称排序）。
func ToolCatalog() []agentinfo.Agent {
	names := make([]string, 0, len(allToolDefs))
	for name := range allToolDefs {
		names = append(names, name)
	}
	sort.Strings(names)

	tools := make([]agentinfo.Tool, 0, len(names))
	for _, name := range names {
		def := allToolDefs[name]
		tools = append(tools, agentinfo.Tool{Name: def.Function.Name, Description: def.Function.Description})
	}
	return []agentinfo.Agent{{
		ID: "desktop_operator", Name: "桌面操作", Source: "agents/desktop",
		Note:  "桌面操控智能体：按自然语言要求操作真实桌面键鼠。",
		Tools: tools,
	}}
}
