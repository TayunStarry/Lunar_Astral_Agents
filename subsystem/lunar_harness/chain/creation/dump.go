package creation

// dump.go 创作体系的数据导出：上一轮规划日志与可调度能力。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/fileflow"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

// exportContext 自由创作的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	creatorJournal := loadRoundJournal(fileflow.ModeCreator)
	assignedJournal := loadRoundJournal(fileflow.ModeAssigned)
	agents := make([]map[string]string, 0, len(agentLabels))
	for agent, label := range agentLabels {
		agents = append(agents, map[string]string{"id": agent, "label": label})
	}
	return map[string]any{
		"kind":             "creative-context",
		"generated_at":     time.Now().Format(time.RFC3339),
		"journal":          creatorJournal,
		"assigned_journal": assignedJournal,
		"agents":           agents,
		"tools":            plannerTools,
		"message_count":    len(creatorJournal.LastPlans),
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "creative",
		Label:       "自由创作",
		Group:       "Go 智能体",
		Description: "自由创作体系的跨轮记录（上一轮计划、备忘、参考资料、错误与内容级变更）及可调度智能体",
		Export:      exportContext,
	})
}
