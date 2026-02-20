package processor

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"transponder/internal/setup"
	"transponder/internal/utils"
)

// ParseMessageResponse 解析消息响应
func (class *Handle) ParseMessageResponse(data map[string]any) (map[string]any, error) {
	// 提取发送者信息
	senderName := class.getSenderName(data)
	// 处理消息内容
	content := class.ProcessOriginalMessageContent(data)
	// 返回结果
	return map[string]any{"sender": senderName, "content": content}, nil
}

// ProcessMessageContent 处理消息内容
func (class *Handle) ProcessMessageContent(rawMsg map[string]any, groupID int64, senderName string) ProcessResult {
	// 提取并转换消息内容
	content := make(ProcessResult, 0)
	// 添加发言人信息作为第一个文本元素
	content = append(content, TextMessage{Type: "text", Text: fmt.Sprintf("[发言人:%s]: ", senderName)})
	// 提取message字段
	if msgField, ok := rawMsg["message"]; ok {
		switch msg := msgField.(type) {
		// 消息是数组格式
		case []any:
			for _, item := range msg {
				content = append(content, class.ConstructChatContent(item)...)
			}
		// 消息是字符串格式（如CQ码），创建一个文本消息项
		case string:
			msgItem := setup.MessageItem{Type: "text", Data: setup.MessageItemData{Text: msg}}
			content = append(content, class.universalCodeEscape(msgItem)...)
		}
	}
	// 如果没有内容，返回空字符串
	if len(content) == 0 {
		return ProcessResult{TextMessage{Type: "text", Text: class.Config.DefaultReply}}
	}
	// 如果只有一个文本消息，直接返回字符串
	if len(content) == 1 {
		if textMsg, ok := content[0].(TextMessage); ok {
			return ProcessResult{TextMessage{Type: "text", Text: textMsg.Text}}
		}
	}
	return content
}

// ProcessOriginalMessageContent 处理原始消息内容
func (class *Handle) ProcessOriginalMessageContent(data map[string]any) ProcessResult {
	var content ProcessResult
	// 处理message字段
	if messageField, ok := data["message"]; ok {
		switch msg := messageField.(type) {
		// 消息是数组格式
		case []any:
			content = make(ProcessResult, 0)
			for _, item := range msg {
				content = append(content, class.ConstructChatContent(item)...)
			}
		// 消息是字符串格式（如CQ码），处理其中的CQ码
		case string:
			cqContents := class.Process(msg)
			if len(cqContents) > 0 {
				content = cqContents
			} else if len(strings.TrimSpace(msg)) > 0 {
				content = ProcessResult{TextMessage{Type: "text", Text: msg}}
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

// ConstructChatContent 构造聊天内容
func (class *Handle) ConstructChatContent(item any) ProcessResult {
	content := make(ProcessResult, 0)
	if itemMap, ok := item.(map[string]any); ok {
		if msgType, ok := itemMap["type"].(string); ok {
			msgItem := setup.MessageItem{Type: msgType, Data: setup.MessageItemData{}}
			msgItem.Data = class.CaptureChatData(itemMap)
			content = append(content, class.universalCodeEscape(msgItem)...)
		}
	}
	return content
}

// CaptureChatData 捕获聊天数据
func (class *Handle) CaptureChatData(itemMap map[string]any) setup.MessageItemData {
	chatData := setup.MessageItemData{}
	if dataMap, ok := itemMap["data"].(map[string]any); ok {
		if text, ok := dataMap["text"].(string); ok {
			chatData.Text = text
		}
		if url, ok := dataMap["url"].(string); ok {
			chatData.Url = url
		}
		if file, ok := dataMap["file"].(string); ok {
			chatData.File = file
		}
		if qq, ok := dataMap["qq"].(string); ok {
			chatData.QQ = qq
		}
		if id, ok := dataMap["id"].(string); ok {
			chatData.ID = id
		}
	}
	return chatData
}

// ProcessRawMessage 处理原始消息作为 fallback
func (class *Handle) ProcessRawMessage(rawMessage string) ProcessResult {
	// 处理raw_message中的CQ码
	cqContents := class.Process(rawMessage)
	if len(cqContents) > 0 {
		return cqContents
	}
	// 如果没有CQ码，创建一个文本消息项
	return ProcessResult{TextMessage{Type: "text", Text: rawMessage}}
}

// ProcessMessageContent 处理消息内容
func (class *Handle) ProcessMessageFieldContent(messageField any) ProcessResult {
	content := make(ProcessResult, 0)
	switch msgContent := messageField.(type) {
	case string:
		// 字符串格式的消息，处理其中的CQ码
		cqContents := class.Process(msgContent)
		if len(cqContents) > 0 {
			content = append(content, cqContents...)
		} else if len(strings.TrimSpace(msgContent)) > 0 {
			content = append(content, TextMessage{Type: "text", Text: msgContent})
		}
	case []any:
		// 数组格式的消息
		for _, item := range msgContent {
			content = append(content, class.ConstructChatContent(item)...)
		}
	}
	return content
}

// HandleGroupMessage 处理群消息
func (class *Handle) HandleGroupMessage(message []byte) (int64, any, error) {
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
	// 设置当前处理的群ID
	class.currentGroupID = groupID
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
	for _, gid := range class.Config.ListenGroupIDs {
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
	if !class.ContainsTriggerKeyword(rawMessage) {
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
