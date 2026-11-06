package viewer

// inbox_item.go 消息条目归一化与页面 ID 还原。

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ==== 评论区类型与 oid ====

var (
	inboxDynamicOidRe = regexp.MustCompile(`(?:/opus/|/dynamic/|t\.bilibili\.com/|h\.bilibili\.com/ywh/|bilibili://(?:opus|dynamic|album)/)(\d+)`)
	inboxVideoAidRe   = regexp.MustCompile(`(?:bilibili://video/|/video/av)(\d+)`)
	inboxReplyIDRe    = regexp.MustCompile(`#reply(\d+)`)
)

// replyIDFromURI 从消息中心 URI 的 #replyNNN 片段提取对方评论 rpid。
func replyIDFromURI(uri string) string {
	if m := inboxReplyIDRe.FindStringSubmatch(uri); len(m) == 2 {
		return m[1]
	}
	return ""
}

func dynamicOidFromURI(uri string) string {
	if m := inboxDynamicOidRe.FindStringSubmatch(uri); len(m) == 2 {
		return m[1]
	}
	return ""
}

func videoAidFromURI(uri string) string {
	if m := inboxVideoAidRe.FindStringSubmatch(uri); len(m) == 2 {
		return m[1]
	}
	return ""
}

// toInboxItem 把消息中心条目归一化成可回应的 inboxItem。
func (m msgFeedItem) toInboxItem(kind string) (inboxItem, bool) {
	p := m.Item
	message := strings.TrimSpace(p.SourceContent)
	myContent := strings.TrimSpace(p.TargetReplyContent)
	if message == "" {
		message = strings.TrimSpace(p.TargetReplyContent)
	}
	if myContent == "" || myContent == message {
		myContent = strings.TrimSpace(p.RootReplyContent)
	}
	user := strings.TrimSpace(m.User.Nickname)
	if user == "" || message == "" {
		return inboxItem{}, false
	}

	typ := int(p.BusinessID)
	if typ < 1 || typ > 100 {
		typ = 0
	}
	if typ == 0 {
		switch {
		case strings.Contains(p.URI, "h.bilibili.com/ywh/"):
			typ = 11
		case strings.Contains(p.URI, "/opus/"),
			strings.Contains(p.URI, "/dynamic/"),
			strings.Contains(p.URI, "t.bilibili.com"),
			strings.Contains(p.URI, "bilibili://dynamic/"):
			typ = 17
		case strings.Contains(p.URI, "/read/cv"):
			typ = 12
		case strings.Contains(p.URI, "/video/"),
			strings.Contains(p.URI, "bilibili://video/"),
			strings.Contains(p.URI, "/bangumi/play/"):
			typ = 1
		case p.Business == "dynamic", p.Business == "opus":
			typ = 17
		case p.Business == "comment", p.Business == "video":
			typ = 1
		default:
			return inboxItem{}, false
		}
	}

	// 动态/图片动态的 subject_id 不是评论区 oid：优先用 URI 里的页面 ID
	oid := ""
	if typ == 11 || typ == 17 {
		oid = dynamicOidFromURI(p.URI)
	}
	if oid == "" {
		oid = p.SubjectID.String()
	}
	if oid == "" {
		oid = dynamicOidFromURI(p.URI)
	}
	if oid == "" {
		oid = videoAidFromURI(p.NativeURI)
	}
	if oid == "" {
		return inboxItem{}, false
	}

	venue := "dynamic"
	switch typ {
	case 1:
		venue = "video"
	case 12:
		venue = "article"
	}

	// @ 消息在部分场景只给 source_id，甚至有缺项：从 URI 的 #replyNNN 兜底
	sourceID := p.SourceID.String()
	if sourceID == "" {
		sourceID = replyIDFromURI(p.URI)
	}
	source := sourceID
	if source == "" || source == oid {
		source = p.TargetID.String()
	}
	if source == "" {
		source = replyIDFromURI(p.URI)
	}

	when := m.ReplyTime
	if when == 0 {
		when = p.ReplyTime
	}
	if when == 0 {
		when = m.AtTime
	}

	return inboxItem{
		Kind:      kind,
		Category:  classifyInbox(kind, typ, p.TargetID, p.RootID),
		Venue:     venue,
		Type:      typ,
		Oid:       oid,
		SourceID:  sourceID,
		RootID:    p.RootID.String(),
		TargetID:  p.TargetID.String(),
		MessageID: m.ID.String(),
		User:      user,
		Mid:       int64(m.User.Mid),
		Message:   message,
		MyContent: myContent,
		Title:     strings.TrimSpace(p.Title),
		URI:       strings.TrimSpace(p.URI),
		Time:      when,
	}, true
}

// parseMsgFeedPage 解析一页消息中心响应，并返回本页最后一条的原始 ID 作为翻页游标。
func parseMsgFeedPage(raw, kind string) ([]inboxItem, string, error) {
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Items []msgFeedItem `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, "", err
	}
	if resp.Code != 0 {
		return nil, "", fmt.Errorf("消息中心 code=%d %s", resp.Code, resp.Message)
	}
	out := make([]inboxItem, 0, len(resp.Data.Items))
	for _, entry := range resp.Data.Items {
		if item, ok := entry.toInboxItem(kind); ok {
			out = append(out, item)
		}
	}
	cursor := ""
	if n := len(resp.Data.Items); n > 0 {
		cursor = resp.Data.Items[n-1].ID.String()
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
