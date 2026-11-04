package LifeMemorizer

// dump.go 生活记忆整理者的数据导出：最近一次整理任务。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          AgentID,
		Label:       "生活记忆整理者",
		Group:       "Go 智能体",
		Description: "生活记忆整理者最近一次的批次规模、尝试次数与失败原因",
		Export: func(includeImages bool) (any, error) {
			_ = includeImages
			return map[string]any{
				"kind":         "life-memorizer-run",
				"generated_at": time.Now().Format(time.RFC3339),
				"last_run":     snapshotLastRun(),
			}, nil
		},
	})
}
