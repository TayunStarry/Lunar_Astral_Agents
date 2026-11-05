package Painter

// register.go 绘制者的数据导出与智能修复注册。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

var (
	// lastRunMu 保护最近一次绘制任务的记录
	lastRunMu sync.Mutex
	// lastRun 最近一次绘制任务的摘要（供数据导出）
	lastRun = map[string]any{}
)

// exportContext 绘制者的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	return map[string]any{
		"kind":         "painter-run",
		"generated_at": time.Now().Format(time.RFC3339),
		"last_run":     snapshotRun(),
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          agentID,
		Label:       roleLabel,
		Group:       "Go 智能体",
		Description: "绘制者最近一次绘制任务的说明、产出句柄与跨轮上下文条数",
		Export:      exportContext,
	})
	agentrepair.RegisterResettable(agentID, roleLabel,
		"绘制者的跨任务画作、最近作品路径与跨轮上下文；reset 清空这些运行时状态",
		resetState,
		func(int) (any, error) { return exportContext(false) })
}

// recordRun 记录最近一次绘制任务的结果。
func recordRun(summary string, handles []string) {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	lastRun = map[string]any{
		"summary":    summary,
		"handles":    handles,
		"updated_at": time.Now().Format(time.RFC3339),
	}
}

// snapshotRun 返回最近一次绘制记录副本。
func snapshotRun() map[string]any {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	out := make(map[string]any, len(lastRun))
	for key, value := range lastRun {
		out[key] = value
	}
	return out
}

// resetState 清空跨任务运行时状态。
func resetState() string {
	stateMu.Lock()
	defer stateMu.Unlock()
	had := state.path != "" || state.artwork.Handle != "" || len(state.history) > 0
	state = runtimeState{}
	if had {
		return "已清空绘制者的跨任务画作、最近作品路径与上下文"
	}
	return "绘制者当前没有跨任务状态"
}
