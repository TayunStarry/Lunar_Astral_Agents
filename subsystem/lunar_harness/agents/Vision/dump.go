package Vision

// dump.go 视觉理解的导出：按次执行，不保留跨需求上下文；导出工具清单与运行说明。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

// exportContext 视觉理解的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	return map[string]any{
		"kind":          "vision-context",
		"generated_at":  time.Now().Format(time.RFC3339),
		"message_count": 0,
		"note":          "视觉理解按次执行图片 / 动图 / 视频的观看与描述，不维护跨需求对话上下文",
		"tools": []map[string]string{
			{"name": ToolDescribe, "description": "观看图片 / 动图并生成内容描述"},
			{"name": ToolAsk, "description": "对图片 / 动图提出定向问题"},
			{"name": ToolWatch, "description": "解读本机视频地址并合成整体感受"},
		},
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "vision",
		Label:       roleLabel,
		Group:       "Go 智能体",
		Description: "视觉理解智能体的工具清单与运行说明（按次执行，不保留上下文）",
		Export:      exportContext,
	})
}
