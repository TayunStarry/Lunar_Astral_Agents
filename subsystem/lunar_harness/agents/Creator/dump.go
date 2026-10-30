package Creator

// dump.go 创作体系的数据导出：上一轮规划日志与可调度能力。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "creative",
		Label:       "创作",
		Group:       "Go 智能体",
		Description: "创作体系的规划日志（上一轮计划、备忘、参考资料、错误与收尾）及可调度智能体",
		Export: func(includeImages bool) (any, error) {
			_ = includeImages
			journal := loadJournal()
			agents := make([]map[string]string, 0, len(agentLabels))
			for agent, label := range agentLabels {
				agents = append(agents, map[string]string{"id": agent, "label": label})
			}
			return map[string]any{
				"kind":          "creative-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"journal":       journal,
				"agents":        agents,
				"tools":         plannerTools,
				"message_count": len(journal.LastPlans),
			}, nil
		},
	})
}
