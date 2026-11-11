package wishchain

// linkage.go 既有机制接入心愿链路的统一入口。
//
// 观影选片、观影反馈、创作选题 / 延续、创作过程问题、观影列表变更等交互，
// 都通过 NoteLink 写入同一条联动记录并驱动心情偏移，保证月华的情感表达与
// 愿望体系使用同一套状态，不再各写各的。

import (
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// 联动类型。
const (
	// LinkViewerWatchlist 观影列表变更
	LinkViewerWatchlist = "观影列表变更"
	// LinkViewerFeedback 观影反馈
	LinkViewerFeedback = "观影反馈"
	// LinkViewerFinished 观影完成
	LinkViewerFinished = "观影完成"
	// LinkViewerRejected 观影未通过 / 失败
	LinkViewerRejected = "观影受挫"
	// LinkCreationTopic 创作选题 / 延续
	LinkCreationTopic = "创作选题"
	// LinkCreationDone 创作完成
	LinkCreationDone = "创作完成"
	// LinkCreationIssue 创作过程问题
	LinkCreationIssue = "创作问题"
	// LinkFailure 创作 / 观影失败：只产生类似逾期的失望，不改变愿望状态
	LinkFailure = "失败"
)

// NoteLink 记录一次机制联动，并按联动类型推动心情偏移。
func NoteLink(kind, summary string) {
	kind = strings.TrimSpace(kind)
	summary = strings.TrimSpace(summary)
	if kind == "" {
		return
	}
	mu.Lock()
	ensureLoadedLocked()
	state.Links = append(state.Links, LinkRecord{Kind: kind, Summary: summary, At: time.Now().Unix()})
	if len(state.Links) > linkLimit {
		state.Links = state.Links[len(state.Links)-linkLimit:]
	}
	saveLocked()
	mu.Unlock()

	if label, strength := linkMood(kind); label != "" {
		NudgeMood(label, strength)
	}
	logger.Info(Module, "[心愿] 联动「%s」：%s", kind, summary)
}

// RecentLinks 返回最近的联动记录副本。
func RecentLinks(limit int) []LinkRecord {
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	if limit <= 0 || limit > len(state.Links) {
		limit = len(state.Links)
	}
	out := make([]LinkRecord, limit)
	copy(out, state.Links[len(state.Links)-limit:])
	return out
}

// linkMood 联动类型 → 心情偏移方向与强度。
func linkMood(kind string) (string, float64) {
	switch kind {
	case LinkViewerWatchlist, LinkCreationTopic:
		return "好奇", 0.08
	case LinkViewerFeedback, LinkViewerFinished, LinkCreationDone:
		return "满足", 0.10
	case LinkViewerRejected, LinkCreationIssue:
		return "遗憾", 0.08
	case LinkFailure:
		return "失望", 0.20
	default:
		return "", 0
	}
}
