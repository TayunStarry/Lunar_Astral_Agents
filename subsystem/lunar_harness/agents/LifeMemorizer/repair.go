package LifeMemorizer

// repair.go 向维修侧注册生活记忆整理者的只读诊断入口。

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.Register(agentrepair.Handler{
		ID:          AgentID,
		Label:       RoleName,
		Description: "按批整理记忆，不保留跨批对话；可读保留阈值、合并策略与最近一次整理记录（只读）",
		Inspect:     func(int) (any, error) { return exportContext(false) },
		Status:      statusSnapshot,
	})
}

// statusSnapshot 返回记忆整理的配置与运行态。
func statusSnapshot() (any, error) {
	inputs := snapshotInputs()
	return map[string]any{
		"kind":                 "life-memorizer-status",
		"model":                config.AgentMultimodalModel,
		"prompt_role":          promptRole,
		"prompt_task":          promptTask,
		"max_text_runes":       MaxTextRunes,
		"model_retry":          ModelRetry,
		"policy":               policySnapshot(),
		"last_run":             snapshotLastRun(),
		"recorded_input_items": len(inputs),
		"recorded_input_runes": totalRunes(inputs),
		"has_recorded_inputs":  len(inputs) > 0,
	}, nil
}
