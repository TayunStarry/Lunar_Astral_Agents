package processor

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
	"transponder/internal/setup"
)

// universalCodeEscape 通用代码转义
func (class *Handle) universalCodeEscape(msgMap setup.MessageItem) ProcessResult {
	content := make(ProcessResult, 0)
	switch msgMap.Type {
	case "text":
		content = append(content, class.ProcessTextMessage(msgMap.Data)...)
	case "image":
		content = append(content, class.ProcessImageMessage(msgMap.Data)...)
	case "file":
		fileName := msgMap.Data.File
		if isImage := class.HasImageExtension(fileName); isImage {
			content = append(content, class.ProcessImageMessage(msgMap.Data)...)
			break
		}
		content = append(content, TextMessage{Type: "text", Text: fmt.Sprintf("[文件: %s]", fileName)})
	case "at":
		content = append(content, class.ProcessAtMessage(msgMap.Data)...)
	case "reply":
		content = append(content, class.processReplyMessage(msgMap.Data)...)
	case "forward":
		// 获取转发消息内容
		forwardContent, err := class.getForwardMessage(msgMap.Data.ID)
		// 处理获取转发消息内容的错误
		if err != nil {
			content = append(content, TextMessage{Type: "text", Text: "[转发消息]"})
			break
		}
		content = append(content, forwardContent...)
	default:
		// 其他未处理的消息类型
		content = append(content, TextMessage{Type: "text", Text: "[CQ码]"})
	}
	return content
}

// processArrayMessageItem 处理数组格式的消息项
func (class *Handle) processArrayMessageItem(itemMap setup.MessageItem) ProcessResult {
	content := make(ProcessResult, 0)
	content = class.universalCodeEscape(itemMap)
	return content
}

// processCQCode 处理单个CQ码
func (class *Handle) processCQCode(cqType string, cqParams string) ProcessResult {
	msgData := setup.MessageItemData{
		File: class.extractParam(cqParams, "file"),
		Url:  class.extractParam(cqParams, "url"),
		QQ:   class.extractParam(cqParams, "qq"),
		ID:   class.extractParam(cqParams, "id"),
	}
	content := class.universalCodeEscape(setup.MessageItem{Type: cqType, Data: msgData})
	return content
}

// ProcessTextMessage 处理文本消息
func (class *Handle) ProcessTextMessage(msgData setup.MessageItemData) ProcessResult {
	content := make(ProcessResult, 0)
	// 处理文本中的CQ码
	cqContents := class.Process(msgData.Text)
	if len(cqContents) > 0 {
		return cqContents
	}
	content = append(content, TextMessage{Type: "text", Text: msgData.Text})
	return content
}

// ProcessImageMessage 处理图片消息
func (class *Handle) ProcessImageMessage(msgData setup.MessageItemData) ProcessResult {
	content := make(ProcessResult, 0)
	imageParam := ImageObjectParameter{URL: msgData.Url, File: msgData.File}
	imageURL := class.getImageURL(imageParam)
	content = append(content, ImageMessage{Type: "image_url", ImageURL: ImageURL{URL: imageURL, Detail: "auto"}})
	return content
}

// ProcessAtMessage 处理@提及消息
func (class *Handle) ProcessAtMessage(msgData setup.MessageItemData) ProcessResult {
	content := make(ProcessResult, 0)
	// 提取用户ID
	userID, err := strconv.ParseInt(msgData.QQ, 10, 64)
	if err != nil {
		return content
	}
	// 提取用户昵称
	nickname := class.getUserName(class.currentGroupID, userID)
	// 构建@提及消息
	content = append(content, TextMessage{Type: "text", Text: fmt.Sprintf("@%s", nickname)})
	// 返回构建好的@提及消息
	return content
}

// getForwardMessage 获取转发消息内容
func (class *Handle) getForwardMessage(forwardID string) (ProcessResult, error) {
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
func (class *Handle) processForwardMessages(messages []any) ProcessResult {
	content := make(ProcessResult, 0)

	// 添加转发消息前缀
	content = append(content, TextMessage{
		Type: "text",
		Text: "[转发消息]: ",
	})

	// 处理每条消息
	for i, msg := range messages {
		// 限制消息数量，避免过长
		if i >= 99 { // 最多处理99条消息
			content = append(content, TextMessage{
				Type: "text",
				Text: "...(消息过多)",
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
		content = append(content, TextMessage{
			Type: "text",
			Text: "...(内容过长)",
		})
	}

	return content
}

// processSingleForwardMessage 处理单条转发消息
func (class *Handle) processSingleForwardMessage(msgMap map[string]any) ProcessResult {
	content := make(ProcessResult, 0)

	// 提取发送者信息
	senderName := class.extractSenderName(msgMap)

	// 添加发送者信息
	if len(strings.TrimSpace(senderName)) > 3 {
		content = append(content, TextMessage{
			Type: "text",
			Text: fmt.Sprintf("[发言人:%s]: ", senderName),
		})
	}

	// 处理消息内容
	if messageField, ok := msgMap["message"]; ok {
		messageContent := class.ProcessMessageFieldContent(messageField)
		content = append(content, messageContent...)
	}

	return content
}

// processReplyMessage 处理引用消息
func (class *Handle) processReplyMessage(msgData setup.MessageItemData) ProcessResult {
	content := make(ProcessResult, 0)
	messageID, err := strconv.ParseInt(msgData.ID, 10, 64)
	if err != nil {
		return content
	}
	if messageID > 0 {
		// 获取原始消息内容
		originalMsg, err := class.getOriginalMessage(messageID)
		if err != nil {
			log.Printf("获取原始消息失败 (消息 ID: %d): %v", messageID, err)
			// 如果获取失败，添加一个占位符
			content = append(content, TextMessage{
				Type: "text",
				Text: "[回复消息]",
			})
		} else {
			// 解析并添加原始消息内容
			if originalContent, ok := originalMsg["content"].(ProcessResult); ok {
				// 添加回复前缀
				senderName := ""
				if name, ok := originalMsg["sender"].(string); ok {
					senderName = name
				}

				if senderName != "" {
					content = append(content, TextMessage{
						Type: "text",
						Text: fmt.Sprintf("[回复 %s]: ", senderName),
					})
				} else {
					content = append(content, TextMessage{
						Type: "text",
						Text: "[回复]: ",
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
func (class *Handle) getOriginalMessage(messageID int64) (map[string]any, error) {
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
