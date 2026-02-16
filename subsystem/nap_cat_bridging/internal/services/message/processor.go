package message

import (
	"fmt"
	"strings"

	"nap_cat_bridging/internal/models"
)

// Processor 消息处理器
type Processor struct {
	handler *Handler
}

// NewProcessor 创建消息处理器
func NewProcessor(handler *Handler) *Processor {
	return &Processor{
		handler: handler,
	}
}

// ProcessMessageContent 处理消息内容
func (p *Processor) ProcessMessageContent(rawMsg map[string]any, groupID int64, senderName string) any {
	// 提取message字段
	var msgItems []models.MsgItem
	if msgField, ok := rawMsg["message"]; ok {
		switch msg := msgField.(type) {
		case []any:
			// 消息是数组格式
			for _, item := range msg {
				if msgMap, ok := item.(map[string]any); ok {
					msgItem := models.MsgItem{
						Type: "",
						Data: nil,
					}
					if msgType, ok := msgMap["type"].(string); ok {
						msgItem.Type = msgType
					}
					if msgData, ok := msgMap["data"].(map[string]any); ok {
						msgItem.Data = msgData
					}
					if msgItem.Type != "" {
						msgItems = append(msgItems, msgItem)
					}
				}
			}
		case string:
			// 消息是字符串格式（如CQ码），创建一个文本消息项
			msgItems = []models.MsgItem{
				{
					Type: "text",
					Data: map[string]any{
						"text": msg,
					},
				},
			}
		}
	}

	// 提取并转换消息内容
	content := make([]map[string]any, 0)

	// 添加发言人信息作为第一个文本元素
	content = append(content, map[string]any{
		"type": "text",
		"text": fmt.Sprintf("[发言人:%s]: ", senderName),
	})

	for _, msg := range msgItems {
		// 将 models.MsgItem 转换为 map[string]any 格式
		msgMap := map[string]any{
			"type": msg.Type,
			"data": msg.Data,
		}
		content = append(content, p.handler.processMessageItem(msgMap, map[string]any{"group_id": float64(groupID)})...)
	}

	// 如果没有内容，返回空字符串
	if len(content) == 0 {
		return ""
	}

	// 如果只有一个文本消息，直接返回字符串
	if len(content) == 1 {
		if textContent, ok := content[0]["type"].(string); ok && textContent == "text" {
			if text, ok := content[0]["text"].(string); ok {
				return text
			}
		}
	}

	return content
}

// ProcessOriginalMessageContent 处理原始消息内容
func (p *Processor) ProcessOriginalMessageContent(data map[string]any) []map[string]any {
	var content []map[string]any
	if messageField, ok := data["message"]; ok {
		switch msg := messageField.(type) {
		case []any:
			// 消息是数组格式
			content = make([]map[string]any, 0)
			for _, item := range msg {
				content = append(content, p.handler.processMessageItem(item, data)...)
			}
		case string:
			// 消息是字符串格式（如CQ码），处理其中的CQ码
			cqContents := p.handler.cqProcessor.Process(msg)
			if len(cqContents) > 0 {
				content = cqContents
			} else if len(strings.TrimSpace(msg)) > 3 {
				content = []map[string]any{
					{
						"type": "text",
						"text": msg,
					},
				}
			}
		}
	}

	// 如果没有解析到content，使用raw_message作为fallback
	if len(content) == 0 {
		if rawMessage, ok := data["raw_message"].(string); ok {
			content = p.ProcessRawMessage(rawMessage)
		}
	}

	return content
}

// ProcessTextMessage 处理文本消息
func (p *Processor) ProcessTextMessage(msgData map[string]any) []map[string]any {
	content := make([]map[string]any, 0)
	if text, ok := msgData["text"].(string); ok {
		// 处理文本中的CQ码
		cqContents := p.handler.cqProcessor.Process(text)
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
func (p *Processor) ProcessImageMessage(msgData map[string]any) []map[string]any {
	content := make([]map[string]any, 0)
	imageURL := p.handler.getImageURL(msgData)
	if imageURL != "" {
		content = append(content, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url":    imageURL,
				"detail": "auto",
			},
		})
	}
	return content
}

// ProcessAtMessage 处理@提及消息
func (p *Processor) ProcessAtMessage(msgData map[string]any, data map[string]any) []map[string]any {
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

	nickname := p.handler.getUserName(groupID, userID)
	content = append(content, map[string]any{
		"type": "text",
		"text": fmt.Sprintf("@%s", nickname),
	})
	return content
}

// ProcessRawMessage 处理原始消息作为 fallback
func (p *Processor) ProcessRawMessage(rawMessage string) []map[string]any {
	// 处理raw_message中的CQ码
	cqContents := p.handler.cqProcessor.Process(rawMessage)
	if len(cqContents) > 0 {
		return cqContents
	}
	// 如果没有CQ码，创建一个文本消息项
	return []map[string]any{
		{
			"type": "text",
			"text": rawMessage,
		},
	}
}
