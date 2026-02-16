package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"nap_cat_bridging/internal/config"
)



// NewClient 创建新的OpenAI客户端
func NewClient(cfg *config.Config) *Client {
	return &Client{
		apiURL: cfg.OpenAIAPIUrl,
		token:  cfg.OpenAIAPIToken,
		model:  cfg.OpenAIAPIModel,
	}
}

// CallAPI 调用OpenAI API
func (c *Client) CallAPI(messages []Message) (string, error) {
	// 创建请求
	request := Request{
		Model:    c.model,
		Messages: messages,
	}

	// 序列化请求
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(request); err != nil {
		config.DisplayDebugMessage = true
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}
	requestJSON := strings.TrimSpace(buffer.String())

	log.Printf("调用 OpenAI API 进行回复")

	if config.DisplayDebugMessage {
		log.Printf("%s", requestJSON)
	}

	// 创建HTTP请求
	httpReq, err := http.NewRequest("POST", c.apiURL, strings.NewReader(requestJSON))
	if err != nil {
		return "", fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	}

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second, // 设置30秒超时
	}
	response, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %v", err)
	}
	defer response.Body.Close()

	// 读取响应
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	// 检查响应状态码
	if response.StatusCode != http.StatusOK {
		log.Printf("OpenAI API返回错误状态码: %d, 响应内容: %s", response.StatusCode, string(responseBody))
		return "", fmt.Errorf("OpenAI API返回错误状态码: %d", response.StatusCode)
	}

	// 解析响应
	var openAIResponse Response
	if err := json.Unmarshal(responseBody, &openAIResponse); err != nil {
		log.Printf("解析OpenAI API响应失败: %v, 响应内容: %s", err, string(responseBody))
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	// 提取助手回复
	if len(openAIResponse.Choices) > 0 {
		message := openAIResponse.Choices[0].Message

		// 处理不同类型的响应内容
		switch content := message.Content.(type) {
		case string:
			return content, nil
		case map[string]any:
			// 如果是对象，尝试提取文本
			if text, ok := content["text"].(string); ok {
				return text, nil
			}
		}

		// 作为最后手段，尝试将内容转换为字符串
		return fmt.Sprintf("%v", message.Content), nil
	}

	return "", fmt.Errorf("没有获取到助手回复")
}

// CreateMessage 创建消息
func CreateMessage(role string, content any) Message {
	return Message{
		Role:    role,
		Content: content,
	}
}

// CreateImageMessage 创建图片消息
func CreateImageMessage(url string) Message {
	imageContent := ImageContent{
		Type: "image_url",
		ImageURL: map[string]string{
			"url":    url,
			"detail": "auto",
		},
	}
	return Message{
		Role:    "user",
		Content: imageContent,
	}
}
