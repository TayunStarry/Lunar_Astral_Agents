package BilibiliViewer

// run.go 评论区与动态接口：读取评论、发布动态、在自己的动态评论区留言。
//
// 主动发言只发生在自己的动态评论区；他人视频评论区只在被动收到 @/追评时由页面操作回应。

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// readCommentsAPI 在页面上下文读视频评论第一页（type=1）。
func (w *webSession) readCommentsAPI(aid int64, max int) ([]commentItem, error) {
	return w.readCommentsAPIPage(1, aid, 1, max, false)
}

// readCommentsAPIPage 读取指定对象评论的指定页（typ：1=视频，17=纯文字动态）。
// byTime 为 true 时按时间排序，用于发布后回读自己的评论。
func (w *webSession) readCommentsAPIPage(typ int, oid int64, page, ps int, byTime bool) ([]commentItem, error) {
	if typ <= 0 {
		typ = 1
	}
	if page < 1 {
		page = 1
	}
	if ps <= 0 || ps > 20 {
		ps = 20
	}
	sort := 2
	if byTime {
		sort = 0
	}
	target := fmt.Sprintf("https://api.bilibili.com/x/v2/reply?type=%d&oid=%d&sort=%d&ps=%d&pn=%d", typ, oid, sort, ps, page)
	text, err := w.pageFetchText("GET", target, "", "", 25*time.Second)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Replies []struct {
				Member struct {
					Uname string `json:"uname"`
				} `json:"member"`
				Content struct {
					Message string `json:"message"`
				} `json:"content"`
				Like int `json:"like"`
			} `json:"replies"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("评论接口 code=%d", resp.Code)
	}
	out := make([]commentItem, 0, len(resp.Data.Replies))
	for _, r := range resp.Data.Replies {
		if strings.TrimSpace(r.Content.Message) == "" {
			continue
		}
		out = append(out, commentItem{User: r.Member.Uname, Content: r.Content.Message, Like: r.Like})
	}
	return out, nil
}

// postCommentOnObject 在指定评论区发一条评论/回复（type：1=视频，11=图文动态，17=纯文字动态）。
// targets 非空时写入 at_name/at_mid，产生真实的 @ 通知。
func (w *webSession) postCommentOnObject(typ int, oid, message string, targets []MentionTarget) error {
	csrf := w.csrf()
	if csrf == "" {
		return fmt.Errorf("页面里读不到 bili_jct，无法发评论")
	}
	form := url.Values{}
	form.Set("oid", oid)
	form.Set("type", fmt.Sprintf("%d", typ))
	form.Set("message", message)
	form.Set("plat", "1")
	form.Set("csrf", csrf)
	if names, mids := mentionAtFields(targets); names != "" {
		form.Set("at_name", names)
		form.Set("at_mid", mids)
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
		return fmt.Errorf("B站拒绝: code=%d %s", resp.Code, resp.Msg)
	}
	return nil
}

// waitDynamicCommentAreaReady 等新动态的评论区真正可读写（刚发布时可能仍在审核/索引）。
func (w *webSession) waitDynamicCommentAreaReady(typ int, oidText string) error {
	oid, err := strconv.ParseInt(strings.TrimSpace(oidText), 10, 64)
	if err != nil || oid <= 0 {
		return fmt.Errorf("评论区 oid 无法解析: %q", oidText)
	}
	deadline := time.Now().Add(dynamicCommentReadyWait)
	var lastErr error
	for {
		if _, err := w.readCommentsAPIPage(typ, oid, 1, 20, false); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("等待 %s 仍不可评论（type=%d oid=%s）: %w", dynamicCommentReadyWait, typ, oidText, lastErr)
		}
		w.wait(dynamicCommentReadyPoll)
	}
}

// postDynamicCommentVerified 发布一条动态评论，并回读评论区确认是否真的出现。
//
// 接口报错时重试；接口已受理但回读未见时不再重发（避免重复评论），只再等一轮后如实报错。
func (w *webSession) postDynamicCommentVerified(typ int, oidText, message string, targets []MentionTarget) error {
	var lastErr error
	for attempt := 1; attempt <= dynamicCommentAttempts; attempt++ {
		if attempt > 1 {
			w.wait(dynamicCommentGap)
		}
		if err := w.postCommentOnObject(typ, oidText, message, targets); err != nil {
			lastErr = err
			continue
		}
		w.wait(dynamicCommentReadbackWait)
		if w.dynamicCommentVisible(typ, oidText, message) {
			return nil
		}
		w.wait(dynamicCommentReadbackWait)
		if w.dynamicCommentVisible(typ, oidText, message) {
			return nil
		}
		return fmt.Errorf("接口已受理但回读未见（可能仍在审核）")
	}
	return lastErr
}

// dynamicCommentVisible 回读动态评论区（按时间排序首页），确认刚发的评论是否已出现。
func (w *webSession) dynamicCommentVisible(typ int, oidText, message string) bool {
	oid, err := strconv.ParseInt(strings.TrimSpace(oidText), 10, 64)
	if err != nil || oid <= 0 {
		return false
	}
	fingerprint := commentFingerprint(message)
	if fingerprint == "" {
		return false
	}
	items, err := w.readCommentsAPIPage(typ, oid, 1, 20, true)
	if err != nil {
		return false
	}
	for _, item := range items {
		if strings.Contains(normalizeInline(item.Content), fingerprint) {
			return true
		}
	}
	return false
}

// commentFingerprint 取评论首行的前 24 个字作为回读指纹（视频评论即 URL）。
func commentFingerprint(message string) string {
	line := strings.TrimSpace(strings.SplitN(message, "\n", 2)[0])
	runes := []rune(line)
	if len(runes) > 24 {
		runes = runes[:24]
	}
	return normalizeInline(string(runes))
}

// normalizeInline 去掉空白，避免回读文本的换行/空格导致指纹对不上。
func normalizeInline(text string) string {
	return strings.Join(strings.Fields(text), "")
}

// publishDiary 把这一轮的观影感受作为纯文本动态发布（type=4）。
func (w *webSession) publishDiary(text string) (string, error) {
	csrf := w.csrf()
	if csrf == "" {
		return "", fmt.Errorf("页面里读不到 bili_jct，无法发动态")
	}
	clean := strings.TrimSpace(text)
	if clean == "" {
		return "", fmt.Errorf("动态正文为空")
	}
	fields := map[string]string{
		"dynamic_id":        "0",
		"type":              "4",
		"rid":               "0",
		"content":           clean,
		"up_choose_comment": "0",
		"up_close_comment":  "0",
		"extension":         `{"emoji_type":1,"from":{"emoji_type":1},"flag_cfg":{}}`,
		"csrf":              csrf,
		"csrf_token":        csrf,
	}
	raw, err := w.pageFetchForm("https://api.vc.bilibili.com/dynamic_svr/v1/dynamic_svr/create", fields, 30*time.Second)
	if err != nil {
		return "", err
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			DynamicID    int64  `json:"dynamic_id"`
			DynamicIDStr string `json:"dynamic_id_str"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return "", err
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("B站拒绝动态: code=%d %s", resp.Code, resp.Msg)
	}
	id := resp.Data.DynamicIDStr
	if id == "" && resp.Data.DynamicID != 0 {
		id = fmt.Sprintf("%d", resp.Data.DynamicID)
	}
	if id == "" {
		return "", fmt.Errorf("动态已提交但没有返回 dynamic_id")
	}
	return id, nil
}

