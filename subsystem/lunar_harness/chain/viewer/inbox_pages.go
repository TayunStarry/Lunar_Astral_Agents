package viewer

// inbox_pages.go 消息中心的翻页读取。
//
// 自动响应要覆盖「所有 @ 我」的互动，只读首页会漏掉被挤下去的旧条目，
// 因此按上一页最后一条的 ID 作为游标向后翻；接口若忽略游标、返回的仍是首页，
// 末条 ID 不变会立即终止，不会死循环。

import (
	"net/url"
	"sort"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

const (
	// inboxPageLimit 每类消息最多翻几页，控制单轮请求量。
	inboxPageLimit = 3
	// inboxPageTimeout 单页请求超时。
	inboxPageTimeout = 25 * time.Second
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
	base := "https://api.bilibili.com/x/msgfeed/reply?platform=web&build=0&mobi_app=web"
	if kind == "at" {
		base = "https://api.bilibili.com/x/msgfeed/at?platform=web&build=0&mobi_app=web"
	}
	out := make([]inboxItem, 0, 40)
	cursor := ""
	for page := 0; page < maxPages; page++ {
		target := base
		if cursor != "" {
			target += "&id=" + url.QueryEscape(cursor)
		}
		raw, err := w.pageFetchText("GET", target, "", "", inboxPageTimeout)
		if err != nil {
			return out, err
		}
		items, lastID, err := parseMsgFeedPage(raw, kind)
		if err != nil {
			return out, err
		}
		out = append(out, items...)
		if lastID == "" || lastID == cursor || len(items) == 0 {
			break
		}
		cursor = lastID
	}
	logger.Info("BilibiliViewer", "消息中心 %s 读取 %d 条", kind, len(out))
	return out, nil
}
