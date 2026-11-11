package CodeExecution

// catalog.go 代码执行工具声明与目录展示。

import (
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

var toolDefinitions = []agentbase.ToolDefinition{
	def(toolExecute, "执行指定路径或句柄引用的 HTML/JavaScript 代码文件，返回执行结果。", map[string]any{
		"path":        strProp("代码文件路径或资源句柄"),
		"timeout_sec": intProp("运行时长上限（秒）", 5, 3600),
	}, []string{"path"}),
	def(toolParse, "列出代码文件中的函数签名与引用的其他代码文件路径。", map[string]any{
		"path": strProp("代码文件路径或资源句柄"),
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

// ToolCatalog 返回代码执行智能体的工具目录。
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

func intProp(description string, minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": minimum, "maximum": maximum}
}
