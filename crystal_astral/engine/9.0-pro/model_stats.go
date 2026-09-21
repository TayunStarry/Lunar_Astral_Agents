package ltp9

// ==== LLM 调用记录（内存环形 + 落盘 {LocalDir}/log/ltp9_model_stats.json） ====
// self-contained 移植 YaraFlow internal/llm/stats.go；去掉了价格字段，保留记录/趋势/用量概览。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// statsPath 求调用记录落盘路径（基于 localBase）。
func statsPath() string {
	return filepath.Join(localBase(), "log", "ltp9_model_stats.json")
}

// ensureStatsStarted 惰性启动自动保存 goroutine（每 30s 冲刷一次脏数据）。
func ensureStatsStarted() {
	globalModelStats.mu.Lock()
	if globalModelStats.started {
		globalModelStats.mu.Unlock()
		return
	}
	globalModelStats.started = true
	globalModelStats.statsPath = statsPath()
	globalModelStats.load()
	globalModelStats.mu.Unlock()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			globalModelStats.saveIfDirty()
		}
	}()
}

// load 从磁盘加载历史记录（需持有 mu）。
func (s *ltmStatsCollector) load() {
	data, err := os.ReadFile(s.statsPath)
	if err != nil {
		return
	}
	var records []ltmCallRecord
	if json.Unmarshal(data, &records) != nil {
		return
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	filtered := make([]ltmCallRecord, 0, len(records))
	for _, r := range records {
		if r.Time.After(cutoff) {
			filtered = append(filtered, r)
		}
	}
	s.records = filtered
}

// save 保存记录到磁盘（需持有 mu）。
func (s *ltmStatsCollector) save() {
	dir := filepath.Dir(s.statsPath)
	_ = os.MkdirAll(dir, 0755)
	if data, err := json.Marshal(s.records); err == nil {
		_ = os.WriteFile(s.statsPath, data, 0644)
		s.dirty = false
	}
}

// saveIfDirty 仅在数据有变化时保存。
func (s *ltmStatsCollector) saveIfDirty() {
	s.mu.Lock()
	if s.dirty {
		s.save()
	}
	s.mu.Unlock()
}

// downstreamRecord 记录一次 LLM 调用。
func downstreamRecord(model, provider, taskType string, promptTokens, compTokens int, latencyMs int64, tokensPerSecond float64, success bool) {
	ensureStatsStarted()
	s := globalModelStats
	s.mu.Lock()
	s.records = append(s.records, ltmCallRecord{
		Time:            time.Now(),
		Model:           model,
		Provider:        provider,
		TaskType:        taskType,
		PromptTokens:    promptTokens,
		CompTokens:      compTokens,
		LatencyMs:       latencyMs,
		TokensPerSecond: tokensPerSecond,
		Success:         success,
	})
	if len(s.records) > s.maxSize {
		s.records = s.records[len(s.records)-s.maxSize:]
	}
	s.dirty = true
	go s.saveIfDirty()
	s.mu.Unlock()
}

// recentRecords 取最近 n 条调用记录（JS 友好格式）。
func recentRecords(n int) []any {
	ensureStatsStarted()
	s := globalModelStats
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n <= 0 || n > len(s.records) {
		n = len(s.records)
	}
	slice := s.records[len(s.records)-n:]
	out := make([]any, 0, len(slice))
	for _, r := range slice {
		out = append(out, map[string]any{
			"time":              r.Time.Format(time.RFC3339),
			"model":             r.Model,
			"provider":          r.Provider,
			"task_type":         r.TaskType,
			"prompt_tokens":     r.PromptTokens,
			"completion_tokens": r.CompTokens,
			"latency_ms":        r.LatencyMs,
			"tokens_per_second": r.TokensPerSecond,
			"success":           r.Success,
		})
	}
	return out
}

// saveModelStats 主动冲刷调用记录（供 Close 时调用）。
func saveModelStats() {
	globalModelStats.mu.Lock()
	defer globalModelStats.mu.Unlock()
	if globalModelStats.statsPath == "" {
		globalModelStats.statsPath = statsPath()
	}
	globalModelStats.save()
}