// mentionAtFields 拆分呼叫目标为评论接口需要的 at_name / at_mid（逗号分隔）。
func mentionAtFields(targets []MentionTarget) (names, mids string) {
	nameList := make([]string, 0, len(targets))
	midList := make([]string, 0, len(targets))
	for _, target := range targets {
		if target.Name == "" || target.UID <= 0 {
			continue
		}
		nameList = append(nameList, target.Name)
		midList = append(midList, fmt.Sprintf("%d", target.UID))
	}
	return strings.Join(nameList, ","), strings.Join(midList, ",")
}

// finishComment 收束评论长度，但不从句子中间硬切。
//
// 评论宁可短，也要是完整的一句话：超长时退回到最近的一个句末标点收尾；
// 没有句末标点再退到逗号；只有在整段没有任何标点时才原样截断。
func finishComment(text string, maxRunes int) string {
	text = strings.TrimSpace(strings.Trim(text, `"'“”「」『』`))
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	cut := runes[:maxRunes]
	for i := len(cut) - 1; i >= 0; i-- {
		switch cut[i] {
		case '。', '！', '？', '!', '?', '…', '~', '～', '\n':
			return strings.TrimSpace(string(cut[:i+1]))
		}
	}
	for i := len(cut) - 1; i >= 0; i-- {
		switch cut[i] {
		case '，', '；', '、', ',', ';':
			return strings.TrimSpace(string(cut[:i+1]))
		}
	}
	return strings.TrimSpace(string(cut))
}
