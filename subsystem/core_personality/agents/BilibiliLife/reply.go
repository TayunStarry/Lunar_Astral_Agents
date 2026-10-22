package BilibiliLife

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type replyTarget struct {
	Rpid    string `json:"rpid"`
	Root    string `json:"root"`
	Parent  string `json:"parent"`
	User    string `json:"user"`
	Message string `json:"message"`
}

// fetchDynamicReplies 只读取动态评论，供可中断状态机逐条回复。
func (w *webSession) fetchDynamicReplies(dynID string) ([]replyTarget, error) {
	listURL := fmt.Sprintf("https://api.bilibili.com/x/v2/reply?type=17&oid=%s&sort=2&ps=20", url.QueryEscape(dynID))
	raw, err := w.pageFetchText("GET", listURL, "", "", 25*time.Second)
	if err != nil {
		return nil, err
	}
	var list struct {
		Code int `json:"code"`
		Data struct {
			Replies []struct {
				Rpid      int64  `json:"rpid"`
				RpidStr   string `json:"rpid_str"`
				Root      int64  `json:"root"`
				RootStr   string `json:"root_str"`
				Parent    int64  `json:"parent"`
				ParentStr string `json:"parent_str"`
				Member    struct {
					Uname string `json:"uname"`
				} `json:"member"`
				Content struct {
					Message string `json:"message"`
				} `json:"content"`
			} `json:"replies"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	if list.Code == -404 {
		return nil, nil
	}
	if list.Code != 0 {
		return nil, fmt.Errorf("动态评论接口 code=%d", list.Code)
	}
	out := make([]replyTarget, 0, len(list.Data.Replies))
	for _, r := range list.Data.Replies {
		if strings.TrimSpace(r.Content.Message) == "" {
			continue
		}
		rpid := r.RpidStr
		if rpid == "" && r.Rpid != 0 {
			rpid = fmt.Sprintf("%d", r.Rpid)
		}
		root := r.RootStr
		if root == "" && r.Root != 0 {
			root = fmt.Sprintf("%d", r.Root)
		}
		parent := r.ParentStr
		if parent == "" && r.Parent != 0 {
			parent = fmt.Sprintf("%d", r.Parent)
		}
		if root == "" {
			root = rpid
		}
		if parent == "" {
			parent = rpid
		}
		out = append(out, replyTarget{Rpid: rpid, Root: root, Parent: parent, User: r.Member.Uname, Message: r.Content.Message})
	}
	return out, nil
}

// postDynamicReply 发布一条动态评论回复。
func (w *webSession) postDynamicReply(dynID string, target replyTarget, message string) error {
	csrf := w.csrf()
	if csrf == "" {
		return fmt.Errorf("页面里读不到 bili_jct，无法回复动态")
	}
	form := url.Values{}
	form.Set("type", "17")
	form.Set("oid", dynID)
	form.Set("message", message)
	form.Set("plat", "1")
	form.Set("csrf", csrf)
	form.Set("root", target.Root)
	form.Set("parent", target.Parent)
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
