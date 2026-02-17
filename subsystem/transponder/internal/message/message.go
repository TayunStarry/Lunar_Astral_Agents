package message

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	status "transponder/internal/config"
	"transponder/internal/utils"
)

// ParseMessageResponse 解析消息响应
func (class *Processor) ParseMessageResponse(data map[string]any) (map[string]any, error) {
	// 提取发送者信息
	senderName := class.getSenderName(data)

	// 处理消息内容
	content := class.ProcessOriginalMessageContent(data)

	// 返回结果
	return map[string]any{
		"sender":  senderName,
		"content": content,
	}, nil
}

// ProcessMessageContent 处理消息内容
func (class *Processor) ProcessMessageContent(rawMsg map[string]any, groupID int64, senderName string) any {
	// 提取message字段
	var msgItems []status.MsgItem
	if msgField, ok := rawMsg["message"]; ok {
		switch msg := msgField.(type) {
		case []any:
			// 消息是数组格式
			for _, item := range msg {
				if msgMap, ok := item.(map[string]any); ok {
					msgItem := status.MsgItem{
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
			msgItems = []status.MsgItem{
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
		content = append(content, class.processMessageItem(msgMap, map[string]any{"group_id": float64(groupID)})...)
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
func (class *Processor) ProcessOriginalMessageContent(data map[string]any) []map[string]any {
	var content []map[string]any
	if messageField, ok := data["message"]; ok {
		switch msg := messageField.(type) {
		case []any:
			// 消息是数组格式
			content = make([]map[string]any, 0)
			for _, item := range msg {
				content = append(content, class.processMessageItem(item, data)...)
			}
		case string:
			// 消息是字符串格式（如CQ码），处理其中的CQ码
			cqContents := class.Process(msg)
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
			content = class.ProcessRawMessage(rawMessage)
		}
	}

	return content
}

// ProcessRawMessage 处理原始消息作为 fallback
func (class *Processor) ProcessRawMessage(rawMessage string) []map[string]any {
	// 处理raw_message中的CQ码
	cqContents := class.Process(rawMessage)
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

// processMessageContent 处理消息内容
func (class *Processor) processMessageContent(messageField interface{}) []map[string]any {
	content := make([]map[string]any, 0)

	switch msgContent := messageField.(type) {
	case string:
		// 字符串格式的消息，处理其中的CQ码
		cqContents := class.Process(msgContent)
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
				itemContent := class.processArrayMessageItem(itemMap)
				content = append(content, itemContent...)
			}
		}
	}

	return content
}

// HandleGroupMessage 处理群消息
func (class *Processor) HandleGroupMessage(message []byte) (int64, any, error) {
	// 首先解析为原始消息
	var rawMsg map[string]any
	if err := json.Unmarshal(message, &rawMsg); err != nil {
		return 0, "", fmt.Errorf("解析消息失败: %v", err)
	}

	// 检查是否是群消息
	if postType, ok := rawMsg["post_type"].(string); !ok || postType != "message" {
		return 0, "", fmt.Errorf("不是消息")
	}
	if msgType, ok := rawMsg["message_type"].(string); !ok || msgType != "group" {
		return 0, "", fmt.Errorf("不是群消息")
	}

	// 提取基本信息
	groupID := int64(utils.GetFloat64Value(rawMsg, "group_id"))
	selfID := int64(utils.GetFloat64Value(rawMsg, "self_id"))
	senderUserID := int64(0)

	if sender, ok := rawMsg["sender"].(map[string]interface{}); ok {
		senderUserID = int64(utils.GetFloat64Value(sender, "user_id"))
	}

	// 提取发送者昵称
	senderName := class.getSenderName(rawMsg)

	// 提取原始消息内容
	rawMessage := ""
	if rm, ok := rawMsg["raw_message"].(string); ok {
		rawMessage = rm
	}

	log.Printf("%s", strings.Repeat("-=", 28))
	log.Printf("接收到来自< QQ群 %d >的消息 | 发送者: %s", groupID, senderName)
	log.Printf("消息内容: %s", rawMessage)

	// 群过滤：若group_id不属于配置的目标群，则直接忽略该消息
	groupFound := false
	for _, gid := range class.config.ListenGroupIDs {
		if gid == groupID {
			groupFound = true
			break
		}
	}
	if !groupFound {
		log.Printf("群 ID %d 不在订阅列表中，忽略消息", groupID)
		return 0, nil, nil
	}

	// 自过滤：若sender.user_id等于< QQ 智能体 >自身的self_id，则忽略该消息
	if senderUserID == selfID {
		log.Printf("消息来自< QQ 智能体 >自身，忽略消息")
		return 0, nil, nil
	}

	// 检查是否包含触发关键词或随机触发
	if !class.containsTriggerKeyword(rawMessage) {
		// 15%概率随机触发
		randomChance := rand.Intn(100)
		if randomChance > 15 {
			log.Printf("消息不包含触发关键词，忽略消息")
			return 0, nil, nil
		}
	}

	// 处理消息内容
	messageContent := class.ProcessMessageContent(rawMsg, groupID, senderName)

	return groupID, messageContent, nil
}
