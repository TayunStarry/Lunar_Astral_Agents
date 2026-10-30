package Savior

// dump.go 智能修复的数据导出：待处理故障队列与静默状态。

import (
	"sort"
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "savior",
		Label:       "智能修复",
		Group:       "Go 智能体",
		Description: "智能修复的待处理故障队列（智能体、错误、尝试次数）与静默截止时间",
		Export: func(includeImages bool) (any, error) {
			_ = includeImages
			failures := failureSnapshot()
			cooldowns := cooldownSnapshot()
			return map[string]any{
				"kind":          "savior-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"failures":      failures,
				"cooldowns":     cooldowns,
				"pending":       len(failures),
				"message_count": len(failures),
			}, nil
		},
	})
}

// failureSnapshot 复制当前待修复故障（按故障编号排序）。
func failureSnapshot() []map[string]any {
	mu.Lock()
	defer mu.Unlock()
	items := make([]*Failure, 0, len(pending))
	for _, failure := range pending {
		items = append(items, failure)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	out := make([]map[string]any, 0, len(items))
	for _, failure := range items {
		out = append(out, map[string]any{
			"id":       failure.ID,
			"agent":    failure.Agent,
			"error":    failure.Error,
			"at":       failure.At.Format(time.RFC3339),
			"attempts": failure.Attempts,
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
