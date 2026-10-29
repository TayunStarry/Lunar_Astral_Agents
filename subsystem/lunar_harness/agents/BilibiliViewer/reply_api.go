package BilibiliViewer

// reply_api.go 页面定位失败时的接口回复兜底。
//
// 消息中心已经给出 root_id / target_id / source_id，这里只做三件事：
//   - 推出评论接口需要的 root 与 parent（回复挂到对方那条评论下）；
//   - 调 x/v2/reply/add 发布；
//   - 回读根评论的子评论，确认回应真的可见，而不是只信接口 code=0。

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// replyIDs 从消息中心条目推出评论接口需要的 root / parent。
//
//	root：根评论 rpid（子回复挂在它下面）
//	parent：对方那条评论的 rpid
func replyIDs(item inboxItem) (root, parent string, ok bool) {
	parent = strings.TrimSpace(item.SourceID)
	root = strings.TrimSpace(item.RootID)
	if root == "" {
		if target := strings.TrimSpace(item.TargetID); target != "" {
			root = target
		} else {
			root = parent
		}
	}
	if parent == "" {
		parent = strings.TrimSpace(item.TargetID)
	}
	if parent == "" {
		parent = root
	}
	// @ 消息可能只有对象 ID（没有 root/target/source）：按根评论发布，
	// 只要 oid 可用就不应判定为「缺 ID」而放弃回应。
	if strings.TrimSpace(item.Oid) == "" {
		return "", "", false
	}
	return root, parent, true
}

// replyByAPI 用消息中心的评论 ID 发布回复，并回读确认可见。
func (w *webSession) replyByAPI(item inboxItem, message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		return fmt.Errorf("回应内容为空")
	}
	root, parent, ok := replyIDs(item)
	if !ok {
		return fmt.Errorf("消息缺少对象或评论 ID，无法接口回复")
	}
	csrf := w.csrf()
	if csrf == "" {
		return fmt.Errorf("页面里读不到 bili_jct，无法回复")
	}

	form := url.Values{}
	form.Set("type", strconv.Itoa(item.Type))
	form.Set("oid", item.Oid)
	form.Set("message", message)
	form.Set("plat", "1")
	form.Set("csrf", csrf)
	// 有 root/parent 时挂到对方评论下；两者为空表示发布一条新的根评论
	if root != "" {
		form.Set("root", root)
	}
	if parent != "" {
		form.Set("parent", parent)
	}

	raw, err := w.pageFetchText("POST", "https://api.bilibili.com/x/v2/reply/add",
		"application/x-www-form-urlencoded", form.Encode(), 25*time.Second)
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
		return fmt.Errorf("B站拒绝回复: code=%d %s", resp.Code, resp.Msg)
	}

	// 只有挂在根评论下才需要回读子评论；根评论发布以接口 code=0 为准
	if root != "" {
		w.wait(3 * time.Second)
		if !w.replyVisible(item, root, message) {
			return fmt.Errorf("接口已受理但回读未见（可能仍在审核）")
		}
	}
	return nil
}

// replyVisible 回读根评论下的子评论，确认刚才的回应已经出现。
func (w *webSession) replyVisible(item inboxItem, root, message string) bool {
	fingerprint := commentFingerprint(message)
	if fingerprint == "" {
		return false
	}
	target := fmt.Sprintf(
		"https://api.bilibili.com/x/v2/reply/reply?type=%d&oid=%s&root=%s&ps=20&pn=1",
		item.Type, url.QueryEscape(item.Oid), url.QueryEscape(root))
	raw, err := w.pageFetchText("GET", target, "", "", 20*time.Second)
	if err != nil {
		return false
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
	if err := json.Unmarshal([]byte(raw), &resp); err != nil || resp.Code != 0 {
		return false
	}
	for _, reply := range resp.Data.Replies {
		if strings.Contains(normalizeInline(reply.Content.Message), fingerprint) {
			return true
		}
	}
	return false
}
