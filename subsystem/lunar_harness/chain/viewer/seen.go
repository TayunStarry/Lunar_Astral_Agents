package viewer

// seen.go 跨轮已看记录：避免连续几轮刷到同一条视频。
//
// 历史记录落在 viewer.db 的 seen_video 表（bvid → 最近一次观看的 Unix 秒）。
// 这里只做只读快照：新流程的已看判定由 viewerstore 的待看列表维护，
// 历史记录仍按 30 天保留期参与选片去重。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

const (
	// seenRetentionDays 已看记录保留天数
	seenRetentionDays = 30
)

// seenSnapshot 返回保留期内看过的 bvid 集合，供一次选片内的快速过滤。
// 一次调用只查一次库，避免对每个候选都读盘。
func seenSnapshot() map[string]bool {
	cutoff := time.Now().Add(-seenRetentionDays * 24 * time.Hour).Unix()
	set, err := viewerstore.LoadSeenVideosSince(cutoff)
	if err != nil {
		logger.Warn("BilibiliViewer", "读取已看记录失败，按空记录处理: %v", err)
		return map[string]bool{}
	}
	return set
}
