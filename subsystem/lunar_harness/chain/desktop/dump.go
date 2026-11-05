//go:build windows

package desktopchain

// dump.go 桌控链运行状态的导出与修复。
//
// 运行状态现在归桌控链所有，因此 agentdump / agentrepair 也在这里注册，
// 保持原标识 desktop，控制台与拯救者无需感知分层变化。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

// exportContext 桌面操作的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	return map[string]any{
		"kind":          "desktop-operator-context",
		"generated_at":  time.Now().Format(time.RFC3339),
		"message_count": 1,
		"run":           Snapshot(),
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "desktop",
		Label:       "桌面操作",
		Group:       "逻辑链",
		Description: "最近一次桌面操纵任务的要求、起止时间、结果或中断原因",
		Export:      exportContext,
	})
	agentrepair.RegisterResettable("desktop", "桌面操作",
		"桌面操纵的运行状态（要求、起止时间、结果）；reset 中断正在运行的任务",
		resetDesktopRun,
		func(int) (any, error) { return exportContext(false) })
}

// resetDesktopRun 中断当前任务；未运行时直接返回。
func resetDesktopRun() string {
	if Stop() {
		return "已中断桌面操纵任务"
	}
	return "桌面操纵当前没有运行中的任务"
}
