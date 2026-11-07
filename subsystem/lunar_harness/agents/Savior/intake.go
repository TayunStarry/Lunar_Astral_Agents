package Savior

// intake.go 故障接收：结构化简报落成故障条目，并按「智能体 + 类型」去重计数。

import (
	"context"
	"errors"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/loopguard"
)

// Report 上报一次故障（兼容旧入口，只有智能体与错误文本）。
func Report(agent string, err error) {
	ReportDetail(agentbase.FailureDetail{Agent: agent, Err: err})
}

// ReportRun 上报一次运行失败，忽略主动取消与循环输出截断。
func ReportRun(agent string, err error) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, loopguard.ErrLoop) {
		return
	}
	ReportDetail(agentbase.FailureDetail{Agent: agent, Err: err})
}

// ReportDetail 上报一次结构化故障。
//
// 同一「智能体 + 故障类型」只保留一条待诊断故障，其余更新为最新状态并累计出现次数；
// 连续出现达到 escalateAfter 时标记升级：仍会给出一次维修建议，但不再自动重试。
func ReportDetail(detail agentbase.FailureDetail) {
	agent := strings.TrimSpace(detail.Agent)
	if agent == "" {
		agent = "unknown"
	}
	kind := strings.TrimSpace(detail.Kind)
	if kind == "" {
		kind = agentbase.FailureKindUnknown
	}
	message := strings.TrimSpace(detail.ErrorText)
	if message == "" && detail.Err != nil {
		message = strings.TrimSpace(detail.Err.Error())
	}
	if message == "" {
		message = "未知错误"
	}
	key := failureKey(agent, kind)

	mu.Lock()
	defer mu.Unlock()
	if until, ok := cooldown[agent]; ok {
		if time.Now().Before(until) {
			return
		}
		delete(cooldown, agent)
	}
	record := history[key]
	if record == nil {
		if len(pending) >= maxPending {
			logger.Warn(roleLabel, "["+roleLabel+"] 故障队列已满，忽略 %s 的新故障", agent)
			return
		}
		record = &faultHistory{}
		history[key] = record
	} else if time.Since(record.LastAt) > historyResetWindow {
		// 长期未复发说明上一次修复已生效，重新按「连续出现」计数。
		record.Count = 0
	}
	record.Count++
	record.LastAt = time.Now()

	if existing, ok := pending[key]; ok {
		fillFailure(existing, detail, message, kind)
		existing.At = time.Now()
		existing.Occurrences = record.Count
		existing.Escalated = existing.Escalated || record.Count >= escalateAfter
		logger.Warn(roleLabel, "["+roleLabel+"] 故障 #%d（%s/%s）再次出现（第 %d 次）：%s",
			existing.ID, agent, kind, record.Count, previewRunes(message, maxBriefRunes))
		return
	}
	seq++
	failure := &Failure{
		ID:          seq,
		Key:         key,
		Agent:       agent,
		At:          time.Now(),
		FirstAt:     time.Now(),
		Occurrences: record.Count,
	}
	fillFailure(failure, detail, message, kind)
	failure.Escalated = failure.Occurrences >= escalateAfter
	pending[key] = failure
	order = append(order, key)
	logger.Warn(roleLabel, "["+roleLabel+"] 收到故障 #%d（%s/%s，第 %d 次）：%s",
		seq, agent, kind, failure.Occurrences, previewRunes(message, maxBriefRunes))
}

// Pending 返回待修复故障数。
func Pending() int {
	mu.Lock()
	defer mu.Unlock()
	return len(pending)
}

// failureKey 去重键：同一智能体的同类故障合并计数。
func failureKey(agent, kind string) string {
	return agent + "|" + kind
}

// fillFailure 把上报字段写入故障条目（文本统一截断）。
func fillFailure(failure *Failure, detail agentbase.FailureDetail, message, kind string) {
	failure.Kind = kind
	failure.Error = previewRunes(message, maxBriefRunes)
	failure.Criterion = previewRunes(detail.Criterion, maxBriefRunes)
	failure.Entry = previewRunes(detail.Entry, 120)
	failure.Tool = previewRunes(detail.Tool, 120)
	failure.ToolArgs = previewRunes(detail.ToolArgs, maxBriefRunes)
	failure.ToolResult = previewRunes(detail.ToolResult, maxBriefRunes)
	failure.Retries = detail.Retries
	failure.ContextMessages = detail.ContextMessages
	failure.ContextRunes = detail.ContextRunes
	failure.Metrics = mergeMetrics(failure.Metrics, detail.Metrics)
}

// mergeMetrics 合并数值水位（覆盖同名键，保持有界）。
func mergeMetrics(base, extra map[string]int64) map[string]int64 {
	if len(extra) == 0 {
		return base
	}
	out := make(map[string]int64, len(base)+len(extra))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

// resolve 给出建议：清空该键的历史计数，避免下次同类故障被误判为复发。
func resolve(key string) {
	mu.Lock()
	delete(pending, key)
	delete(history, key)
	delete(escalated, key)
	mu.Unlock()
}

// storeAdvice 记录一条维修建议（同键只保留最新，最多 maxAdvices 条）。
func storeAdvice(failure *Failure, advice *Advice) {
	mu.Lock()
	defer mu.Unlock()
	advices[failure.Key] = advice
	for len(advices) > maxAdvices {
		oldestKey := ""
		var oldest time.Time
		for key, item := range advices {
			if oldestKey == "" || item.At.Before(oldest) {
				oldestKey, oldest = key, item.At
			}
		}
		delete(advices, oldestKey)
	}
}
