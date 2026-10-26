//go:build windows

package desktop

// dump.go 桌面操作的导出：最近一次人工任务的运行状态。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "desktop",
		Label:       "桌面操作",
		Group:       "Go 智能体",
		Description: "最近一次桌面操纵任务的要求、起止时间、结果或中断原因",
		Export: func(includeImages bool) (any, error) {
			return map[string]any{
				"kind":          "desktop-operator-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"message_count": 1,
				"run":           Snapshot(),
			}, nil
		},
	})
}
