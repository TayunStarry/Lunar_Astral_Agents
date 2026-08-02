package websearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NewOpenAIProvider 创建 OpenAI v1 协议兼容的 LLM 客户端
func NewOpenAIProvider(baseURL, apiKey, model string, maxTokens int, temperature float64) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL:     strings.TrimRight(baseURL, "/"),
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		client:      &http.Client{Timeout: 120 * time.Second},
	}
}

// Chat 发送聊天请求并获取响应文本
func (p *OpenAIProvider) Chat(messages []ChatMessage) (string, error) {
	reqBody := chatRequest{
		Model:       p.model,
		Messages:    messages,
		MaxTokens:   p.maxTokens,
		Temperature: p.temperature,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	apiURL := p.baseURL + "/chat/completions"
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 LLM 服务失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return "", fmt.Errorf("LLM 服务错误 (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return "", fmt.Errorf("LLM 服务返回异常状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("LLM 返回空响应")
	}

	return chatResp.Choices[0].Message.Content, nil
}