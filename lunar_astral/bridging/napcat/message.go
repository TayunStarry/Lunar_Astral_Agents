package napcat

// 消息解析与处理逻辑

import (
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// HandleNapcatMessage 处理从 Napcat 接收到的消息
func HandleNapcatMessage(rawMessage []byte) {
	var napcatMsg NapcatMessage
	if err := json.Unmarshal(rawMessage, &napcatMsg); err != nil {
		LoggerGeneral.SubError("LunarCore", "Napcat", "解析消息失败: %v", err)
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

	// 解析消息内容（返回 OpenAI 多模态格式）
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

	// 提取文本内容用于关键词检测
	contentStr := extractTextContent(content)

	// 关键词检测
	if !containsKeyword(contentStr) {
		LoggerGeneral.SubInfo("LunarCore", "Napcat", "群 %d: 消息不含触发关键词，仅缓存 (当前 %d 条)", napcatMsg.GroupID, GetCacheSize())
		return
	}

	// 触发关键词 → 将缓存消息推送给智能体
	LoggerGeneral.SubInfo("LunarCore", "Napcat", "群 %d: 检测到触发关键词 (发送者: %s)", napcatMsg.GroupID, napcatMsg.Sender.Nickname)

	// 构建推送内容：将缓存中的消息构建为 OpenAI 格式
	cachedMessages := GetCachedMessages()
	openAIMessages := buildOpenAIMessages(cachedMessages)

	// 通过回调向智能体推送消息
	if SendMessageToAgent != nil {
		SendMessageToAgent(openAIMessages)
	} else {
		LoggerGeneral.SubError("LunarCore", "Napcat", "SendMessageToAgent 回调未注册，无法推送消息")
	}

	// 推送后清空缓存
	ClearCachedMessages()
}

// parseMessageSegments 解析消息段列表，返回 OpenAI 多模态格式内容
// 纯文本返回 string，包含图片返回 []map[string]interface{}
func parseMessageSegments(segments []MessageSegment) (interface{}, bool) {
	var contentArray []map[string]interface{}
	var contentStr string
	var hasImages bool

	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var textData TextData
			if err := json.Unmarshal(segment.Data, &textData); err == nil {
				appendContent(&contentArray, &contentStr, textData.Text)
			}
		case "at":
			var atData AtData
			if err := json.Unmarshal(segment.Data, &atData); err == nil {
				atUserID, err := strconv.ParseInt(atData.QQ, 10, 64)
				var atText string
				if err != nil {
					atText = "@" + atData.QQ + " "
				} else {
					atText = fmt.Sprintf("@%d ", atUserID)
				}
				appendContent(&contentArray, &contentStr, atText)
			}
		case "reply":
			var replyData ReplyData
			if err := json.Unmarshal(segment.Data, &replyData); err == nil {
				replyContent, err := GetMessageContent(replyData.ID)
				if err != nil {
					appendContent(&contentArray, &contentStr, "[回复] ")
				} else {
					appendContent(&contentArray, &contentStr, "[回复: "+replyContent+"] ")
				}
			}
		case "image":
			var imageData ImageData
			if err := json.Unmarshal(segment.Data, &imageData); err == nil {
				hasImages = true
				// 首次遇到图片时，将已有的纯文本迁移到数组格式
				if len(contentArray) == 0 && contentStr != "" {
					contentArray = append(contentArray, map[string]interface{}{
						"type": "text",
						"text": contentStr,
					})
					contentStr = ""
				}
				contentArray = append(contentArray, map[string]interface{}{
					"type": "image_url",
					"image_url": map[string]string{
						"url": imageData.URL,
					},
				})
			}
		case "forward":
			var forwardData ForwardData
			if err := json.Unmarshal(segment.Data, &forwardData); err == nil {
				appendContent(&contentArray, &contentStr, "[转发消息] ")
			}
		case "file":
			var fileData FileData
			if err := json.Unmarshal(segment.Data, &fileData); err == nil {
				fileText := "[文件] " + fileData.File + " "
				appendContent(&contentArray, &contentStr, fileText)
			}
		}
	}

	if hasImages {
		return contentArray, true
	}
	return contentStr, false
}

// appendContent 根据当前内容格式追加文本
// 如果已经是数组格式（有图片），追加为 text 类型元素；否则追加到纯字符串
func appendContent(contentArray *[]map[string]interface{}, contentStr *string, text string) {
	if len(*contentArray) > 0 {
		*contentArray = append(*contentArray, map[string]interface{}{
			"type": "text",
			"text": text,
		})
	} else {
		*contentStr += text
	}
}

// extractTextContent 从 content (string 或 []map[string]interface{}) 中提取纯文本
func extractTextContent(content interface{}) string {
	if str, ok := content.(string); ok {
		return str
	}
	if arr, ok := content.([]map[string]interface{}); ok {
		var sb strings.Builder
		for _, item := range arr {
			if item["type"] == "text" {
				if text, ok := item["text"].(string); ok {
					sb.WriteString(text)
				}
			}
		}
		return sb.String()
	}
	return ""
}

// buildOpenAIMessages 将缓存消息构建为 OpenAI 格式消息列表
func buildOpenAIMessages(messages []CachedMessage) []map[string]interface{} {
	var openAIMessages []map[string]interface{}

	for _, msg := range messages {
		if msg.HasImages {
			// 多模态格式：在内容数组前加上发送者标记
			contentArray, _ := msg.Content.([]map[string]interface{})
			withSender := append([]map[string]interface{}{
				{
					"type": "text",
					"text": msg.Nickname + " : ",
				},
			}, contentArray...)
			openAIMessages = append(openAIMessages, map[string]interface{}{
				"role":    "user",
				"content": withSender,
			})
		} else {
			// 纯文本格式
			contentStr, _ := msg.Content.(string)
			finalContent := msg.Nickname + " : " + contentStr
			openAIMessages = append(openAIMessages, map[string]interface{}{
				"role":    "user",
				"content": finalContent,
			})
		}
	}

	return openAIMessages
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
