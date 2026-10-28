package BilibiliViewer

// events.go 哔哩观影事件：把「开始前 / 结束前」推到 LTP9 引擎域。
//
//   - before_life_starts：可取消本次观影；取消仍消耗一次本日额度；
//   - before_life_ends：纯触发通知（正常完成或中途失败），返回值不被使用。

import (
	StarLTP "LunarSubsystem/LunarHarness/engine/plugin"
)

// dispatchBeforeLifeStarts 观影开始前事件：返回是否允许开始，以及插件给出的取消原因。
func dispatchBeforeLifeStarts(cfg Config) (bool, string) {
	replies := cfg.RepliesPerRound
	if replies <= 0 {
		replies = defaultRepliesPerRound
	}
	res := StarLTP.Emit(StarLTP.EventBeforeLifeStarts, map[string]any{
		"forced":          false,
		"candidates":      candidateMax,
		"slicesPerVideo":  watchSliceDefault,
		"repliesPerRound": replies,
		"dailyCount":      todayWatchCount(),
		"dailyLimit":      dailyWatchLimit(cfg),
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

// dispatchBeforeLifeEnds 观影结束前事件（纯触发器：忽略返回值）。
func dispatchBeforeLifeEnds(r *runner, result, reason string) {
	if r == nil {
		return
	}
	payload := map[string]any{
		"result":        result,
		"videosWatched": len(r.approved),
		"videosSampled": len(r.videos),
		"repliesUsed":   r.replied,
		"dynamic":       r.dynID != "",
	}
	if reason != "" {
		payload["reason"] = reason
	}
	StarLTP.Emit(StarLTP.EventBeforeLifeEnds, payload, "")
}
