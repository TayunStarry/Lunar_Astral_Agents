package BilibiliViewer

// inbox.go 未读消息：B 站「回复我的」与「@我的」消息中心。
//
// 观影者每轮在进入选片前先把未读处理掉，覆盖三类触发场景：
//   - 视频评论区里对月华评论的回复与追评；
//   - 月华发布的动态（日记动态）下对她的动态或评论的回复；
//   - 直接 @月华 的评论或动态。
//
// 统一从消息中心接口取上下文，再按 B 站评论区类型回到原帖回复：
// 1=视频、11=图片动态、12=专栏、17=纯文字动态；oid 取 subject_id 或 uri 里的真实对象 ID。
// 已处理过的消息按 key 落盘去重，避免同一轮或下一轮重复打扰；消息中心偶尔失败
// 不阻断生活链，按下轮再试处理。

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// inboxMaxAge 只处理最近 7 天的未读：首次启用去重表时不至于把陈年消息全回一遍
	inboxMaxAge = 7 * 24 * time.Hour
)

// flexID 兼容 B 站接口里「数字或字符串」两种 ID 编码（rpid / dynamic_id 常见字符串）。
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

// msgFeedItem 消息中心「回复我的 / @我的」的原始条目（两个接口结构基本一致）。
type msgFeedItem struct {
	ID   flexID `json:"id"`
	User struct {
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

// inboxItem 归一化后的待回应消息：带上回复接口需要的 type/oid/root/parent。
type inboxItem struct {
	Kind      string `json:"kind"`  // reply / at
	Venue     string `json:"venue"` // video / dynamic / article
	Type      int    `json:"type"`  // B 站评论区类型：1=视频，11=图片动态，12=专栏，17=纯文字动态
	Oid       string `json:"oid"`
	Root      string `json:"root,omitempty"`
	Parent    string `json:"parent,omitempty"`
	SourceID  string `json:"source_id,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	User      string `json:"user"`
	Message   string `json:"message"`
	MyContent string `json:"my_content,omitempty"`
	Title     string `json:"title,omitempty"`
	URI       string `json:"uri,omitempty"`
	Time      int64  `json:"time,omitempty"`
}

// keys 返回去重键：消息中心 ID、被回复/被 @ 的目标评论 ID、来源评论 ID。
// 同一条互动可能同时出现在「回复我的」和「@我的」两个流里，必须靠这些键合并。
func (it inboxItem) keys() []string {
	venue := fmt.Sprintf("%d:%s", it.Type, it.Oid)
	keys := make([]string, 0, 3)
	if it.MessageID != "" {
		keys = append(keys, "msg:"+it.MessageID)
	}
	if it.Parent != "" {
		keys = append(keys, "reply:"+venue+":"+it.Parent)
	}
	if it.SourceID != "" {
		keys = append(keys, "source:"+venue+":"+it.SourceID)
	}
	return keys
}

// ==== 评论区类型与 oid ====
//
// 消息中心里 business_id 不是对象 ID，而是 B 站评论区类型代码（u8）：
// 1=视频、11=图片动态、12=专栏、17=纯文字动态。oid 要从 subject_id 或 uri 里取；
// 动态/图文动态优先用 uri 里的真实 opus/dynamic ID（subject_id 在新版 opus 下不一定可靠）。
var (
	inboxDynamicOidRe = regexp.MustCompile(`(?:/opus/|/dynamic/|t\.bilibili\.com/|h\.bilibili\.com/ywh/|bilibili://(?:opus|dynamic|album)/)(\d+)`)
	inboxVideoAidRe   = regexp.MustCompile(`(?:bilibili://video/|/video/av)(\d+)`)
)

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

// toInboxItem 把消息中心条目归一化成可回复的 inboxItem。
//
// 字段语义（消息中心，与 B 站消息页一致）：
//   - source_content：对方这次发来的内容（回复内容 / 含 @ 的内容）；
//   - target_reply_content：被回复的我的内容；
//   - source_id：对方这条评论/回复的 ID；
//   - target_id：被回复的我的评论 ID；root_id：根评论 ID；
//   - business_id：评论区类型代码（不是 oid）；subject_id：评论区对象 ID。
//
// 回复接口需要 type（评论区类型）+ oid（对象 ID）+ root（根评论）+ parent（被回复的评论）。
func (m msgFeedItem) toInboxItem(kind string) (inboxItem, bool) {
	p := m.Item
	message := strings.TrimSpace(p.SourceContent)
	myContent := strings.TrimSpace(p.TargetReplyContent)
	if message == "" {
		// 兼容个别接口版本把新内容放在 target_reply_content 的情况
		message = strings.TrimSpace(p.TargetReplyContent)
	}
	if myContent == "" || myContent == message {
		myContent = strings.TrimSpace(p.RootReplyContent)
	}
	user := strings.TrimSpace(m.User.Nickname)
	if user == "" || message == "" {
		return inboxItem{}, false
	}

	// business_id 是评论区类型代码（1/11/12/17...），这是最可靠的来源
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

	// oid：消息中心的 subject_id 就是评论接口要的对象 ID（视频=aid、动态=评论 rid）。
	// 新版 opus 的 uri 是「动态展示 ID」，和评论 oid 不是同一个值，不能拿来当 oid。
	oid := p.SubjectID.String()
	if oid == "" {
		// 兜底：老版动态的 URI 里就是评论 oid（t.bilibili.com/<动态id>）
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

	// parent 指向对方这条内容；root 指向会话根评论（我的评论是顶层时它就是根）
	parent := p.SourceID.String()
	if parent == "" {
		parent = p.TargetID.String()
	}
	root := p.RootID.String()
	if root == "" {
		root = p.TargetID.String()
	}
	if root == "" {
		root = parent
	}
	if parent == "" {
		parent = root
	}
	// 个别 @ 消息把「动态/视频本体 ID」放在 source_id：它不是评论，不能当 parent
	if parent == oid {
		parent = ""
	}
	if root == oid {
		root = ""
	}
	if parent == "" && root != "" {
		parent = root
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
		Venue:     venue,
		Type:      typ,
		Oid:       oid,
		Root:      root,
		Parent:    parent,
		SourceID:  p.SourceID.String(),
		MessageID: m.ID.String(),
		User:      user,
		Message:   message,
		MyContent: myContent,
		Title:     strings.TrimSpace(p.Title),
		URI:       strings.TrimSpace(p.URI),
		Time:      when,
	}, true
}

// parseMsgFeed 解析消息中心响应；code != 0 时返回错误（未登录时通常是 -101）。
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

	// 新消息优先；同一条互动可能同时出现在两个流里，按 keys 去重
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

// postInboxReply 按消息来源回到原帖回复：视频用 type=1，动态用 type=17。
func (w *webSession) postInboxReply(item inboxItem, message string) error {
	csrf := w.csrf()
	if csrf == "" {
		return fmt.Errorf("页面里读不到 bili_jct，无法回复未读消息")
	}
	form := url.Values{}
	form.Set("type", strconv.Itoa(item.Type))
	form.Set("oid", item.Oid)
	form.Set("message", message)
	form.Set("plat", "1")
	form.Set("csrf", csrf)
	if item.Root != "" {
		form.Set("root", item.Root)
	}
	if item.Parent != "" {
		form.Set("parent", item.Parent)
	}
	text, err := w.pageFetchText("POST", "https://api.bilibili.com/x/v2/reply/add",
		"application/x-www-form-urlencoded", form.Encode(), 25*time.Second)
	if err != nil {
		return err
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("B站拒绝回复: code=%d %s", resp.Code, resp.Msg)
	}
	return nil
}

// generateInboxReply 根据触发场景（视频评论追评 / 动态回复 / @我）生成一句回应。
func generateInboxReply(item inboxItem) (string, error) {
	scene := "视频评论区"
	if item.Type == 17 {
		scene = "动态评论区"
	}
	task := "正在回复 B 站" + scene + "里对自己评论的回复，只输出一句自然、友好的回应，1–60 字；不争论、不引战、不放链接/@。"
	if item.Kind == "at" {
		task = "有人在 B 站" + scene + "里 @ 了你，只输出一句自然、友好的回应，1–60 字；不争论、不引战、不放链接/@。"
	}
	system, err := lifeSystem(task)
	if err != nil {
		return "", err
	}
	var material strings.Builder
	fmt.Fprintf(&material, "对方：%s\n对方的消息：%s\n", item.User, item.Message)
	if item.MyContent != "" {
		fmt.Fprintf(&material, "我原来发的内容：%s\n", item.MyContent)
	}
	if item.Title != "" {
		fmt.Fprintf(&material, "所在位置：《%s》\n", item.Title)
	}
	text, err := chatText(system, material.String())
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "@", ""))
	if text == "" {
		return "", fmt.Errorf("回应为空")
	}
	return truncate(text, 60), nil
}

// inboxScene 互动发生的场景描述（写记忆用）
func inboxScene(item inboxItem) string {
	venue := "视频评论区"
	switch {
	case item.Venue == "dynamic" || item.Type == 17 || item.Type == 11:
		venue = "动态区"
	case item.Venue == "article" || item.Type == 12:
		venue = "专栏评论区"
	}
	if item.Title != "" {
		return "《" + item.Title + "》的" + venue
	}
	return venue
}

// inboxNote 互动类型补充（被 @ 还是追评）
func inboxNote(item inboxItem) string {
	if item.Kind == "at" {
		return "有人 @ 我"
	}
	return "回复我的评论"
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
