package DialogueBridging

// 消息解析与单线程收发流程（按职责拆分）

import (
	"LunarSubsystem/LunarHarness/clients/napcat"
	"encoding/json"
	"strconv"
)

func resolveNickname(userID int64, sender napcat.Sender) string {
	if sender.Nickname != "" {
		return sender.Nickname
	}
	if sender.Card != "" {
		return sender.Card
	}
	if name, err := napcat.GetStrangerNickname(userID); err == nil && name != "" {
		return name
	}
	return strconv.FormatInt(userID, 10)
}

// resolveMemberName 解析群聊发言用户昵称：群名片优先，昵称兜底
func resolveMemberName(sender napcat.Sender) string {
	if sender.Card != "" {
		return sender.Card
	}
	return sender.Nickname
}

// containsAtSelf 判断消息段中是否 @了当前账号
func containsAtSelf(segments []napcat.MessageSegment, selfID int64) bool {
	selfStr := strconv.FormatInt(selfID, 10)
	for _, segment := range segments {
		if segment.Type != "at" {
			continue
		}
		var atData napcat.AtData
		if json.Unmarshal(segment.Data, &atData) == nil && atData.QQ == selfStr {
			return true
		}
	}
	return false
}

// directAtOther 判断消息是否直接 @ 了群里除月华之外的其他具体成员（A@B 私聊语义）。
// @全体成员（qq=all / 0）面向全群广播，不算两人私聊；仅检查顶层消息段，
// 合并转发与回复引用内部的 @ 由渲染层呈现，不参与该判定。
func directAtOther(segments []napcat.MessageSegment, selfStr string) bool {
	for _, segment := range segments {
		if segment.Type != "at" {
			continue
		}
		var atData napcat.AtData
		if json.Unmarshal(segment.Data, &atData) != nil {
			continue
		}
		if atData.QQ == selfStr || atData.QQ == "" || atData.QQ == "all" || atData.QQ == "0" {
			continue
		}
		return true
	}
	return false
}

// replyQuotesSelf 判断回复引用的原始消息是否为月华自己所发：
// 「引用月华的话再发言」在语意上仍是对月华说话，应当转给月华。
func replyQuotesSelf(segments []napcat.MessageSegment, selfID int64) bool {
	for _, segment := range segments {
		if segment.Type != "reply" {
			continue
		}
		var replyData napcat.ReplyData
		if json.Unmarshal(segment.Data, &replyData) != nil {
			continue
		}
		id := rawIDString(replyData.ID)
		if id == "" {
			continue
		}
		detail, err := napcat.GetMessageDetail(id)
		if err != nil || detail == nil {
			continue
		}
		if detail.Sender.UserID == selfID {
			return true
		}
	}
	return false
}

// snapshotGroupPool 获取群聊缓存池的快照
