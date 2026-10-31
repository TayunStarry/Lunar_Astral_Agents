package DocumentReader

// dump.go 文献解析的导出：按次执行，不保留跨需求上下文；导出工具清单与运行说明。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "document_reader",
		Label:       "文献解析",
		Group:       "Go 智能体",
		Description: "文献解析智能体的文件查询工具清单与运行说明（按次执行，不保留上下文）",
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
				"kind":          "document-reader-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"message_count": 0,
				"note":          "文献解析按次执行文本内容探查，单文件超过 10000 字强制分批，不维护跨需求对话上下文",
				"tools":         tools,
			}, nil
		},
	})
}
