package viewer

// inbox_item.go 消息条目到本链模型的转换与批量去重。
//
// 归一化（业务类型推断、oid 还原、ID 兜底）在 engine/bilibili；
// 这里只补通知分类，并保留自动响应需要的去重与过期过滤。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/bilibili"
)

// toInboxItem 把引擎归一化后的消息转换成可回应的 inboxItem，并补上通知分类。
func toInboxItem(m bilibili.InboxMessage) inboxItem {
	return inboxItem{
		Kind:      m.Kind,
		Category:  classifyInbox(m.Kind, m.Type, m.TargetID, m.RootID),
		Venue:     m.Venue,
		Type:      m.Type,
		Oid:       m.Oid,
		SourceID:  m.SourceID,
		RootID:    m.RootID,
		TargetID:  m.TargetID,
		MessageID: m.MessageID,
		User:      m.User,
		Mid:       m.Mid,
		Message:   m.Message,
		MyContent: m.MyContent,
		Title:     m.Title,
		URI:       m.URI,
		Time:      m.Time,
	}
}

// parseMsgFeedPage 解析一页消息中心响应，并返回本页最后一条的原始 ID 作为翻页游标。
func parseMsgFeedPage(raw, kind string) ([]inboxItem, string, error) {
	messages, cursor, err := bilibili.ParseInboxPage(raw, kind)
	if err != nil {
		return nil, "", err
	}
	out := make([]inboxItem, 0, len(messages))
	for _, message := range messages {
		out = append(out, toInboxItem(message))
	}
	return out, cursor, nil
}

// dedupeInboxItems 按消息 ID 与来源去重，保留先出现的条目。
func dedupeInboxItems(items []inboxItem) []inboxItem {
	seen := make(map[string]bool, len(items)*2)
	out := items[:0]
	for _, item := range items {
		dup := false
		for _, key := range item.keys() {
			if seen[key] {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		for _, key := range item.keys() {
			seen[key] = true
		}
		out = append(out, item)
	}
	return out
}

// filterUnreadInbox 过滤过期与已处理消息，并合并同一批里的重复项。
func filterUnreadInbox(items []inboxItem, seen inboxSeenStore, now time.Time) []inboxItem {
	out := make([]inboxItem, 0, len(items))
	batch := make(map[string]bool, len(items)*2)
	for _, item := range items {
		if item.Time > 0 && now.Sub(time.Unix(item.Time, 0)) > inboxMaxAge {
			continue
		}
		keys := item.keys()
		if seen.has(keys) {
			continue
		}
		dup := false
		for _, key := range keys {
			if batch[key] {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		for _, key := range keys {
			batch[key] = true
		}
		out = append(out, item)
	}
	return out
}
