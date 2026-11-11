package ContentRollback

// catalog.go 内容回滚工具声明与目录展示。

import (
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

var toolDefinitions = []agentbase.ToolDefinition{
	def(toolList, "列出指定路径或句柄文件的历史版本。", map[string]any{
		"path": strProp("文件路径或资源句柄"),
	}, []string{"path"}),
	def(toolRollback, "把文件回滚到指定版本；不传 version 时回滚到最近版本。", map[string]any{
		"path":    strProp("文件路径或资源句柄"),
		"version": strProp("版本 ID（可空）"),
	}, []string{"path"}),
}

func init() {
	for _, def := range toolDefinitions {
		toolreg.Register(AgentID, toolreg.Schema{
			Name:        def.Function.Name,
			Description: def.Function.Description,
			Parameters:  def.Function.Parameters,
		})
	}
}

func definitions() []agentbase.ToolDefinition {
	return append([]agentbase.ToolDefinition(nil), toolDefinitions...)
}

// ToolCatalog 返回内容回滚智能体的工具目录。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{ID: AgentID, Name: DisplayName, Tools: agentinfo.ToolsFromRegistry(AgentID)}}
}

func def(name, description string, properties map[string]any, required []string) agentbase.ToolDefinition {
	return agentbase.ToolDefinition{Type: "function", Function: agentbase.ToolFunctionDef{
		Name:        name,
		Description: description,
		Parameters: map[string]any{
			"type":       "object",
			"properties": properties,
			"required":   required,
		},
	}}
}

func strProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}
