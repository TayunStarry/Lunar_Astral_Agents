package processor

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"subsystem/component/setup"
	"subsystem/component/utils"
)

// getImageURL 获取图片URL
func (class *Handle) getImageURL(data ImageObjectParameter) string {
	var imageURL string
	// 尝试从不同的字段获取图片URL
	if data.URL != "" {
		// 处理HTML实体
		imageURL = utils.ProcessImageURL(data.URL)
	} else if data.File != "" {
		// 如果只有文件路径，使用占位符
		imageURL = class.BaseURL + "/read/resources/placeholder/blank-02.png"
	}
	return imageURL
}

// HasImageExtension 检查文件名是否为图片扩展名
func (class *Handle) HasImageExtension(fileName string) bool {
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
func (class *Handle) Process(text string) ProcessResult {
	content := make(ProcessResult, 0)

	// 查找所有CQ码
	r := regexp.MustCompile(`\[CQ:([^,\]]+)([^\]]*)\]`)
	matches := r.FindAllStringSubmatchIndex(text, -1)

	if len(matches) == 0 {
		// 没有找到CQ码，返回原文本
		content = append(content, TextMessage{
			Type: "text",
			Text: text,
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
				content = append(content, TextMessage{
					Type: "text",
					Text: prefixText,
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
			content = append(content, TextMessage{
				Type: "text",
				Text: suffixText,
			})
		}
	}

	return content
}

// extractParam 提取CQ码参数
func (class *Handle) extractParam(params string, key string) string {
	r := regexp.MustCompile(`(?i)` + key + `=([^,\]]+)`)
	match := r.FindStringSubmatch(params)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

// extractSenderName 提取发送者信息
func (class *Handle) extractSenderName(msgMap map[string]any) string {
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
func (class *Handle) getSenderName(rawMsg map[string]any) string {
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
func (class *Handle) getUserName(groupID int64, userID int64) string {
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

// generateEmbedding 生成文本嵌入向量
func (class *Handle) generateEmbedding(text string) ([]float64, error) {
	// 构建请求体
	requestBody := EmbeddingRequestBody{
		Model:          "system-embedding",
		Input:          []string{text},
		TaskType:       "search_document",
		Dimensionality: 256,
	}
	// 编码请求体为JSON
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("编码请求体失败: %v", err)
	}

	// 构建完整的请求URL
	requestURL := class.BaseURL + "/v1/embeddings"
	log.Printf("调用< %s >进行处理", requestURL)

	// 创建请求
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 检查HTTP响应状态码
	if resp.StatusCode != http.StatusOK {
		log.Printf("嵌入API返回非200状态码: %d", resp.StatusCode)
		log.Printf("响应内容: %s", string(respBody))
		return nil, fmt.Errorf("嵌入API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 检查响应体是否为空
	if len(respBody) == 0 {
		return nil, fmt.Errorf("嵌入API返回空响应")
	}

	// 检查响应体是否为有效的JSON
	var jsonTest interface{}
	if err := json.Unmarshal(respBody, &jsonTest); err != nil {
		log.Printf("嵌入API返回非JSON响应: %s", string(respBody))
		return nil, fmt.Errorf("嵌入API返回非JSON响应: %s, 错误: %v", string(respBody), err)
	}

	// 解析响应
	var embeddingResp EmbeddingResponse
	if err := json.Unmarshal(respBody, &embeddingResp); err != nil {
		log.Printf("嵌入API响应解析失败，响应内容: %s", string(respBody))
		return nil, fmt.Errorf("解析响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	// 检查响应数据
	if len(embeddingResp.Data) == 0 {
		return nil, fmt.Errorf("响应中没有嵌入数据")
	}

	// 返回嵌入向量的前256个元素
	embedding := embeddingResp.Data[0].Embedding
	if len(embedding) > 256 {
		embedding = embedding[:256]
	}
	return embedding, nil
}

// ContainsTriggerKeyword 检查消息是否包含触发关键词
func (class *Handle) ContainsTriggerKeyword(message string) bool {
	for _, keyword := range class.Config.TriggerKeywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

// SendGroupMsg 发送群消息
func (class *Handle) SendGroupMsg(groupID int64, content string) error {
	// 创建消息项
	message := []setup.MessageItem{{Type: "text", Data: setup.MessageItemData{Text: content}}}

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

// SendGroupImageMsg 发送群图片消息
func (class *Handle) SendGroupImageMsg(groupID int64, imageUrl string) error {
	fullImageUrl := class.BaseURL + imageUrl
	log.Printf("发送图片URL: %s", fullImageUrl)

	// 创建图片消息项
	message := []setup.MessageItem{
		{
			Type: "image",
			Data: setup.MessageItemData{Url: fullImageUrl, File: fullImageUrl},
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
		return fmt.Errorf("发送群图片消息失败: %v", err)
	}

	log.Printf("已向< QQ群 %d >发送图片消息", groupID)
	return nil
}
