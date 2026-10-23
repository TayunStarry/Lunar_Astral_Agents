package BilibiliLife

// events.go 生活事件：把「生活开始前 / 结束前」推到 LTP9 引擎域。
//
//  - before_life_starts：可取消本次生活；取消仍消耗一次本日额度，并计为完成一次生活，
//    避免插件与自动调度互相拉扯导致反复开浏览器；
//  - before_life_ends：纯触发通知（正常完成或中途失败），返回值不被使用，无取消/改写机能。
// 无插件订阅、未回传业务结果或回传结构非法时，一律按未取消处理。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	StarLTP "LunarSubsystem/LunarHarness/engine/plugin"
)

// dispatchBeforeLifeStarts 生活开始前事件：返回是否允许开始，以及插件给出的取消原因。
func dispatchBeforeLifeStarts(cfg Config, forced bool) (bool, string) {
	// 与 newRunner 的归一化保持一致：插件看到的是本轮真实生效的额度
	videos := cfg.VideosPerRound
	if videos <= 0 {
		videos = defaultVideosPerRound
	}
	videos = clampInt(videos, videoMinCount, videoMaxCount)
	slices := cfg.SlicesPerVideo
	if slices <= 0 {
		slices = defaultSlicesPerVideo
	}
	replies := cfg.RepliesPerRound
	if replies <= 0 {
		replies = defaultRepliesPerRound
	}
	res := StarLTP.Emit(StarLTP.EventBeforeLifeStarts, map[string]any{
		"forced":          forced,
		"videosPerRound":  videos,
		"slicesPerVideo":  slices,
		"repliesPerRound": replies,
		"dailyCount":      todayAutoCount(),
		"dailyLimit":      cfg.DailyLimit,
	}, "")
	feedback, ok := res.Return.(map[string]any)
	if !res.Returned || !ok {
		return true, ""
	}
	if cancel, _ := feedback["cancel"].(bool); cancel {
		reason, _ := feedback["reason"].(string)
		return false, reason
	}
	return true, ""
}

// dispatchBeforeLifeEnds 生活结束前事件（纯触发器：忽略返回值）。
// result 取值 finished（正常完成）/ failed（中途失败）；reason 仅失败时提供。
func dispatchBeforeLifeEnds(r *runner, result, reason string) {
	if r == nil || r.state == nil {
		return
	}
	payload := map[string]any{
		"result":        result,
		"forced":        r.state.Forced,
		"videosWatched": r.state.VideoIndex,
		"repliesUsed":   r.state.RepliesUsed,
	}
	if reason != "" {
		payload["reason"] = reason
	}
	StarLTP.Emit(StarLTP.EventBeforeLifeEnds, payload, "")
}

// todayAutoCount 返回今日已消耗的自动生活次数（跨日视为 0，不落盘）。
func todayAutoCount() int {
	stats := loadAutoStats()
	if stats.Date != time.Now().Format("2006-01-02") {
		return 0
	}
	return stats.Count
}

// consumeLifeQuota 消耗一次本日额度并记为本轮已完成：
// 取消发生在生活真正开始前，但按「算完成了一次生活」结算，避免立即重试。
func consumeLifeQuota(reason string) {
	stats := loadAutoStats()
	today := time.Now().Format("2006-01-02")
	if stats.Date != today {
		stats.Date = today
		stats.Count = 0
	}
	stats.Count++
	stats.LastAutoAt = time.Now().Format(time.RFC3339)
	saveAutoStats(stats)
	if reason == "" {
		reason = "插件未给出原因"
	}
	logger.Info("BilibiliLife", "生活开始前被 LTP9 插件取消（%s），已消耗一次本日额度并计为完成一次生活（今天第 %d 次）", reason, stats.Count)
}
