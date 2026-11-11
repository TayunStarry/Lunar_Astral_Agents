//go:build windows

package desktopchain

// dump.go 桌控链运行状态的导出与修复。
//
// 运行状态现在归桌控链所有，因此 agentdump / agentrepair 也在这里注册，
// 保持原标识 desktop，控制台与拯救者无需感知分层变化。

import (
	"time"

	"LunarSubsystem/LunarHarness/agents/desktop"
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

// familyExportContext 族内各成员最近一次执行结果；dump 导出与只读诊断共用这一份。
func familyExportContext(includeImages bool) (any, error) {
	return map[string]any{
		"kind":         "desktop-family-members",
		"generated_at": time.Now().Format(time.RFC3339),
		"members":      desktop.SnapshotCalls(),
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "desktop",
		Label:       "桌面编排",
		Group:       "逻辑链",
		Description: "最近一次桌面编排任务的要求、起止时间、结果或中断原因",
		Export:      exportContext,
	})
	agentrepair.RegisterReadonly("desktop", "桌面编排",
		"桌面编排的运行状态（要求、起止时间、结果，只读）",
		func(int) (any, error) { return exportContext(false) })
	agentdump.Register(agentdump.Provider{
		ID:          chainScopeID,
		Label:       chainScopeLabel,
		Group:       "Go 智能体",
		Description: chainScopeLabel + "族内各成员（感知/规划/执行）最近一次执行结果",
		Export:      familyExportContext,
	})
}
