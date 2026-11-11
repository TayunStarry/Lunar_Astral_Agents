package ManipulateFiles

// catalog.go 文件操纵工具声明与目录展示。

import (
	"LunarSubsystem/LunarHarness/engine/resourcehub"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

var toolDefinitions = []agentbase.ToolDefinition{
	{
		Type: "function",
		Function: agentbase.ToolFunctionDef{
			Name:        toolFetch,
			Description: resourcehub.ToolDescription(),
			Parameters:  resourcehub.ToolSchema(),
		},
	},
	toolagentDef(toolReplace, "按行替换文件内容；all=true 时替换所有行。", map[string]any{
		"path":       toolagentStr("文件路径或资源句柄"),
		"start_line": toolagentInt("起始行（1 基，all=true 时忽略）", 1, 1<<20),
		"end_line":   toolagentInt("结束行（含端点，all=true 时忽略）", 1, 1<<20),
		"content":    toolagentStr("替换后的完整文本"),
		"all":        toolagentBool("是否替换所有行"),
	}, []string{"path", "content"}),
	toolagentDef(toolAppend, "在文件末尾追加一行或多行内容。", map[string]any{
		"path":    toolagentStr("文件路径或资源句柄"),
		"content": toolagentStr("追加内容"),
	}, []string{"path", "content"}),
	toolagentDef(toolDelete, "删除文件中指定行区间的内容。", map[string]any{
		"path":       toolagentStr("文件路径或资源句柄"),
		"start_line": toolagentInt("起始行（1 基）", 1, 1<<20),
		"end_line":   toolagentInt("结束行（含端点）", 1, 1<<20),
	}, []string{"path", "start_line", "end_line"}),
	toolagentDef(toolCreate, "在指定路径创建新的空白文件。", map[string]any{
		"path": toolagentStr("文件路径"),
	}, []string{"path"}),
	toolagentDef(toolUninstall, "删除指定路径下的文件。", map[string]any{
		"path": toolagentStr("文件路径或资源句柄"),
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

// ToolCatalog 返回文件操纵智能体的工具目录。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{ID: AgentID, Name: DisplayName, Tools: agentinfo.ToolsFromRegistry(AgentID)}}
}

func toolagentDef(name, description string, properties map[string]any, required []string) agentbase.ToolDefinition {
	if required == nil {
		required = []string{}
	}
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

func toolagentStr(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func toolagentInt(description string, minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": minimum, "maximum": maximum}
}

func toolagentBool(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}
