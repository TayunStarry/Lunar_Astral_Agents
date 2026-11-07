package Screenshoter

// register.go 截图者的数据导出与智能修复注册。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

var (
	// lastRunMu 保护最近一次截图任务的记录
	lastRunMu sync.Mutex
	// lastRun 最近一次截图任务的摘要（供数据导出）
	lastRun = map[string]any{}
)

// exportContext 截图者的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	return map[string]any{
		"kind":         "screenshoter-run",
		"generated_at": time.Now().Format(time.RFC3339),
		"last_run":     snapshotRun(),
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          agentID,
		Label:       roleLabel,
		Group:       "Go 智能体",
		Description: roleLabel + "最近一次截图任务的说明、是否带主帧与跨轮上下文条数",
		Export:      exportContext,
	})
	agentrepair.RegisterReadonly(agentID, roleLabel,
		roleLabel+"的跨任务会话上下文与最近一次任务记录（只读）",
		func(int) (any, error) { return exportContext(false) })
}

// recordRun 记录最近一次截图任务的结果。
func recordRun(summary string, hasPrimary bool) {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	lastRun = map[string]any{
		"summary":     summary,
		"has_primary": hasPrimary,
		"updated_at":  time.Now().Format(time.RFC3339),
	}
}

// snapshotRun 返回最近一次截图记录副本。
func snapshotRun() map[string]any {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	out := make(map[string]any, len(lastRun))
	for key, value := range lastRun {
		out[key] = value
	}
	return out
}

// resetState 清空跨任务会话上下文。
func resetState() string {
	stateMu.Lock()
	defer stateMu.Unlock()
	had := len(state.history) > 0
	state = runtimeState{}
	if had {
		return "已清空" + roleLabel + "的跨任务会话上下文"
	}
	return roleLabel + "当前没有跨任务上下文"
}
