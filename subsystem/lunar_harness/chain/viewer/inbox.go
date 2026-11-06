package viewer

// inbox.go 消息中心读取：把「回复我的」与「@我的」归一化成可回应的条目。
//
// 只负责读取与分类；发布回应见 reply_delivery.go。

import (
	"fmt"
	"strconv"
	"strings"
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
