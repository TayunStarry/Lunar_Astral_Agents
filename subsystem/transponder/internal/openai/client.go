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
	"transponder/internal/config"
	"transponder/internal/message"
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
func (class *Client) CallAPI(messages []Message, processor *message.Processor) (string, error) {
	// 创建请求
	request := Request{
		Model:      class.model,
		Messages:   messages,
		Tools:      GetTools(),
		ToolChoice: "auto",
	}

	// 序列化请求
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(request); err != nil {
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}
	requestJSON := strings.TrimSpace(buffer.String())

	log.Printf("调用< API : %s >进行处理", class.apiURL)

	if config.DisplayDebugMessage {
		log.Printf("%s", requestJSON)
	}

	// 创建HTTP请求
	httpReq, err := http.NewRequest("POST", class.apiURL, strings.NewReader(requestJSON))
	if err != nil {
		return "", fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	if class.token != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", class.token))
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

		// 检查是否有工具调用
		if len(message.ToolCalls) > 0 {
			// 执行工具调用
			for _, toolCall := range message.ToolCalls {
				log.Printf("执行工具: %s", toolCall.Function.Name)
				// 执行工具
				result, err := ExecuteTool(toolCall, processor)
				if err != nil {
					log.Printf("工具执行失败: %v", err)
					result = fmt.Sprintf("工具执行失败: %v", err)
				}
				// 创建工具响应消息
				toolResponse := Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: toolCall.ID,
					Name:       toolCall.Function.Name,
				}
				// 将工具响应添加到消息列表
				messages = append(messages, toolResponse)
				log.Printf("工具响应: %s", result)
			}
			// 递归调用API，继续处理工具响应
			return class.CallAPI(messages, processor)
		}

		// 处理不同类型的响应内容
		switch content := message.Content.(type) {
		case string:
			if content == "" {
				return "㊥", nil
			}
			return content, nil
		case map[string]any:
			// 如果是对象，尝试提取文本
			if text, ok := content["text"].(string); ok {
				if text == "" {
					return "㊥", nil
				}
				return text, nil
			}
		}

		// 作为最后手段，尝试将内容转换为字符串
		result := fmt.Sprintf("%v", message.Content)
		if result == "" {
			return "㊥", nil
		}
		return result, nil
	}

	return "㊥", nil
}

// CreateMessage 创建消息
func CreateMessage(role string, content any) Message {
	return Message{
		Role:    role,
		Content: content,
	}
}
