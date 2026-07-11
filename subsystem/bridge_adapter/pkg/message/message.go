package message

// 消息解析与处理逻辑

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"bridge_adapter/pkg/cache"
	"bridge_adapter/pkg/config"
	"bridge_adapter/pkg/logger"
	"bridge_adapter/pkg/napcat"
	"bridge_adapter/pkg/types"
)

// ParseMessageSegments 解析消息段列表，返回格式化的内容和是否包含图片
func ParseMessageSegments(segments []types.MessageSegment, groupID int64) (interface{}, bool, error) {
	var contentArray []map[string]interface{}
	var contentStr string
	var hasImages bool

	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var textData types.TextData
			if err := json.Unmarshal(segment.Data, &textData); err != nil {
				return nil, false, err
			}
			if len(contentArray) > 0 {
				contentArray = append(contentArray, map[string]interface{}{
					"type": "text",
					"text": textData.Text,
				})
			} else {
				contentStr += textData.Text
			}
		case "reply":
			var replyData types.ReplyData
			if err := json.Unmarshal(segment.Data, &replyData); err != nil {
				return nil, false, err
			}
			replyContent, err := napcat.GetMessageContent(replyData.ID)
			if err != nil {
				logger.Warn("获取回复消息内容失败: %v", err)
				replyText := "[回复] "
				appendContent(&contentArray, &contentStr, replyText)
			} else {
				replyText := "[回复: " + replyContent + "] "
				appendContent(&contentArray, &contentStr, replyText)
			}
		case "image":
			var imageData types.ImageData
			if err := json.Unmarshal(segment.Data, &imageData); err != nil {
				return nil, false, err
			}
			hasImages = true
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
		case "at":
			var atData types.AtData
			if err := json.Unmarshal(segment.Data, &atData); err != nil {
				return nil, false, err
			}
			atUserID, err := strconv.ParseInt(atData.QQ, 10, 64)
			var atText string
			if err != nil {
				atText = "@" + atData.QQ + " "
			} else {
				userName := config.GetUserName(groupID, atUserID)
				atText = "@" + userName + " "
			}
			appendContent(&contentArray, &contentStr, atText)
		case "forward":
			var forwardData types.ForwardData
			if err := json.Unmarshal(segment.Data, &forwardData); err != nil {
				return nil, false, err
			}
			forwardContent, forwardHasImages, err := napcat.GetForwardMessageContent(forwardData.ID, groupID)
			if err != nil {
				logger.Warn("获取转发消息内容失败: %v", err)
				forwardText := "[转发消息] "
				appendContent(&contentArray, &contentStr, forwardText)
			} else {
				forwardPrefix := "[转发消息]: "
				appendContent(&contentArray, &contentStr, forwardPrefix)
				if forwardHasImages {
					hasImages = true
					if len(contentArray) == 0 && contentStr != "" {
						contentArray = append(contentArray, map[string]interface{}{
							"type": "text",
							"text": contentStr,
						})
						contentStr = ""
					}
					if fcArray, ok := forwardContent.([]map[string]interface{}); ok {
						contentArray = append(contentArray, fcArray...)
					}
				} else {
					if fcStr, ok := forwardContent.(string); ok {
						appendContent(&contentArray, &contentStr, fcStr)
					}
				}
			}
		case "file":
			var fileData types.FileData
			if err := json.Unmarshal(segment.Data, &fileData); err != nil {
				return nil, false, err
			}
			fileText := "[文件] " + fileData.File + " " + fileData.URL + " "
			appendContent(&contentArray, &contentStr, fileText)
		default:
			logger.Warn("未知的消息段类型: %s", segment.Type)
		}
	}

	if hasImages {
		return contentArray, true, nil
	}
	return contentStr, false, nil
}

// appendContent 根据当前内容格式追加文本
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

// BuildOpenAIMessages 从缓存消息构建 OpenAI 格式消息列表
func BuildOpenAIMessages(messages []types.CachedMessage) []types.OpenAIMessage {
	var openAIMessages []types.OpenAIMessage

	for _, msg := range messages {
		senderName := config.GetUserName(msg.GroupID, msg.UserID)

		if msg.HasImages {
			contentArray, _ := msg.Content.([]map[string]interface{})
			withSender := append([]map[string]interface{}{
				{
					"type": "text",
					"text": senderName + " : ",
				},
			}, contentArray...)
			openAIMessages = append(openAIMessages, types.OpenAIMessage{
				Role:    "user",
				Content: withSender,
			})
		} else {
			contentStr, _ := msg.Content.(string)
			finalContent := senderName + " : " + contentStr
			openAIMessages = append(openAIMessages, types.OpenAIMessage{
				Role:    "user",
				Content: finalContent,
			})
		}
	}

	return openAIMessages
}

