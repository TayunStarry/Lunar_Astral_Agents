package message

import (
	"bytes"
	"encoding/json"
	"strconv"

	"bridge_adapter/pkg/config"
	"bridge_adapter/pkg/logger"
	"bridge_adapter/pkg/napcat"
	"bridge_adapter/pkg/types"
)

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

func BuildMessagesFromCache() []types.OpenAIMessage {
	var openAIMessages []types.OpenAIMessage

	for _, msg := range config.MessageCache {
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

func HandleNapcatMessage(message []byte) {
	var napcatMsg types.NapcatMessage
	if err := json.Unmarshal(message, &napcatMsg); err != nil {
		logger.Error("解析 napcat 消息失败: %v", err)
		return
	}

	if napcatMsg.UserID == napcatMsg.SelfID {
		return
	}

	if !config.IsInListenGroup(napcatMsg.GroupID) {
		return
	}

	if napcatMsg.MessageType != "group" {
		return
	}

	content, hasImages, err := ParseMessageSegments(napcatMsg.Message, napcatMsg.GroupID)
	if err != nil {
		logger.Error("解析消息段失败: %v", err)
		return
	}

	cachedMsg := types.CachedMessage{
		GroupID:   napcatMsg.GroupID,
		UserID:    napcatMsg.UserID,
		Content:   content,
		HasImages: hasImages,
	}
	config.AddToMessageCache(cachedMsg)

	logger.Debug("消息已添加到缓存，当前缓存 %d 条消息", len(config.MessageCache))

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

	if !config.ContainsTriggerKeyword(contentStr) {
		logger.Debug("消息不包含触发关键词，仅缓存: %s", contentStr)
		return
	}

	logger.Info("检测到触发关键词，准备发送 %d 条缓存消息", len(config.MessageCache))

	config.LastGroupID = napcatMsg.GroupID

	openAIMessages := BuildMessagesFromCache()

	err = SendToLunarCore(openAIMessages)
	if err != nil {
		logger.Error("发送消息到 lunar_core 失败: %v", err)
		return
	}

	config.ClearMessageCache()
	logger.Info("缓存消息已清除")
}

func SendToLunarCore(messages []types.OpenAIMessage) error {
	url := config.AppConfig.QQAdapter.LunarCoreUrl + "/write/message"

	requestData := types.BatchMessageRequest{
		Messages: messages,
	}

	body, err := json.Marshal(requestData)
	if err != nil {
		return err
	}

	req, err := napcat.NewHTTPRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := napcat.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := napcat.ReadResponseBody(resp)
	if err != nil {
		return err
	}

	logger.Debug("lunar_core 响应: %s", respBody)

	return nil
}
