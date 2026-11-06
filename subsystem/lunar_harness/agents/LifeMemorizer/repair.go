package LifeMemorizer

// repair.go 向拯救者注册生活记忆整理者的修复能力。

import (
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.Register(agentrepair.Handler{
		ID:          AgentID,
		Label:       RoleName,
		Description: "按批整理记忆，不保留跨批对话；可临时放宽正文保留下限，或把反复无法合并的簇标记为保留原条目",
		Actions:     []string{"relax_retain", "mark_unmergeable", "reset"},
		Inspect:     func(int) (any, error) { return exportContext(false) },
		Status:      statusSnapshot,
		Verify:      verifyPolicy,
		Repair:      applyRepair,
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

// applyRepair 执行生活记忆整理者的修复动作。
func applyRepair(action string, params map[string]any) (string, error) {
	switch action {
	case "relax_retain":
		percent := agentrepair.IntParam(params, "min_retain_percent", MinRetainPercent-5, retainPercentFloor, MinRetainPercent)
		ttl := policyTTLFrom(params)
		effective, until := setRetainOverride(percent, ttl)
		return fmt.Sprintf("整批条目化正文保留下限临时下调到 %d%%（到期 %s），下次整批整理按新值判定；相似簇合并固定按 %d%% 判定，不受此策略影响",
			effective, until.Format("15:04"), MergeMinRetainPercent), nil
	case "mark_unmergeable":
		inputs := snapshotInputs()
		if len(inputs) == 0 {
			return "", fmt.Errorf("没有保留上次合并输入，无法定位该簇；请先读取运行态确认")
		}
		marked, until := markClusterUnmergeable(inputs, policyTTLFrom(params))
		return fmt.Sprintf("已把 %d 条记忆标记为不可合并（到期 %s），其所在相似簇在有效期内直接保留原条目",
			marked, until.Format("15:04")), nil
	case "reset":
		return resetRepairState(), nil
	default:
		return "", fmt.Errorf("生活记忆整理者不支持动作：%s", action)
	}
}

// policyTTLFrom 读取策略有效期（分钟），夹在 [5, 120]。
func policyTTLFrom(params map[string]any) time.Duration {
	minutes := agentrepair.IntParam(params, "ttl_minutes", int(policyDefaultTTL/time.Minute),
		int(5), int(policyMaxTTL/time.Minute))
	return time.Duration(minutes) * time.Minute
}
