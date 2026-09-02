package FaceLTP

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// callModel 调用多模态模型（OpenAI v1 function calling，stream:false），返回模型的回复消息。
// 模型配置从 modelConfig（lunar_config.json 的 agent 字段）读取，不硬编码模型名。
func callModel(messages []chatMessage) (chatMessage, error) {
	body := chatRequest{
		Model:    modelConfig.Model,
		Messages: messages,
		Tools:    agentTools,
		Stream:   false,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return chatMessage{}, fmt.Errorf("序列化聊天请求失败: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, modelConfig.URL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return chatMessage{}, fmt.Errorf("创建聊天请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if modelConfig.Key != "" {
		req.Header.Set("Authorization", "Bearer "+modelConfig.Key)
	}

	resp, err := agentHTTPClient.Do(req)
	if err != nil {
		return chatMessage{}, fmt.Errorf("聊天请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return chatMessage{}, fmt.Errorf("读取聊天响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return chatMessage{}, fmt.Errorf("模型 API 返回状态 %d: %s", resp.StatusCode, truncateStr(string(respBody), 300))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return chatMessage{}, fmt.Errorf("解析聊天响应失败: %w", err)
	}
	if chatResp.Error != nil {
		return chatMessage{}, fmt.Errorf("模型 API 错误: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return chatMessage{}, fmt.Errorf("模型 API 返回空响应")
	}
	m := chatResp.Choices[0].Message
	return chatMessage{Role: m.Role, Content: m.Content, ToolCalls: m.ToolCalls}, nil
}

// truncateStr 截断字符串到指定长度（rune 安全）
func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}