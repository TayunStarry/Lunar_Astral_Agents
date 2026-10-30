package Descriptor

// dump.go 通用描述的导出：按次执行，不保留跨需求上下文；导出工具清单与运行说明。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "descriptor",
		Label:       "通用描述",
		Group:       "Go 智能体",
		Description: "通用描述的文件查询工具清单与运行说明（按次执行，不保留上下文）",
		Export: func(includeImages bool) (any, error) {
			defs := fileToolDefs()
			tools := make([]map[string]string, 0, len(defs))
			for _, def := range defs {
				tools = append(tools, map[string]string{
					"name":        def.Function.Name,
					"description": def.Function.Description,
				})
			}
			return map[string]any{
				"kind":          "descriptor-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"message_count": 0,
				"note":          "通用描述按次执行影像描述与文件探查，不维护跨需求对话上下文",
				"tools":         tools,
			}, nil
		},
	})
}
