package BilibiliViewer

// inbox.go 消息中心读取：把「回复我的」与「@我的」归一化成可回应的条目。
//
// 只负责读取与分类；发布回应由 notify.go 通过页面操作完成。

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// flexID 兼容 B 站接口里「数字或字符串」两种 ID 编码。
type flexID int64

// classifyInbox 把消息中心条目归到四类通知：
//  1. @我的；
//  2. 视频评论区里对我评论的追评（有目标评论）；
//  3. 我发布的视频下的新增根评论（没有目标评论）；
//  4. 动态/专栏的评论与互动。
func classifyInbox(kind string, typ int, targetID, rootID flexID) notifyCategory {
	if kind == "at" {
		return notifyAt
	}
	if typ == 1 {
		if targetID != 0 || rootID != 0 {
			return notifyVideoReply
		}
		return notifyVideoComment
	}
	return notifyDynamic
}

func (f *flexID) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(string(raw), `"`)
	if text == "" || text == "null" {
		*f = 0
		return nil
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexID(value)
	return nil
}

func (f flexID) String() string {
	if f == 0 {
		return ""
	}
	return strconv.FormatInt(int64(f), 10)
}

// msgFeedItem 消息中心「回复我的 / @我的」的原始条目。
type msgFeedItem struct {
	ID   flexID `json:"id"`
	User struct {
		Mid      flexID `json:"mid"`
		Nickname string `json:"nickname"`
	} `json:"user"`
	ReplyTime int64 `json:"reply_time"`
	AtTime    int64 `json:"at_time"`
	Item      struct {
		Type               string `json:"type"`
		Business           string `json:"business"`
		BusinessID         flexID `json:"business_id"`
		SubjectID          flexID `json:"subject_id"`
		RootID             flexID `json:"root_id"`
		TargetID           flexID `json:"target_id"`
		SourceID           flexID `json:"source_id"`
		TargetReplyContent string `json:"target_reply_content"`
		SourceContent      string `json:"source_content"`
		RootReplyContent   string `json:"root_reply_content"`
		Title              string `json:"title"`
		URI                string `json:"uri"`
		NativeURI          string `json:"native_uri"`
		ReplyTime          int64  `json:"reply_time"`
	} `json:"item"`
}

// keys 返回去重键：消息中心 ID 与对方评论 ID。
func (it inboxItem) keys() []string {
	venue := fmt.Sprintf("%d:%s", it.Type, it.Oid)
	keys := make([]string, 0, 2)
	if it.MessageID != "" {
		keys = append(keys, "msg:"+it.MessageID)
	}
	if it.SourceID != "" {
		keys = append(keys, "source:"+venue+":"+it.SourceID)
	}
	return keys
}

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

// parseMsgFeed 解析消息中心响应；code != 0 时返回错误。
func parseMsgFeed(raw, kind string) ([]inboxItem, error) {
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Items []msgFeedItem `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("消息中心 code=%d %s", resp.Code, resp.Message)
	}
	out := make([]inboxItem, 0, len(resp.Data.Items))
	for _, entry := range resp.Data.Items {
		if item, ok := entry.toInboxItem(kind); ok {
			out = append(out, item)
		}
	}
	return out, nil
}

// fetchInboxItems 读取「回复我的」+「@我的」，合并去重后按时间倒序返回。
func (w *webSession) fetchInboxItems() ([]inboxItem, error) {
	const (
		replyURL = "https://api.bilibili.com/x/msgfeed/reply?platform=web&build=0&mobi_app=web"
		atURL    = "https://api.bilibili.com/x/msgfeed/at?platform=web&build=0&mobi_app=web"
	)
	var items []inboxItem
	var firstErr error
	if raw, err := w.pageFetchText("GET", replyURL, "", "", 25*time.Second); err != nil {
		firstErr = err
	} else if parsed, err := parseMsgFeed(raw, "reply"); err != nil {
		firstErr = err
	} else {
		items = append(items, parsed...)
	}
	if raw, err := w.pageFetchText("GET", atURL, "", "", 25*time.Second); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	} else if parsed, err := parseMsgFeed(raw, "at"); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	} else {
		items = append(items, parsed...)
	}
	if len(items) == 0 && firstErr != nil {
		return nil, firstErr
	}

	sort.SliceStable(items, func(i, j int) bool { return items[i].Time > items[j].Time })
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
	return out, nil
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

// jsonUnmarshal 字符串版 JSON 解码（页面接口返回的都是字符串）。
func jsonUnmarshal(raw string, out any) error {
	return json.Unmarshal([]byte(raw), out)
}
