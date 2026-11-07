package Savior

// dump.go 智能修复的数据导出：待处理故障队列、维修建议、升级记录与静默状态。

import (
	"sort"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

// exportContext 智能修复的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	failures := failureSnapshot()
	return map[string]any{
		"kind":          "savior-context",
		"generated_at":  time.Now().Format(time.RFC3339),
		"failures":      failures,
		"advices":       adviceSnapshot(),
		"escalated":     escalationSnapshot(),
		"cooldowns":     cooldownSnapshot(),
		"pending":       len(failures),
		"message_count": len(failures),
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "savior",
		Label:       roleLabel,
		Group:       "Go 智能体",
		Description: roleLabel + "的待诊断故障与已给出的维修建议（判断、可能原因、建议步骤、风险点、置信度）",
		Export:      exportContext,
	})
	agentrepair.RegisterReadonly("savior", roleLabel,
		roleLabel+"自身的待诊断故障、已给出的维修建议与静默状态（只读）",
		func(int) (any, error) { return exportContext(false) })
}

// failureSnapshot 复制当前待诊断故障（按故障编号排序）。
func failureSnapshot() []map[string]any {
	mu.Lock()
	items := make([]*Failure, 0, len(pending))
	for _, failure := range pending {
		items = append(items, failure)
	}
	mu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	out := make([]map[string]any, 0, len(items))
	for _, failure := range items {
		out = append(out, map[string]any{
			"id":               failure.ID,
			"agent":            failure.Agent,
			"kind":             failure.Kind,
			"error":            failure.Error,
			"criterion":        failure.Criterion,
			"entry":            failure.Entry,
			"tool":             failure.Tool,
			"tool_args":        failure.ToolArgs,
			"retries":          failure.Retries,
			"context_messages": failure.ContextMessages,
			"context_runes":    failure.ContextRunes,
			"metrics":          failure.Metrics,
			"occurrences":      failure.Occurrences,
			"attempts":         failure.Attempts,
			"escalated":        failure.Escalated,
			"conclusions":      failure.Conclusions,
			"at":               failure.At.Format(time.RFC3339),
		})
	}
	return out
}

// adviceSnapshot 复制已给出的维修建议（按时间排序，最多 maxAdvices 条）。
func adviceSnapshot() []map[string]any {
	mu.Lock()
	items := make([]*Advice, 0, len(advices))
	for _, advice := range advices {
		items = append(items, advice)
	}
	mu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].At.Before(items[j].At) })
	out := make([]map[string]any, 0, len(items))
	for _, advice := range items {
		out = append(out, map[string]any{
			"failure_id": advice.FailureID,
			"agent":      advice.Agent,
			"kind":       advice.Kind,
			"error":      advice.Error,
			"diagnosis":  advice.Diagnosis,
			"causes":     advice.Causes,
			"steps":      advice.Steps,
			"risks":      advice.Risks,
			"confidence": advice.Confidence,
			"basis":      advice.Basis,
			"at":         advice.At.Format(time.RFC3339),
		})
	}
	return out
}

// escalationSnapshot 复制已升级记录（按升级时间排序）。
func escalationSnapshot() []map[string]any {
	mu.Lock()
	items := make([]*escalation, 0, len(escalated))
	for _, record := range escalated {
		items = append(items, record)
	}
	mu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].At.Before(items[j].At) })
	out := make([]map[string]any, 0, len(items))
	for _, record := range items {
		out = append(out, map[string]any{
			"agent":       record.Failure.Agent,
			"kind":        record.Failure.Kind,
			"error":       record.Failure.Error,
			"occurrences": record.Failure.Occurrences,
			"attempts":    record.Failure.Attempts,
			"conclusion":  record.Conclusion,
			"at":          record.At.Format(time.RFC3339),
		})
	}
	return out
}

// cooldownSnapshot 复制当前静默状态（剩余秒数为 0 表示已到期）。
func cooldownSnapshot() map[string]int64 {
	mu.Lock()
	defer mu.Unlock()
	out := make(map[string]int64, len(cooldown))
	now := time.Now()
	for agent, until := range cooldown {
		remain := until.Sub(now)
		if remain < 0 {
			remain = 0
		}
		out[agent] = int64(remain.Seconds())
	}
	return out
}
