package viewer

// inbox_pages.go 消息中心的读取编排。
//
// 接口分页（游标推进、末页判定）在 engine/bilibili；
// 这里只负责两类消息的合并、排序与去重。

import (
	"sort"

	"LunarSubsystem/LunarHarness/engine/bilibili"
)

const (
	// inboxPageLimit 每类消息最多翻几页，控制单轮请求量。
	inboxPageLimit = 3
)

// fetchInboxAllItems 分页读取「回复我的」+「@我的」，合并去重后按时间倒序返回。
func (w *webSession) fetchInboxAllItems() ([]inboxItem, error) {
	var items []inboxItem
	var firstErr error
	for _, kind := range []string{"reply", "at"} {
		found, err := w.fetchInboxKind(kind, inboxPageLimit)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		items = append(items, found...)
	}
	if len(items) == 0 && firstErr != nil {
		return nil, firstErr
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Time > items[j].Time })
	return dedupeInboxItems(items), nil
}

// fetchInboxKind 读取一类消息的指定页数。
func (w *webSession) fetchInboxKind(kind string, maxPages int) ([]inboxItem, error) {
	messages, err := bilibili.ReadInboxKind(w, kind, maxPages)
	out := make([]inboxItem, 0, len(messages))
	for _, message := range messages {
		out = append(out, toInboxItem(message))
	}
	if err != nil {
		return out, err
	}
	return out, nil
}
