package bilibili

// inbox.go 消息中心接口：响应解包、字段映射与翻页。
//
// 自动响应要覆盖「所有 @ 我」的互动，只读首页会漏掉被挤下去的旧条目，
// 因此按上一页最后一条的 ID 作为游标向后翻；接口若忽略游标、返回的仍是首页，
// 末条 ID 不变会立即终止，不会死循环。

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// flexID 兼容 B 站接口里「数字或字符串」两种 ID 编码。
type flexID int64

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

// toInboxMessage 把消息中心条目归一化成可回应的条目。
// 通知分类属于业务判断，由调用方根据 Kind/Type/TargetID/RootID 计算。
func (m msgFeedItem) toInboxMessage(kind string) (InboxMessage, bool) {
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
		return InboxMessage{}, false
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
			return InboxMessage{}, false
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
		return InboxMessage{}, false
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

	return InboxMessage{
		Kind:      kind,
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

// ParseInboxPage 解析一页消息中心响应，并返回本页最后一条的原始 ID 作为翻页游标。
func ParseInboxPage(raw, kind string) ([]InboxMessage, string, error) {
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
	out := make([]InboxMessage, 0, len(resp.Data.Items))
	for _, entry := range resp.Data.Items {
		if item, ok := entry.toInboxMessage(kind); ok {
			out = append(out, item)
		}
	}
	cursor := ""
	if n := len(resp.Data.Items); n > 0 {
		cursor = resp.Data.Items[n-1].ID.String()
	}
	return out, cursor, nil
}

// ReadInboxKind 读取一类消息的指定页数（分页归本包）。
func ReadInboxKind(f Fetcher, kind string, maxPages int) ([]InboxMessage, error) {
	base := inboxReplyURL
	if kind == "at" {
		base = inboxAtURL
	}
	out := make([]InboxMessage, 0, 40)
	cursor := ""
	for page := 0; page < maxPages; page++ {
		target := base
		if cursor != "" {
			target += "&id=" + url.QueryEscape(cursor)
		}
		raw, err := f.FetchText("GET", target, "", "", inboxPageTimeout)
		if err != nil {
			return out, err
		}
		items, lastID, err := ParseInboxPage(raw, kind)
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
