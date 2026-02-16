package message

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// universalCodeEscape 通用代码转义
func (class *Processor) universalCodeEscape(msgType string, msgMap map[string]any, data map[string]any) []map[string]any {
	content := make([]map[string]any, 0)
	switch msgType {
	case "text":
		if msgData, ok := msgMap["data"].(map[string]any); ok {
			content = append(content, class.ProcessTextMessage(msgData)...)
		}
	case "image":
		if msgData, ok := msgMap["data"].(map[string]any); ok {
			content = append(content, class.ProcessImageMessage(msgData)...)
		}
	case "file":
		// 文件消息，检查是否是图片文件
		if msgData, ok := msgMap["data"].(map[string]any); ok {
			fileName := msgData["file"].(string)
			if isImage := class.HasImageExtension(fileName); isImage {
				content = append(content, class.ProcessImageMessage(msgData)...)
				break
			}
			content = append(content, map[string]any{
				"type": "text",
				"text": fmt.Sprintf("[文件: %s]", fileName),
			})
		}
	case "at":
		if msgData, ok := msgMap["data"].(map[string]any); ok {
			content = append(content, class.ProcessAtMessage(msgData, data)...)
		}
	case "reply":
		if msgData, ok := msgMap["data"].(map[string]any); ok {
			content = append(content, class.processReplyMessage(msgData)...)
		}
	case "forward":
		if msgData, ok := msgMap["data"].(map[string]any); ok {
			// 获取转发消息内容
			forwardContent, err := class.getForwardMessage(msgData["id"].(string))
			if err != nil {
				content = append(content, map[string]any{
					"type": "text",
					"text": "[转发消息]",
				})
			} else {
				// 添加转发消息内容
				content = append(content, forwardContent...)
			}
		}
	default:
		// 其他未处理的消息类型
		content = append(content, map[string]any{
			"type": "text",
			"text": "[CQ码]",
		})
	}

	return content
}

// processArrayMessageItem 处理数组格式的消息项
func (class *Processor) processArrayMessageItem(itemMap map[string]any) []map[string]any {
	content := make([]map[string]any, 0)

	msgType := ""
	if t, ok := itemMap["type"].(string); ok {
		msgType = t
	}

	// 使用通用代码转义函数处理消息项
	content = class.universalCodeEscape(msgType, itemMap, nil)

	return content
}

// processMessageItem 处理单个消息项
func (class *Processor) processMessageItem(item any, data map[string]any) []map[string]any {
	content := make([]map[string]any, 0)
	if msgMap, ok := item.(map[string]any); ok {

		msgType := ""
		if t, ok := msgMap["type"].(string); ok {
			msgType = t
		}

		content = class.universalCodeEscape(msgType, msgMap, data)
	}
	return content
}

// processCQCode 处理单个CQ码
func (class *Processor) processCQCode(cqType string, cqParams string) []map[string]any {
	msgData := map[string]any{
		"file": class.extractParam(cqParams, "file"),
		"url":  class.extractParam(cqParams, "url"),
		"qq":   class.extractParam(cqParams, "qq"),
		"id":   class.extractParam(cqParams, "id"),
	}
	content := class.universalCodeEscape(cqType, msgData, msgData)
	return content
}

// ProcessTextMessage 处理文本消息
func (class *Processor) ProcessTextMessage(msgData map[string]any) []map[string]any {
	content := make([]map[string]any, 0)
	if text, ok := msgData["text"].(string); ok {
		// 处理文本中的CQ码
		cqContents := class.Process(text)
		if len(cqContents) > 0 {
			return cqContents
		}
		content = append(content, map[string]any{
			"type": "text",
			"text": text,
		})
	}
	return content
}

// ProcessImageMessage 处理图片消息
func (class *Processor) ProcessImageMessage(msgData map[string]any) []map[string]any {
	content := make([]map[string]any, 0)
	imageURL := class.getImageURL(msgData)
	content = append(content, map[string]any{
		"type": "image_url",
		"image_url": map[string]any{
			"url":    imageURL,
			"detail": "auto",
		},
	})
	return content
}

// ProcessAtMessage 处理@提及消息
func (class *Processor) ProcessAtMessage(msgData map[string]any, data map[string]any) []map[string]any {
	content := make([]map[string]any, 0)
	var userID int64
	if qq, ok := msgData["qq"].(string); ok {
		fmt.Sscanf(qq, "%d", &userID)
	} else if qq, ok := msgData["qq"].(float64); ok {
		userID = int64(qq)
	}

	// 尝试获取用户昵称
	groupID := int64(0)
	if gid, ok := data["group_id"].(float64); ok {
		groupID = int64(gid)
	}

	nickname := class.getUserName(groupID, userID)
	content = append(content, map[string]any{
		"type": "text",
		"text": fmt.Sprintf("@%s", nickname),
	})
	return content
}

