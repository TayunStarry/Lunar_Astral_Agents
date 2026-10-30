package FileEditor

// dump.go 文件编辑智能体的数据导出：供「数据导出」屏 / 调试端点导出当前上下文。
//
// 文件编辑按次执行、不保留对话上下文，因此这里导出「可复现的运行配置」：
// 系统提示词、工具清单与文档库现状。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "file_editor",
		Label:       "文件编辑",
		Group:       "Go 智能体",
		Description: "文件编辑智能体的系统提示词、工具清单与文档库现状（按次执行，不保留上下文）",
		Export: func(includeImages bool) (any, error) {
			_ = includeImages
			files, err := listDocuments("")
			if err != nil {
				files = nil
			}
			tools := ToolCatalog()
			var toolList any
			if len(tools) > 0 {
				toolList = tools[0].Tools
			}
			return map[string]any{
				"kind":          "file-editor-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"module":        Module,
				"system_prompt": systemPrompt(),
				"tool_count":    len(allTools),
				"tools":         toolList,
				"documents":     files,
				"message_count": len(files),
			}, nil
		},
	})
}
