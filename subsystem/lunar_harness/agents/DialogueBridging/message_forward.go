package DialogueBridging

// 消息解析与单线程收发流程（按职责拆分）

import (
	"LunarSubsystem/LunarHarness/clients/napcat"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"encoding/json"
	"strconv"
	"strings"
)

func processForwardSegment(groupID int64, id string, depth int) string {
	if id == "" {
		return "[转发消息] "
	}
	if depth >= maxForwardDepth {
		return "[嵌套转发消息（层级过深，省略）] "
	}
	messages, err := napcat.GetForwardMessageContent(id)
	if err != nil || len(messages) == 0 {
		logger.SubError("LunarCore", "Napcat", "获取合并转发消息失败: %v", err)
		return "[转发消息] "
	}

	indent := strings.Repeat("    ", depth)
	var sb strings.Builder
	if depth == 0 {
		sb.WriteString("[转发消息]\n")
	}
	for _, msg := range messages {
		sender := msg.Sender.Card
		if sender == "" {
			sender = msg.Sender.Nickname
		}
		if sender == "" {
			sender = strconv.FormatInt(msg.Sender.UserID, 10)
		}
		segments := msg.Message
		if len(segments) == 0 {
			segments = msg.Content
		}
		// 纯转发节点：递归展开为嵌套块
		if nestedID := findNestedForward(segments); nestedID != "" {
			sb.WriteString(indent)
			sb.WriteString(sender)
			sb.WriteString(" 转发了以下内容：\n")
			sb.WriteString(processForwardSegment(groupID, nestedID, depth+1))
			continue
		}
		sb.WriteString(indent)
		sb.WriteString(sender)
		sb.WriteString(": ")
		sb.WriteString(extractSegmentText(groupID, depth, segments))
		sb.WriteString("\n")
	}
	return sb.String()
}

// findNestedForward 若消息段为单个合并转发节点，返回其 ID，否则返回空
func findNestedForward(segments []napcat.MessageSegment) string {
	if len(segments) != 1 || segments[0].Type != "forward" {
		return ""
	}
	var forwardData napcat.ForwardData
	if json.Unmarshal(segments[0].Data, &forwardData) == nil {
		return rawIDString(forwardData.ID)
	}
	return ""
}

// rawIDString 提取 JSON id 字段的字符串值（兼容 "123" 与 123 两种形态）
