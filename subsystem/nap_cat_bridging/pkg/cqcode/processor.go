package cqcode

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"nap_cat_bridging/internal/utils"
	"nap_cat_bridging/pkg/websocket"
)

// Processor CQ码处理器
type Processor struct {
	wsClient *websocket.Client
}

// NewProcessor 创建CQ码处理器
func NewProcessor(wsClient *websocket.Client) *Processor {
	return &Processor{
		wsClient: wsClient,
	}
}

// Process 处理包含CQ码的文本
func (p *Processor) Process(text string) []map[string]any {
	content := make([]map[string]any, 0)

	// 查找所有CQ码
	r := regexp.MustCompile(`\[CQ:([^,\]]+)([^\]]*)\]`)
	matches := r.FindAllStringSubmatchIndex(text, -1)

	if len(matches) == 0 {
		// 没有找到CQ码，返回原文本
		content = append(content, map[string]any{
			"type": "text",
			"text": text,
		})
		return content
	}

	// 处理每个CQ码
	lastIndex := 0
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		// 获取CQ类型
		cqType := text[match[2]:match[3]]
		// 获取CQ参数
		cqParams := text[match[4]:match[5]]

		// 添加CQ码前的文本
		if match[0] > lastIndex {
			prefixText := text[lastIndex:match[0]]
			if len(strings.TrimSpace(prefixText)) > 3 {
				content = append(content, map[string]any{
					"type": "text",
					"text": prefixText,
				})
			}
		}

		// 处理CQ码
		cqContent := p.processCQCode(cqType, cqParams)
		content = append(content, cqContent...)

		lastIndex = match[1]
	}

	// 添加最后一个CQ码后的文本
	if lastIndex < len(text) {
		suffixText := text[lastIndex:]
		if len(strings.TrimSpace(suffixText)) > 3 {
			content = append(content, map[string]any{
				"type": "text",
				"text": suffixText,
			})
		}
	}

	return content
}

// processCQCode 处理单个CQ码
func (p *Processor) processCQCode(cqType string, cqParams string) []map[string]any {
	content := make([]map[string]any, 0)

	switch cqType {
	case "image":
		// 图片消息
		imageURL := p.extractParam(cqParams, "url")
		if imageURL == "" {
			imageURL = p.extractParam(cqParams, "file")
		}
		if imageURL != "" {
			content = append(content, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url":    imageURL,
					"detail": "auto",
				},
			})
		}
	case "file":
		// 文件消息，检查是否是图片文件
		fileName := p.extractParam(cqParams, "file")
		imageURL := p.extractParam(cqParams, "url")

		// 检查文件扩展名是否为图片
		if strings.HasSuffix(strings.ToLower(fileName), ".png") ||
			strings.HasSuffix(strings.ToLower(fileName), ".jpg") ||
			strings.HasSuffix(strings.ToLower(fileName), ".jpeg") ||
			strings.HasSuffix(strings.ToLower(fileName), ".gif") ||
			strings.HasSuffix(strings.ToLower(fileName), ".webp") {
			if imageURL != "" {
				content = append(content, map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url":    imageURL,
						"detail": "auto",
					},
				})
			}
		} else {
			content = append(content, map[string]any{
				"type": "text",
				"text": "[文件]",
			})
		}
	case "at":
		// @提及消息
		qq := p.extractParam(cqParams, "qq")
		if len(strings.TrimSpace(qq)) > 3 {
			content = append(content, map[string]any{
				"type": "text",
				"text": fmt.Sprintf("@%s", qq),
			})
		}
	case "forward":
		// 转发消息
		forwardID := p.extractParam(cqParams, "id")
		if forwardID != "" {
			// 获取转发消息内容
			forwardContent, err := p.getForwardMessage(forwardID)
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
		content = append(content, map[string]any{
			"type": "text",
			"text": "[CQ码]",
		})
	}

	return content
}

// extractParam 提取CQ码参数
func (p *Processor) extractParam(params string, key string) string {
	r := regexp.MustCompile(`(?i)` + key + `=([^,\]]+)`)
	match := r.FindStringSubmatch(params)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

// getForwardMessage 获取转发消息内容
func (p *Processor) getForwardMessage(forwardID string) ([]map[string]any, error) {
	// 创建请求参数
	params := map[string]interface{}{
		"message_id": forwardID,
	}

	// 发送消息
	echo, err := p.wsClient.SendMessage("get_forward_msg", params)
	if err != nil {
		return nil, fmt.Errorf("发送get_forward_msg请求失败: %v", err)
	}

	// 等待响应
	for {
		messageBytes, err := p.wsClient.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("读取消息失败: %v", err)
		}

		// 解析响应
		var response websocket.WSResponse
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
						content := p.processForwardMessages(messages)
						return content, nil
					}
				}
				return nil, fmt.Errorf("获取转发消息失败: 数据格式错误")
			}
			return nil, fmt.Errorf("获取转发消息失败: %s", response.Message)
		}
	}
}

