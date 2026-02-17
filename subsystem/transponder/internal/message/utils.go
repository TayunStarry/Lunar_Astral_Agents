package message

import (
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strings"
	status "transponder/internal/config"
	"transponder/internal/utils"
)

// getImageURL 获取图片URL
func (class *Processor) getImageURL(data map[string]any) string {
	var imageURL string
	// 尝试从不同的字段获取图片URL
	if url, ok := data["url"].(string); ok {
		// 处理HTML实体
		imageURL = utils.ProcessImageURL(url)
	} else if _, ok := data["file"].(string); ok {
		// 如果只有文件路径，使用占位符
		imageURL = "http://localhost/placeholder.jpg"
	}
	return imageURL
}

// HasImageExtension 检查文件名是否为图片扩展名
func (class *Processor) HasImageExtension(fileName string) bool {
	imageExtensions := []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}
	lowerFileName := strings.ToLower(fileName)
	isImage := false
	for _, ext := range imageExtensions {
		if strings.HasSuffix(lowerFileName, ext) {
			isImage = true
			break
		}
	}
	return isImage
}

// Process 处理包含CQ码的文本
func (class *Processor) Process(text string) []map[string]any {
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
		cqContent := class.processCQCode(cqType, cqParams)
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

// extractParam 提取CQ码参数
func (class *Processor) extractParam(params string, key string) string {
	r := regexp.MustCompile(`(?i)` + key + `=([^,\]]+)`)
	match := r.FindStringSubmatch(params)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

// extractSenderName 提取发送者信息
func (class *Processor) extractSenderName(msgMap map[string]any) string {
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

// getSenderName 获取发送者昵称
func (class *Processor) getSenderName(rawMsg map[string]any) string {
	if sender, ok := rawMsg["sender"].(map[string]interface{}); ok {
		// 优先使用群名片
		if card, ok := sender["card"].(string); ok && card != "" {
			return card
		} else if nickname, ok := sender["nickname"].(string); ok {
			return nickname
		}
	}
	return ""
}

// getUserName 获取用户昵称
func (class *Processor) getUserName(groupID int64, userID int64) string {
	// 尝试获取用户昵称
	nickname := ""
	if groupID > 0 && userID > 0 {
		if members, ok := class.groupMembers[groupID]; ok {
			if name, ok := members[userID]; ok {
				nickname = name
			}
		}
	}

	// 如果找不到昵称，使用用户ID
	if nickname == "" {
		nickname = fmt.Sprintf("%d", userID)
	}

	return nickname
}

// containsTriggerKeyword 检查消息是否包含触发关键词
func (class *Processor) containsTriggerKeyword(message string) bool {
	for _, keyword := range class.config.TriggerKeywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

// SendGroupMsg 发送群消息
func (class *Processor) SendGroupMsg(groupID int64, content string) error {
	// 创建消息项
	message := []status.MsgItem{
		{
			Type: "text",
			Data: map[string]string{
				"text": content,
			},
		},
	}

	// 创建请求参数
	params := SendGroupMsgParams{
		GroupID: groupID,
		Message: message,
	}

	// 发送消息
	_, err := class.wsClient.SendMessage("send_group_msg", params)
	if err != nil {
		return fmt.Errorf("发送群消息失败: %v", err)
	}

	log.Printf("已回应< QQ群 %d >的消息", groupID)
	// 15%概率触发表情包发送
	if rand.Intn(100) < 15 {
		// 生成响应文本的嵌入向量
		embedding, err := class.generateEmbedding(content)
		if err != nil {
			log.Printf("生成嵌入向量失败: %v", err)
			return nil
		}

		// 查询知识库获取相关图片
		knowledgeMsg, err := class.queryKnowledgeBase(embedding)
		if err != nil {
			log.Printf("查询知识库失败: %v", err)
			return nil
		}

		// 发送图片消息
		if knowledgeMsg != nil && knowledgeMsg.ImageUrl != "" {
			if err := class.SendGroupImageMsg(groupID, knowledgeMsg.ImageUrl); err != nil {
				log.Printf("发送图片消息失败: %v", err)
			}
		}
	}
	return nil
}