// getForwardMessage 获取转发消息内容
func (class *Processor) getForwardMessage(forwardID string) ([]map[string]any, error) {
	// 创建请求参数
	params := map[string]interface{}{
		"message_id": forwardID,
	}

	// 发送消息
	echo, err := class.wsClient.SendMessage("get_forward_msg", params)
	if err != nil {
		return nil, fmt.Errorf("发送get_forward_msg请求失败: %v", err)
	}

	// 设置超时时间
	timeout := time.Now().Add(30 * time.Second)

	// 等待响应
	for time.Now().Before(timeout) {
		messageBytes, err := class.wsClient.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("读取消息失败: %v", err)
		}

		// 解析响应
		var response struct {
			Echo    string      `json:"echo"`
			Status  string      `json:"status"`
			Data    interface{} `json:"data"`
			Message string      `json:"message"`
		}
		if err := json.Unmarshal(messageBytes, &response); err != nil {
			continue
		}

		// 检查是否是get_forward_msg的响应
		if strings.Contains(response.Echo, echo) {
			if response.Status == "ok" && response.Data != nil {
				// 解析数据
				if data, ok := response.Data.(map[string]any); ok {
					// 提取消息列表
					if messages, ok := data["messages"].([]any); ok {
						content := class.processForwardMessages(messages)
						return content, nil
					}
				}
				return nil, fmt.Errorf("获取转发消息失败: 数据格式错误")
			}
			return nil, fmt.Errorf("获取转发消息失败: %s", response.Message)
		}
	}

	return nil, fmt.Errorf("获取转发消息超时")
}

// processForwardMessages 处理转发消息列表
func (class *Processor) processForwardMessages(messages []any) []map[string]any {
	content := make([]map[string]any, 0)

	// 添加转发消息前缀
	content = append(content, map[string]any{
		"type": "text",
		"text": "[转发消息]: ",
	})

	// 处理每条消息
	for i, msg := range messages {
		// 限制消息数量，避免过长
		if i >= 99 { // 最多处理99条消息
			content = append(content, map[string]any{
				"type": "text",
				"text": "...(消息过多)",
			})
			break
		}

		if msgMap, ok := msg.(map[string]any); ok {
			// 处理单条消息
			messageContent := class.processSingleForwardMessage(msgMap)
			content = append(content, messageContent...)
		}
	}

	// 限制总长度
	if len(content) > 100 {
		content = content[:100]
		content = append(content, map[string]any{
			"type": "text",
			"text": "...(内容过长)",
		})
	}

	return content
}

// processSingleForwardMessage 处理单条转发消息
func (class *Processor) processSingleForwardMessage(msgMap map[string]any) []map[string]any {
	content := make([]map[string]any, 0)

	// 提取发送者信息
	senderName := class.extractSenderName(msgMap)

	// 添加发送者信息
	if len(strings.TrimSpace(senderName)) > 3 {
		content = append(content, map[string]any{
			"type": "text",
			"text": fmt.Sprintf("[发言人:%s]: ", senderName),
		})
	}

	// 处理消息内容
	if messageField, ok := msgMap["message"]; ok {
		messageContent := class.processMessageContent(messageField)
		content = append(content, messageContent...)
	}

	return content
}

// processReplyMessage 处理引用消息
func (class *Processor) processReplyMessage(msgData map[string]any) []map[string]any {
	content := make([]map[string]any, 0)
	var messageID int64
	if id, ok := msgData["id"].(string); ok {
		fmt.Sscanf(id, "%d", &messageID)
	} else if id, ok := msgData["id"].(float64); ok {
		messageID = int64(id)
	}

	if messageID > 0 {
		// 获取原始消息内容
		originalMsg, err := class.getOriginalMessage(messageID)
		if err != nil {
			log.Printf("获取原始消息失败 (消息 ID: %d): %v", messageID, err)
			// 如果获取失败，添加一个占位符
			content = append(content, map[string]any{
				"type": "text",
				"text": "[回复消息]",
			})
		} else {
			// 解析并添加原始消息内容
			if originalContent, ok := originalMsg["content"].([]map[string]any); ok {
				// 添加回复前缀
				senderName := ""
				if name, ok := originalMsg["sender"].(string); ok {
					senderName = name
				}

				if senderName != "" {
					content = append(content, map[string]any{
						"type": "text",
						"text": fmt.Sprintf("[回复 %s]: ", senderName),
					})
				} else {
					content = append(content, map[string]any{
						"type": "text",
						"text": "[回复]: ",
					})
				}

				// 添加原始消息内容
				content = append(content, originalContent...)
			}
		}
	}
	return content
}

// getOriginalMessage 获取原始消息内容
func (class *Processor) getOriginalMessage(messageID int64) (map[string]any, error) {
	// 创建请求参数
	params := map[string]any{
		"message_id": messageID,
	}

	// 发送消息
	echo, err := class.wsClient.SendMessage("get_msg", params)
	if err != nil {
		return nil, fmt.Errorf("发送get_msg请求失败: %v", err)
	}

	// 设置超时时间
	timeout := time.Now().Add(30 * time.Second)

	// 等待响应
	for time.Now().Before(timeout) {
		messageBytes, err := class.wsClient.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("读取消息失败: %v", err)
		}

		// 解析响应
		var response struct {
			Echo    string      `json:"echo"`
			Status  string      `json:"status"`
			Data    interface{} `json:"data"`
			Message string      `json:"message"`
		}
		if err := json.Unmarshal(messageBytes, &response); err != nil {
			continue
		}

		// 检查是否是get_msg的响应
		if strings.Contains(response.Echo, echo) {
			if response.Status == "ok" && response.Data != nil {
				// 解析数据
				if data, ok := response.Data.(map[string]any); ok {
					return class.ParseMessageResponse(data)
				}
			}
			return nil, fmt.Errorf("获取原始消息失败: %s", response.Message)
		}
	}

	return nil, fmt.Errorf("获取原始消息超时")
}
