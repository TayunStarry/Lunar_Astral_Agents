package LunarMatrix

// dump.go Lunar Matrix 的导出：独立上下文（最近若干轮需求与答复）。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "lunar_matrix",
		Label:       "互助论坛",
		Group:       "Go 智能体",
		Description: "互助论坛智能体最近的需求/答复上下文（模块：Lunar Matrix；不含内部 api_key）",
		Export: func(includeImages bool) (any, error) {
			messages := historySnapshot()
			return map[string]any{
				"kind":          "lunar-matrix-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"message_count": len(messages),
				"messages":      messages,
			}, nil
		},
	})
}
