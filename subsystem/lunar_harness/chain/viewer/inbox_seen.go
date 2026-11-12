package viewer

// inbox_seen.go 未读消息的去重记录：已处理的消息键落在 viewer.db 的 inbox_seen 表，
// 避免同一轮或下一轮重复回复。
//
// 表结构、事务与 TTL / 条数截断的落库实现见 engine/viewerstore；
// 保留期与上限仍由本链定标。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

const (
	// inboxSeenTTL 去重记录保留 30 天，过期后清理，避免表无限增长
	inboxSeenTTL = 30 * 24 * time.Hour
	// inboxSeenMax 去重记录条数上限（超出时保留最新）
	inboxSeenMax = 2000
)

// loadInboxSeen 读取去重键快照（读取失败按空记录处理，不阻断本轮回应）。
func loadInboxSeen() inboxSeenStore {
	store := inboxSeenStore{Keys: map[string]int64{}}
	keys, err := viewerstore.LoadInboxSeenKeys()
	if err != nil {
		logger.Warn("BilibiliViewer", "读取未读消息去重记录失败，按空记录处理: %v", err)
		return store
	}
	store.Keys = keys
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

// markInboxSeenKeys 记录已处理的消息键（同一条互动可能命中多个去重键）。
func markInboxSeenKeys(keys []string) {
	if len(keys) == 0 {
		return
	}
	now := time.Now()
	if err := viewerstore.MarkInboxSeenKeys(keys, now.Unix(), now.Add(-inboxSeenTTL).Unix(), inboxSeenMax); err != nil {
		logger.Warn("BilibiliViewer", "保存未读消息去重记录失败: %v", err)
	}
}
