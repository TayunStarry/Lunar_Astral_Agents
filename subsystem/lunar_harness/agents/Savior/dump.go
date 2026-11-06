package Savior

// dump.go 智能修复的数据导出：待处理故障队列、升级记录与静默状态。

import (
	"sort"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

// exportContext 智能修复的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	failures := failureSnapshot()
	return map[string]any{
		"kind":          "savior-context",
		"generated_at":  time.Now().Format(time.RFC3339),
		"failures":      failures,
		"escalated":     escalationSnapshot(),
		"cooldowns":     cooldownSnapshot(),
		"pending":       len(failures),
		"message_count": len(failures),
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "savior",
		Label:       "智能修复",
		Group:       "Go 智能体",
		Description: "智能修复的待处理故障（类型、判据、失败工具、水位、出现次数）与已升级记录",
		Export:      exportContext,
	})
}

// failureSnapshot 复制当前待修复故障（按故障编号排序）。
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
