package LifeMemorizer

// policy.go 记忆整理的修复策略：可临时下调的正文保留下限、不可合并簇标记与复跑验证。
//
// 这些策略只影响整理行为，不改写任何已存记忆；「不可合并」的簇由链路层保留原条目。

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

// effectiveMinRetain 返回当前生效的正文保留下限（临时策略到期自动失效）。
func effectiveMinRetain() int {
	policyMu.Lock()
	defer policyMu.Unlock()
	if retainOverride > 0 && time.Now().Before(retainUntil) {
		return retainOverride
	}
	retainOverride = 0
	retainUntil = time.Time{}
	return MinRetainPercent
}

// setRetainOverride 临时下调正文保留下限，返回生效值与到期时间。
func setRetainOverride(percent int, ttl time.Duration) (int, time.Time) {
	if percent < retainPercentFloor {
		percent = retainPercentFloor
	}
	if percent > MinRetainPercent {
		percent = MinRetainPercent
	}
	if ttl <= 0 {
		ttl = policyDefaultTTL
	}
	if ttl > policyMaxTTL {
		ttl = policyMaxTTL
	}
	until := time.Now().Add(ttl)
	policyMu.Lock()
	retainOverride = percent
	retainUntil = until
	policyMu.Unlock()
	return percent, until
}

// contentFingerprint 计算单条记忆的稳定指纹（归一空白后哈希）。
//
// 按条目而不是按整批记录：相似簇会随着新记忆加入而变化，整批指纹会失效，
// 以条目为键才能让「该簇不可合并」在后续批次里继续命中。
func contentFingerprint(content string) string {
	text := strings.Join(strings.Fields(content), " ")
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(text))
	return fmt.Sprintf("%x", hash.Sum64())
}

// isClusterSkipped 判断该批记忆里是否有条目处于「不可合并」有效期。
func isClusterSkipped(contents []string) bool {
	now := time.Now()
	policyMu.Lock()
	defer policyMu.Unlock()
	for key, until := range skippedClusters {
		if now.After(until) {
			delete(skippedClusters, key)
		}
	}
	for _, content := range contents {
		if until, ok := skippedClusters[contentFingerprint(content)]; ok && now.Before(until) {
			return true
		}
	}
	return false
}

// markClusterUnmergeable 把该批记忆的条目都记为不可合并，返回标记条数与到期时间。
func markClusterUnmergeable(contents []string, ttl time.Duration) (int, time.Time) {
	if ttl <= 0 {
		ttl = policyDefaultTTL
	}
	if ttl > policyMaxTTL {
		ttl = policyMaxTTL
	}
	until := time.Now().Add(ttl)
	policyMu.Lock()
	marked := 0
	for _, content := range contents {
		if strings.TrimSpace(content) == "" {
			continue
		}
		skippedClusters[contentFingerprint(content)] = until
		marked++
	}
	for len(skippedClusters) > maxSkippedClusters {
		oldestKey := ""
		var oldest time.Time
		for key, deadline := range skippedClusters {
			if oldestKey == "" || deadline.Before(oldest) {
				oldestKey, oldest = key, deadline
			}
		}
		delete(skippedClusters, oldestKey)
	}
	policyMu.Unlock()
	return marked, until
}

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

// policySnapshot 返回修复策略的当前状态。
func policySnapshot() map[string]any {
	policyMu.Lock()
	defer policyMu.Unlock()
	now := time.Now()
	active := retainOverride > 0 && now.Before(retainUntil)
	effective := MinRetainPercent
	overrideUntil := ""
	if active {
		effective = retainOverride
		overrideUntil = retainUntil.Format(time.RFC3339)
	}
	skipped := 0
	for _, until := range skippedClusters {
		if now.Before(until) {
			skipped++
		}
	}
	return map[string]any{
		"default_min_retain_percent":   MinRetainPercent,
		"effective_min_retain_percent": effective,
		"override_active":              active,
		"override_until":               overrideUntil,
		"skipped_clusters":             skipped,
		"merge_min_retain_percent":     MergeMinRetainPercent,
		"merge_min_runes":              MergeMinRunes,
	}
}

// resetRepairState 清空修复策略、最近记录与复跑输入；reset 动作的明确范围。
func resetRepairState() string {
	policyMu.Lock()
	retainOverride = 0
	retainUntil = time.Time{}
	skippedClusters = map[string]time.Time{}
	policyMu.Unlock()
	inputsMu.Lock()
	lastInputs = nil
	inputsMu.Unlock()
	return resetLastRun() + "，并恢复默认阈值、清除不可合并标记与复跑输入"
}

// verifyPolicy 复跑最近一次失败合并，确认修复是否真的生效。
func verifyPolicy(ctx context.Context, params map[string]any) (any, error) {
	inputs := snapshotInputs()
	if len(inputs) == 0 {
		return map[string]any{
			"verified": false,
			"mode":     "rerun",
			"reason":   "没有保留上次合并输入，无法复跑；请读取运行态确认策略已生效",
		}, nil
	}
	if isClusterSkipped(inputs) {
		return map[string]any{
			"verified":    true,
			"mode":        "skip",
			"reason":      "该记忆簇已标记为不可合并，下次直接保留原条目，不再触发合并判定",
			"input_items": len(inputs),
			"thresholds":  mergeThresholds(inputs),
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
