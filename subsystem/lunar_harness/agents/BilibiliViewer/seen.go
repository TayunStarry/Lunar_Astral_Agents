package BilibiliViewer

// seen.go 跨轮已看记录：避免连续几轮刷到同一条视频。
//
// 记录落在 local_data/database/bilibili_life/seen.json（bvid → 最近一次观看的 Unix 秒）。
// 保留 30 天、最多 500 条：短视频的推荐池变化快，太久以前的记录继续拦只会让候选越来越少。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

const (
	// seenRetentionDays 已看记录保留天数
	seenRetentionDays = 30
	// seenMaxEntries 已看记录条数上限（超出按最久未看淘汰）
	seenMaxEntries = 500
)

// seenStore 已看记录文件结构
type seenStore struct {
	Videos map[string]int64 `json:"videos"` // bvid → 最近一次观看的 Unix 秒
}

var seenMu sync.Mutex

func seenFile() string {
	return filepath.Join(localRoot(), "database", "bilibili_life", "seen.json")
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

// saveSeenLocked 写回已看记录（调用方需持有 seenMu）
func saveSeenLocked(store seenStore) {
	path := seenFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logger.Warn("BilibiliViewer", "创建已看记录目录失败: %v", err)
		return
	}
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		logger.Warn("BilibiliViewer", "写入已看记录失败: %v", err)
	}
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

// markSeen 记录一条视频已看（同时清理过期条目）
func markSeen(bvid string) {
	if bvid == "" {
		return
	}
	seenMu.Lock()
	defer seenMu.Unlock()
	store := loadSeenLocked()
	store.Videos[bvid] = time.Now().Unix()
	pruneSeen(&store, time.Now())
	saveSeenLocked(store)
}

// pruneSeen 删除超期条目；超过条数上限时按最近观看时间从旧到新淘汰
func pruneSeen(store *seenStore, now time.Time) {
	cutoff := now.Add(-seenRetentionDays * 24 * time.Hour).Unix()
	for bvid, ts := range store.Videos {
		if ts < cutoff {
			delete(store.Videos, bvid)
		}
	}
	if len(store.Videos) <= seenMaxEntries {
		return
	}
	type entry struct {
		bvid string
		ts   int64
	}
	entries := make([]entry, 0, len(store.Videos))
	for bvid, ts := range store.Videos {
		entries = append(entries, entry{bvid: bvid, ts: ts})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ts < entries[j].ts })
	for _, e := range entries[:len(entries)-seenMaxEntries] {
		delete(store.Videos, e.bvid)
	}
}
