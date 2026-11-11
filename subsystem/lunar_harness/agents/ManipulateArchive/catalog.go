package ManipulateArchive

// catalog.go 归档操纵工具声明与目录展示。

import (
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

var toolDefinitions = []agentbase.ToolDefinition{
	def(toolCreate, "创建归档文件并加入指定文件。", map[string]any{
		"target":  strProp("归档路径（.zip / .ltpx）"),
		"sources": arrayProp("要加入归档的文件或目录路径"),
	}, []string{"target", "sources"}),
	def(toolUninstall, "删除指定路径下的归档文件。", map[string]any{
		"target": strProp("归档路径或句柄"),
	}, []string{"target"}),
	def(toolExtract, "从归档中提取指定文件。", map[string]any{
		"target":     strProp("归档路径或句柄"),
		"entry":      strProp("归档内条目名"),
		"output_dir": strProp("提取到哪个目录（可空，默认归档所在目录）"),
	}, []string{"target", "entry"}),
	def(toolList, "列出归档中的全部条目。", map[string]any{
		"target": strProp("归档路径或句柄"),
	}, []string{"target"}),
	def(toolAdd, "把指定文件添加到归档中。", map[string]any{
		"target":  strProp("归档路径或句柄"),
		"entry":   strProp("归档内条目名"),
		"source":  strProp("本地来源文件路径或句柄"),
		"content": strProp("直接写入条目的文本内容（与 source 二选一）"),
	}, []string{"target", "entry"}),
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

// ToolCatalog 返回归档操纵智能体的工具目录。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{ID: AgentID, Name: DisplayName, Tools: agentinfo.ToolsFromRegistry(AgentID)}}
}

func def(name, description string, properties map[string]any, required []string) agentbase.ToolDefinition {
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

func strProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func arrayProp(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "string"},
	}
}