// processForwardMessages 处理转发消息列表
func (p *Processor) processForwardMessages(messages []any) []map[string]any {
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
			messageContent := p.processSingleForwardMessage(msgMap)
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
func (p *Processor) processSingleForwardMessage(msgMap map[string]any) []map[string]any {
	content := make([]map[string]any, 0)

	// 提取发送者信息
	senderName := p.extractSenderName(msgMap)

	// 添加发送者信息
	if len(strings.TrimSpace(senderName)) > 3 {
		content = append(content, map[string]any{
			"type": "text",
			"text": fmt.Sprintf("[发言人:%s]: ", senderName),
		})
	}

	// 处理消息内容
	if messageField, ok := msgMap["message"]; ok {
		messageContent := p.processMessageContent(messageField)
		content = append(content, messageContent...)
	}

	return content
}

// extractSenderName 提取发送者信息
func (p *Processor) extractSenderName(msgMap map[string]any) string {
	senderName := ""
	if sender, ok := msgMap["sender"].(map[string]any); ok {
		if card, ok := sender["card"].(string); ok && card != "" {
			senderName = card
		} else if nickname, ok := sender["nickname"].(string); ok {
			senderName = nickname
		}
	}
	return senderName
}

// processMessageContent 处理消息内容
func (p *Processor) processMessageContent(messageField interface{}) []map[string]any {
	content := make([]map[string]any, 0)

	switch msgContent := messageField.(type) {
	case string:
		// 字符串格式的消息，处理其中的CQ码
		cqContents := p.Process(msgContent)
		if len(cqContents) > 0 {
			content = append(content, cqContents...)
		} else if len(strings.TrimSpace(msgContent)) > 3 {
			content = append(content, map[string]any{
				"type": "text",
				"text": msgContent,
			})
		}
	case []any:
		// 数组格式的消息
		for _, item := range msgContent {
			if itemMap, ok := item.(map[string]any); ok {
				itemContent := p.processArrayMessageItem(itemMap)
				content = append(content, itemContent...)
			}
		}
	}

	return content
}

// processArrayMessageItem 处理数组格式的消息项
func (p *Processor) processArrayMessageItem(itemMap map[string]any) []map[string]any {
	content := make([]map[string]any, 0)

	msgType := ""
	if t, ok := itemMap["type"].(string); ok {
		msgType = t
	}

	switch msgType {
	case "text":
		// 文本消息
		if msgData, ok := itemMap["data"].(map[string]any); ok {
			if text, ok := msgData["text"].(string); ok {
				// 处理文本中的CQ码
				cqContents := p.Process(text)
				if len(cqContents) > 0 {
					content = append(content, cqContents...)
				} else if len(strings.TrimSpace(text)) > 3 {
					content = append(content, map[string]any{
						"type": "text",
						"text": text,
					})
				}
			}
		}
	case "image":
		// 图片消息
		if msgData, ok := itemMap["data"].(map[string]any); ok {
			imageURL := p.extractImageURL(msgData)
			if imageURL != "" {
				content = append(content, map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url":    imageURL,
						"detail": "auto",
					},
				})
			}
		}
	case "at":
		// @提及消息
		if msgData, ok := itemMap["data"].(map[string]any); ok {
			atContent := p.processAtMessage(msgData)
			content = append(content, atContent...)
		}
	case "file":
		// 文件消息
		content = append(content, map[string]any{
			"type": "text",
			"text": "[文件]",
		})
	case "reply":
		// 引用消息
		content = append(content, map[string]any{
			"type": "text",
			"text": "[引用消息]",
		})
	}

	return content
}

// extractImageURL 提取图片URL并处理HTML实体
func (p *Processor) extractImageURL(msgData map[string]any) string {
	return utils.ExtractImageURL(msgData)
}

// processAtMessage 处理@提及消息
func (p *Processor) processAtMessage(msgData map[string]any) []map[string]any {
	content := make([]map[string]any, 0)

	qq := ""
	if qqVal, ok := msgData["qq"].(string); ok {
		qq = qqVal
	} else if qqVal, ok := msgData["qq"].(float64); ok {
		qq = fmt.Sprintf("%d", int64(qqVal))
	}

	atText := fmt.Sprintf("@%s", qq)
	if len(strings.TrimSpace(atText)) > 3 {
		content = append(content, map[string]any{
			"type": "text",
			"text": atText,
		})
	}

	return content
}
