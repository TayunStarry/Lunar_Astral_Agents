package viewer

// seen.go 跨轮已看记录：避免连续几轮刷到同一条视频。
//
// 记录落在 local_data/database/bilibili/seen.json（bvid → 最近一次观看的 Unix 秒）。
// 这里只做只读快照：新流程的已看判定由 viewerstore 的待看列表维护，
// 历史记录仍按 30 天保留期参与选片去重。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

const (
	// seenRetentionDays 已看记录保留天数
	seenRetentionDays = 30
)

// seenStore 已看记录文件结构
type seenStore struct {
	Videos map[string]int64 `json:"videos"` // bvid → 最近一次观看的 Unix 秒
}

var seenMu sync.Mutex

func seenFile() string {
	return filepath.Join(localRoot(), "database", "bilibili", "seen.json")
}

// loadSeenLocked 读取已看记录（文件不存在或损坏时返回空记录，调用方需持有 seenMu）
func loadSeenLocked() seenStore {
	store := seenStore{Videos: map[string]int64{}}
	raw, err := os.ReadFile(seenFile())
	if err != nil {
		return store
	}
	if err := json.Unmarshal(raw, &store); err != nil {
		logger.Warn("BilibiliViewer", "解析已看记录失败，按空记录处理: %v", err)
		return seenStore{Videos: map[string]int64{}}
	}
	if store.Videos == nil {
		store.Videos = map[string]int64{}
	}
	return store
}

// seenSnapshot 返回保留期内看过的 bvid 集合，供一次选片内的快速过滤。
// 一次调用只读一次文件，避免对每个候选都读盘。
func seenSnapshot() map[string]bool {
	seenMu.Lock()
	defer seenMu.Unlock()
	store := loadSeenLocked()
	cutoff := time.Now().Add(-seenRetentionDays * 24 * time.Hour).Unix()
	set := make(map[string]bool, len(store.Videos))
	for bvid, ts := range store.Videos {
		if ts >= cutoff {
			set[bvid] = true
		}
	}
	return set
}
