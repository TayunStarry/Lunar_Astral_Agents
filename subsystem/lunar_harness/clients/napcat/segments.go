package napcat

// segments.go 出站消息段构造：文本、@、引用与提及解析。

import (
	"strconv"
	"strings"
)

// textSegment 纯文本消息段
func textSegment(text string) []map[string]any {
	return []map[string]any{
		{"type": "text", "data": map[string]string{"text": text}},
	}
}

// atSegment @消息段
func atSegment(qq string) map[string]any {
	return map[string]any{
		"type": "at",
		"data": map[string]string{"qq": qq},
	}
}

// replySegment 引用消息段（引用某条原消息，id 为 QQ 消息 ID）
func replySegment(messageID string) map[string]any {
	return map[string]any{
		"type": "reply",
		"data": map[string]string{"id": messageID},
	}
}

// buildOutboundSegments 将回应文本组装为消息段：
// 文本中的 [对 xxx 说] 转换为真实 @ 段，无法解析的提及保留原文；
// replyToUserID 非零时在最前追加 @，与其重复的提及标记直接吞掉避免双重 @；
// quoteMessageID 非零时在整条消息最前追加 reply 段（QQ 要求引用段位于消息段首位）。
func buildOutboundSegments(groupID int64, text string, replyToUserID int64, quoteMessageID int64) []map[string]any {
	segments := make([]map[string]any, 0, 5)
	if quoteMessageID != 0 {
		segments = append(segments, replySegment(strconv.FormatInt(quoteMessageID, 10)))
	}
	replyToQQ := ""
	if replyToUserID != 0 {
		replyToQQ = strconv.FormatInt(replyToUserID, 10)
		segments = append(segments, atSegment(replyToQQ))
	}

	lastIndex := 0
	for _, loc := range mentionPattern.FindAllStringSubmatchIndex(text, -1) {
		name := strings.TrimSpace(text[loc[2]:loc[3]])
		qq := lookupAtTarget(groupID, name)
		if qq == "" {
			// 无法解析为真实成员的提及保留原文
			continue
		}
		if replyToQQ != "" && qq == replyToQQ {
			// 前置回应 @ 已覆盖该目标，吞掉标记避免重复 @
			lastIndex = loc[1]
			continue
		}
		if loc[0] > lastIndex {
			segments = append(segments, map[string]any{
				"type": "text",
				"data": map[string]string{"text": text[lastIndex:loc[0]]},
			})
		}
		segments = append(segments, atSegment(qq))
		lastIndex = loc[1]
	}
	if rest := text[lastIndex:]; rest != "" {
		segments = append(segments, map[string]any{
			"type": "text",
			"data": map[string]string{"text": rest},
		})
	}
	// 回应 @ 与正文之间补一个空格，避免 QQ 端渲染成 "@某某正文"
	if replyToQQ != "" {
		for _, seg := range segments {
			if seg["type"] != "text" {
				continue
			}
			if data, ok := seg["data"].(map[string]string); ok && data["text"] != "" &&
				!strings.HasPrefix(data["text"], " ") && !strings.HasPrefix(data["text"], "\n") {
				data["text"] = " " + data["text"]
			}
			break
		}
	}
	return segments
}

// lookupAtTarget 将提及名称解析为可 @ 的目标（QQ号字符串），无法识别返回 ""
func lookupAtTarget(groupID int64, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if name == "全体成员" || name == "所有人" {
		return "all"
	}
	if name == "我" {
		// 指代月华自身，无需 @
		return ""
	}
	client := Default()
	client.memberMu.Lock()
	defer client.memberMu.Unlock()
	for userID, memberName := range client.memberNameCache[groupID] {
		if memberName == name {
			return strconv.FormatInt(userID, 10)
		}
	}
	return ""
}
