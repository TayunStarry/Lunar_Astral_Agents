package LifeMemorizer

// policy.go 记忆整理的运行态：保留最近一次合并输入，供修复后复跑验证。
//
// 智能维修只允许复位已经产生的状态（最近记录与复跑输入）；
// 不得临时下调阈值或标记条目跳过合并，那会改变后续批次的判定策略。

import (
	"context"
	"strings"
)

// recordInputs 保存最近一次合并输入的有界副本，供修复后复跑验证。
func recordInputs(contents []string) {
	inputsMu.Lock()
	defer inputsMu.Unlock()
	kept := make([]string, 0, min(len(contents), maxRecordedInputs))
	total := 0
	for _, content := range contents {
		if len(kept) >= maxRecordedInputs {
			break
		}
		text := strings.Join(strings.Fields(content), " ")
		if text == "" {
			continue
		}
		runes := len([]rune(text))
		if total+runes > maxRecordedInputRunes {
			break
		}
		total += runes
		kept = append(kept, text)
	}
	lastInputs = kept
}

// snapshotInputs 复制最近一次合并输入。
func snapshotInputs() []string {
	inputsMu.Lock()
	defer inputsMu.Unlock()
	return append([]string(nil), lastInputs...)
}

// policySnapshot 返回固定的整理判据（只读，供运行态展示）。
func policySnapshot() map[string]any {
	return map[string]any{
		"min_retain_percent":       MinRetainPercent,
		"merge_min_retain_percent": MergeMinRetainPercent,
		"merge_min_runes":          MergeMinRunes,
	}
}

// resetRepairState 清空最近记录与复跑输入；reset 动作的明确范围。
func resetRepairState() string {
	inputsMu.Lock()
	lastInputs = nil
	inputsMu.Unlock()
	return resetLastRun() + "，并清空复跑输入"
}

// verifyPolicy 验证修复结果：reset 后按状态读回确认，其余情况复跑最近一次失败合并。
func verifyPolicy(ctx context.Context, _ map[string]any) (any, error) {
	inputs := snapshotInputs()
	if len(inputs) == 0 {
		if len(snapshotLastRun()) == 0 {
			return map[string]any{
				"verified": true,
				"mode":     "state_check",
				"detail":   "最近整理记录与复跑输入均已清空；未改动阈值或合并策略",
			}, nil
		}
		return map[string]any{
			"verified": false,
			"mode":     "state_check",
			"reason":   "复跑输入已清空，但最近整理记录仍在；请重新执行 reset",
		}, nil
	}
	merged, err := mergeInternal(ctx, inputs, false)
	if err != nil {
		return map[string]any{
			"verified":   false,
			"mode":       "rerun",
			"reason":     err.Error(),
			"thresholds": mergeThresholds(inputs),
		}, nil
	}
	if len(merged) == 0 {
		return map[string]any{
			"verified":    true,
			"mode":        "rerun",
			"reason":      "复跑未产生合并结果，链路层会保留原条目",
			"input_items": len(inputs),
			"thresholds":  mergeThresholds(inputs),
		}, nil
	}
	return map[string]any{
		"verified":     true,
		"mode":         "rerun",
		"reason":       "复跑合并通过当前正文下限",
		"input_items":  len(inputs),
		"output_items": len(merged),
		"output_runes": totalRunes(merged),
		"thresholds":   mergeThresholds(inputs),
	}, nil
}

// mergeThresholds 返回本次合并实际使用的判定口径（百分比与绝对字数下限）。
func mergeThresholds(contents []string) map[string]any {
	return map[string]any{
		"merge_min_retain_percent": MergeMinRetainPercent,
		"merge_min_output_runes":   mergeRetainFloor(contents),
	}
}
