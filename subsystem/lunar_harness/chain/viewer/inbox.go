package viewer

// inbox.go 消息中心分类：把「回复我的」与「@我的」归到四类通知。
//
// 接口读取、响应解包与字段映射见 engine/bilibili；本文件只保留业务分类与去重键。

import "fmt"

// classifyInbox 把消息中心条目归到四类通知：
//  1. @我的；
//  2. 视频评论区里对我评论的追评（有目标评论）；
//  3. 我发布的视频下的新增根评论（没有目标评论）；
//  4. 动态/专栏的评论与互动。
func classifyInbox(kind string, typ int, targetID, rootID string) notifyCategory {
	if kind == "at" {
		return notifyAt
	}
	if typ == 1 {
		if targetID != "" || rootID != "" {
			return notifyVideoReply
		}
		return notifyVideoComment
	}
	return notifyDynamic
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
