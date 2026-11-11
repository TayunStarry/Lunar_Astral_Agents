package bilibili

// reply.go 评论发布接口与子评论回读。
//
// 消息中心已经给出 root_id / target_id / source_id，调用方负责推出评论接口需要的
// root 与 parent；本包只负责表单组装、签名（csrf）与响应解包。

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var csrfRe = regexp.MustCompile(`(?:^|;\s*)bili_jct=([^;]+)`)

// CSRFToken 从 document.cookie 里提取 bili_jct。
func CSRFToken(cookie string) string {
	m := csrfRe.FindStringSubmatch(cookie)
	if len(m) < 2 {
		return ""
	}
	value, _ := url.QueryUnescape(m[1])
	return value
}

// PostReply 调 x/v2/reply/add 发布评论/回复，是唯一的接口发布实现。
//
// Root/Parent 同时为空表示发布根评论；Targets 非空时写入 at_name/at_mid 产生真实 @。
func PostReply(p Page, req ReplyRequest) error {
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return fmt.Errorf("评论内容为空")
	}
	csrf := p.CSRFToken()
	if csrf == "" {
		return fmt.Errorf("页面里读不到 bili_jct，无法发布评论")
	}

	form := url.Values{}
	form.Set("type", strconv.Itoa(req.Type))
	form.Set("oid", req.Oid)
	form.Set("message", message)
	form.Set("plat", "1")
	form.Set("csrf", csrf)
	if req.Root != "" {
		form.Set("root", req.Root)
	}
	if req.Parent != "" {
		form.Set("parent", req.Parent)
	}
	if names, mids := mentionAtFields(req.Targets); names != "" {
		form.Set("at_name", names)
		form.Set("at_mid", mids)
	}

	raw, err := p.FetchText("POST", replyAddURL, "application/x-www-form-urlencoded", form.Encode(), replyAddTimeout)
	if err != nil {
		return err
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("B站拒绝发布: code=%d %s", resp.Code, resp.Msg)
	}
	return nil
}

// SubReplies 回读根评论下的子评论，供调用方判断回应是否真的可见。
func SubReplies(f Fetcher, typ int, oid, root string, ps int) ([]Comment, error) {
	if ps <= 0 {
		ps = replyVisiblePageSize
	}
	target := fmt.Sprintf("%s?type=%d&oid=%s&root=%s&ps=%d&pn=1",
		replyListURL, typ, url.QueryEscape(oid), url.QueryEscape(root), ps)
	raw, err := f.FetchText("GET", target, "", "", replyVisibleTimeout)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Replies []struct {
				Content struct {
					Message string `json:"message"`
				} `json:"content"`
			} `json:"replies"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("子评论接口 code=%d", resp.Code)
	}
	out := make([]Comment, 0, len(resp.Data.Replies))
	for _, reply := range resp.Data.Replies {
		out = append(out, Comment{Content: reply.Content.Message})
	}
	return out, nil
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
