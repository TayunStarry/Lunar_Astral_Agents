package BilibiliViewer

// inbox_seen.go 未读消息的去重记录：落盘保存已回应的消息键，
// 避免同一轮或下一轮重复回复；动态即时回复（phaseReplies）也复用这份记录。

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
	// inboxSeenTTL 去重记录保留 30 天，过期后清理，避免文件无限增长
	inboxSeenTTL = 30 * 24 * time.Hour
	// inboxSeenMax 去重记录条数上限（超出时保留最新）
	inboxSeenMax = 2000
)

var (
	inboxSeenMu sync.Mutex
	// inboxSeenFile 去重记录路径；测试可替换
	inboxSeenFile = func() string {
		return filepath.Join(localRoot(), "database", "bilibili_life", "inbox.json")
	}
)

type inboxSeenStore struct {
	Keys map[string]int64 `json:"keys"`
}

func loadInboxSeen() inboxSeenStore {
	store := inboxSeenStore{Keys: map[string]int64{}}
	raw, err := os.ReadFile(inboxSeenFile())
	if err != nil {
		return store
	}
	_ = json.Unmarshal(raw, &store)
	if store.Keys == nil {
		store.Keys = map[string]int64{}
	}
	return store
}

func (s inboxSeenStore) has(keys []string) bool {
	for _, key := range keys {
		if _, ok := s.Keys[key]; ok {
			return true
		}
	}
	return false
}

func markInboxSeen(items ...inboxItem) {
	keys := make([]string, 0, len(items)*3)
	for _, item := range items {
		keys = append(keys, item.keys()...)
	}
	markInboxSeenKeys(keys)
}

// markInboxSeenKeys 记录已处理的消息键；动态即时回复（phaseReplies）也用它避免下轮重复。
func markInboxSeenKeys(keys []string) {
	if len(keys) == 0 {
		return
	}
	inboxSeenMu.Lock()
	defer inboxSeenMu.Unlock()
	store := loadInboxSeen()
	now := time.Now()
	for _, key := range keys {
		if key != "" {
			store.Keys[key] = now.Unix()
		}
	}
	pruneInboxSeen(&store, now)
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(inboxSeenFile()), 0o755); err != nil {
		logger.Warn("BilibiliViewer", "创建未读去重目录失败: %v", err)
		return
	}
	if err := os.WriteFile(inboxSeenFile(), raw, 0o644); err != nil {
		logger.Warn("BilibiliViewer", "保存未读消息去重记录失败: %v", err)
	}
}

func pruneInboxSeen(store *inboxSeenStore, now time.Time) {
	if store.Keys == nil {
		store.Keys = map[string]int64{}
		return
	}
	cutoff := now.Add(-inboxSeenTTL).Unix()
	for key, ts := range store.Keys {
		if ts < cutoff {
			delete(store.Keys, key)
		}
	}
	if len(store.Keys) <= inboxSeenMax {
		return
	}
	type entry struct {
		key string
		ts  int64
	}
	entries := make([]entry, 0, len(store.Keys))
	for key, ts := range store.Keys {
		entries = append(entries, entry{key: key, ts: ts})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ts > entries[j].ts })
	for _, e := range entries[inboxSeenMax:] {
		delete(store.Keys, e.key)
	}
}
