package BilibiliLife

// quota.go 每日刷视频调用额度：按自然日计数，跨日自动归零。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// dailyStats 单日刷视频调用统计
type dailyStats struct {
	Date       string `json:"date"`
	WatchCalls int    `json:"watch_calls"`
}

// dailyStatsFile 每日额度文件（与生活断点同目录）
func dailyStatsFile() string {
	return filepath.Join(localRoot(), "database", "bilibili_life", "daily.json")
}

func loadDailyStats() dailyStats {
	var stats dailyStats
	raw, err := os.ReadFile(dailyStatsFile())
	if err == nil {
		_ = json.Unmarshal(raw, &stats)
	}
	return stats
}

func saveDailyStats(stats dailyStats) {
	if err := os.MkdirAll(filepath.Dir(dailyStatsFile()), 0o755); err != nil {
		return
	}
	raw, _ := json.MarshalIndent(stats, "", "  ")
	_ = os.WriteFile(dailyStatsFile(), raw, 0o644)
}

// dailyWatchLimit 每日刷视频次数上限（配置非法时回退默认值）。
func dailyWatchLimit(cfg Config) int {
	if cfg.DailyLimit > 0 {
		return cfg.DailyLimit
	}
	return defaultDailyWatchLimit
}

// todayWatchCount 今日已调用刷视频的次数（跨日视为 0）。
func todayWatchCount() int {
	stats := loadDailyStats()
	if stats.Date != time.Now().Format("2006-01-02") {
		return 0
	}
	return stats.WatchCalls
}

// watchQuotaExhausted 今日刷视频次数是否已用完。
func watchQuotaExhausted(cfg Config) bool {
	stats := loadDailyStats()
	if stats.Date != time.Now().Format("2006-01-02") {
		return false
	}
	return stats.WatchCalls >= dailyWatchLimit(cfg)
}

// consumeWatchQuota 消耗一次今日刷视频额度（跨日自动重置计数）。
func consumeWatchQuota(_ Config) {
	stats := loadDailyStats()
	today := time.Now().Format("2006-01-02")
	if stats.Date != today {
		stats.Date = today
		stats.WatchCalls = 0
	}
	stats.WatchCalls++
	saveDailyStats(stats)
}

// WatchQuotaExhausted 供工具入口快速判断今天还能不能刷视频。
func WatchQuotaExhausted() bool {
	return watchQuotaExhausted(LoadConfig())
}
