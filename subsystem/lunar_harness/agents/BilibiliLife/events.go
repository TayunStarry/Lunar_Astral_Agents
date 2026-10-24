package BilibiliLife

// events.go 观影者事件：把「开始前 / 结束前」推到 LTP9 引擎域。
//
//  - before_life_starts：可取消本次观影；取消仍消耗一次本日刷视频额度，避免插件与调用方反复拉扯；
//  - before_life_ends：纯触发通知（正常完成或中途失败），返回值不被使用，无取消/改写机能。
// 无插件订阅、未回传业务结果或回传结构非法时，一律按未取消处理。

import (
	StarLTP "LunarSubsystem/LunarHarness/engine/plugin"
)

// dispatchBeforeLifeStarts 观影开始前事件：返回是否允许开始，以及插件给出的取消原因。
func dispatchBeforeLifeStarts(cfg Config, flow Flow) (bool, string) {
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
		"flow":            string(flow),
		"forced":          false,
		"videosPerRound":  videos,
		"slicesPerVideo":  slices,
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
// result 取值 finished（正常完成）/ failed（中途失败）；reason 仅失败时提供。
func dispatchBeforeLifeEnds(r *runner, result, reason string) {
	if r == nil || r.state == nil {
		return
	}
	payload := map[string]any{
		"result":        result,
		"flow":          string(r.state.Flow),
		"videosWatched": r.state.VideoIndex,
		"repliesUsed":   r.state.RepliesUsed,
	}
	if reason != "" {
		payload["reason"] = reason
	}
	StarLTP.Emit(StarLTP.EventBeforeLifeEnds, payload, "")
}
