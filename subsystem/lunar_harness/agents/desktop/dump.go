//go:build windows

package desktop

// dump.go 桌面操纵智能体（模型侧）的导出：各阶段最近一次执行结果。
//
// 任务运行状态（要求、起止时间、结果）已上移到 chain/desktop，由桌控链导出；
// 本包只暴露每个模型阶段的最近执行结果。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "desktop_agent",
		Label:       "桌面操作-模型",
		Group:       "Go 智能体",
		Description: "桌面操纵智能体各阶段（编纂/启动/UIA/规划/操作）最近一次执行结果",
		Export: func(includeImages bool) (any, error) {
			return map[string]any{
				"kind":         "desktop-agent-phases",
				"generated_at": time.Now().Format(time.RFC3339),
				"phases":       snapshotPhases(),
			}, nil
		},
	})
}