// HandleNapcatMessage 处理从 Napcat 接收到的消息
func HandleNapcatMessage(rawMessage []byte) {
	var napcatMsg types.NapcatMessage
	if err := json.Unmarshal(rawMessage, &napcatMsg); err != nil {
		logger.Error("解析 napcat 消息失败: %v", err)
		return
	}

	// 过滤自己发送的消息
	if napcatMsg.UserID == napcatMsg.SelfID {
		return
	}

	// 过滤非监听群的消息
	if !config.IsInListenGroup(napcatMsg.GroupID) {
		return
	}

	// 过滤非群消息
	if napcatMsg.MessageType != "group" {
		return
	}

	// 解析消息内容
	content, hasImages, err := ParseMessageSegments(napcatMsg.Message, napcatMsg.GroupID)
	if err != nil {
		logger.Error("解析消息段失败: %v", err)
		return
	}

	// 添加到对应群聊的缓存
	cachedMsg := types.CachedMessage{
		GroupID:   napcatMsg.GroupID,
		UserID:    napcatMsg.UserID,
		Content:   content,
		HasImages: hasImages,
	}
	cache.AddMessage(cachedMsg)

	// 提取文本内容用于关键词检测
	var contentStr string
	if str, ok := content.(string); ok {
		contentStr = str
	} else {
		if arr, ok := content.([]map[string]interface{}); ok {
			for _, item := range arr {
				if item["type"] == "text" {
					if data, ok := item["text"].(string); ok {
						contentStr += data
					}
				}
			}
		}
	}

	// 关键词检测
	if !config.ContainsTriggerKeyword(contentStr) {
		logger.Debug("群 %d: 消息不含触发关键词，仅缓存 (当前 %d 条)", napcatMsg.GroupID, len(cache.GetGroupMessages(napcatMsg.GroupID)))
		return
	}

	// 触发关键词 → 处理该群的消息
	triggerUser := config.GetUserName(napcatMsg.GroupID, napcatMsg.UserID)
	keyword := config.FindTriggerKeyword(contentStr)

	logger.Info("群 %d: 检测到触发关键词 '%s' (发送者: %s)", napcatMsg.GroupID, keyword, triggerUser)

	// 记录来源群号
	config.LastGroupID = napcatMsg.GroupID

	// 为该群生成消息摘要
	summary := cache.GenerateSummary(napcatMsg.GroupID, triggerUser, keyword)
	cache.AddSummary(napcatMsg.GroupID, summary)
	logger.Info("群 %d: 已生成消息摘要 (关键词=%s, 摘要长度=%d)", napcatMsg.GroupID, keyword, len(summary.Content))

	// 构建并发送该群的所有缓存消息
	groupMessages := cache.GetGroupMessages(napcatMsg.GroupID)
	openAIMessages := BuildOpenAIMessages(groupMessages)

	err = SendToLunarCore(openAIMessages)
	if err != nil {
		logger.Error("发送群 %d 消息到 lunar_core 失败: %v", napcatMsg.GroupID, err)
		return
	}

	// 发送成功后清除该群的缓存
	cache.ClearGroupCache(napcatMsg.GroupID)

	// 打印缓存统计
	totalGroups, totalMessages, totalSummaries := cache.GetCacheStats()
	logger.Info("缓存统计: %d 个群, %d 条消息, %d 条摘要", totalGroups, totalMessages, totalSummaries)
}

// SendToLunarCore 将消息发送到 lunar_core 的消息写入接口
func SendToLunarCore(messages []types.OpenAIMessage) error {
	url := config.AppConfig.QQAdapter.LunarCoreUrl + "/write/message"

	requestData := types.BatchMessageRequest{
		Messages: messages,
	}

	body, err := json.Marshal(requestData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := napcat.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	logger.Debug("lunar_core 响应: %s", respBody)

	return nil
}
