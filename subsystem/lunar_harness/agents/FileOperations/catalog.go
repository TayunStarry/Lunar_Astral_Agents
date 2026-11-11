package FileOperations

// catalog.go 文件操作工具声明与目录展示。

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
	{
		Type: "function",
		Function: agentbase.ToolFunctionDef{
			Name:        toolManipulate,
			Description: "按顺序派发文件操纵、归档操纵、代码执行或内容回滚子任务；任一步失败即停止。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"operations": map[string]any{
						"type":        "array",
						"description": "按顺序执行的操作列表，单次最多 6 个",
						"maxItems":    maxSteps,
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"type": map[string]any{
									"type":        "string",
									"enum":        []string{"manipulate_files", "manipulate_archive", "execute_code", "rollback"},
									"description": "子智能体类型",
								},
								"description": map[string]any{
									"type":        "string",
									"description": "交给该子智能体的完整自然语言任务",
								},
							},
							"required": []string{"type", "description"},
						},
					},
				},
				"required": []string{"operations"},
			},
		},
	},
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

// ToolCatalog 返回文件操作智能体的工具目录。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{ID: AgentID, Name: DisplayName, Tools: agentinfo.ToolsFromRegistry(AgentID)}}
}
