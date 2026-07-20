package napcat

// 消息解析与处理逻辑

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"logger"
)

// HandleNapcatMessage 处理从 Napcat 接收到的消息
func HandleNapcatMessage(rawMessage []byte) {
	var napcatMsg NapcatMessage
	if err := json.Unmarshal(rawMessage, &napcatMsg); err != nil {
		logger.SubError("LunarCore", "Napcat", "解析消息失败: %v", err)
		return
	}

	// 过滤自己发送的消息
	if napcatMsg.UserID == napcatMsg.SelfID {
		return
	}

	// 过滤非群消息
	if napcatMsg.MessageType != "group" {
		return
	}

	// 过滤非目标群聊的消息
	if napcatMsg.GroupID != bridgeConfig.BridgingTarget {
		return
	}

	// 解析消息内容
	content, hasImages := parseMessageSegments(napcatMsg.Message)

	// 添加到缓存
	cachedMsg := CachedMessage{
		GroupID:   napcatMsg.GroupID,
		UserID:    napcatMsg.UserID,
		Nickname:  napcatMsg.Sender.Nickname,
		Content:   content,
		HasImages: hasImages,
	}
	AddCachedMessage(cachedMsg)

	// 关键词检测
	if !containsKeyword(content) {
		logger.SubInfo("LunarCore", "Napcat", "群 %d: 消息不含触发关键词，仅缓存 (当前 %d 条)", napcatMsg.GroupID, GetCacheSize())
		return
	}

	// 触发关键词 → 将缓存消息推送给智能体
	logger.SubInfo("LunarCore", "Napcat", "群 %d: 检测到触发关键词 (发送者: %s)", napcatMsg.GroupID, napcatMsg.Sender.Nickname)

	// 构建推送内容：将缓存中的消息拼接
	cachedMessages := GetCachedMessages()
	pushContent := buildPushContent(cachedMessages)

	// 通过回调向智能体推送消息
	if SendMessageToAgent != nil {
		SendMessageToAgent(pushContent, napcatMsg.Sender.Nickname)
	} else {
		logger.SubError("LunarCore", "Napcat", "SendMessageToAgent 回调未注册，无法推送消息")
	}

	// 推送后清空缓存
	ClearCachedMessages()
}

// parseMessageSegments 解析消息段列表，返回格式化的文本内容
func parseMessageSegments(segments []MessageSegment) (string, bool) {
	var contentStr string
	var hasImages bool

	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var textData TextData
			if err := json.Unmarshal(segment.Data, &textData); err == nil {
				contentStr += textData.Text
			}
		case "at":
			var atData AtData
			if err := json.Unmarshal(segment.Data, &atData); err == nil {
				atUserID, err := strconv.ParseInt(atData.QQ, 10, 64)
				if err != nil {
					contentStr += "@" + atData.QQ + " "
				} else {
					contentStr += fmt.Sprintf("@%d ", atUserID)
				}
			}
		case "reply":
			var replyData ReplyData
			if err := json.Unmarshal(segment.Data, &replyData); err == nil {
				replyContent, err := GetMessageContent(replyData.ID)
				if err != nil {
					contentStr += "[回复] "
				} else {
					contentStr += "[回复: " + replyContent + "] "
				}
			}
		case "image":
			hasImages = true
			contentStr += "[图片] "
		case "forward":
			contentStr += "[转发消息] "
		case "file":
			var fileData FileData
			if err := json.Unmarshal(segment.Data, &fileData); err == nil {
				contentStr += "[文件] " + fileData.File + " "
			}
		}
	}

	return contentStr, hasImages
}

// containsKeyword 检查消息内容是否包含配置的触发关键词
func containsKeyword(content string) bool {
	if len(bridgeConfig.BridgingKeywords) == 0 {
		return true
	}
	for _, keyword := range bridgeConfig.BridgingKeywords {
		if strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}

// buildPushContent 将缓存消息拼接为推送给智能体的内容
func buildPushContent(messages []CachedMessage) string {
	var sb strings.Builder
	for i, msg := range messages {
		sb.WriteString(msg.Nickname)
		sb.WriteString(": ")
		sb.WriteString(msg.Content)
		if i < len(messages)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
