//go:build windows

package AutoLTP

import (
	"LunarSubsystem/GeneralConfig"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// autoConfig 惰性加载模型配置（模型名/URL/Key），仅首次调用时解析一次。
func autoConfig() {
	autoConfigOnce.Do(func() {
		autoModel = stringOr(*GeneralConfig.AgentMultimodalModel, "system-multimodal")
		autoURL = normV1(stringOr(*GeneralConfig.AgentMultimodalURL, "http://127.0.0.1:36789/v1"))
		autoKey = stringOr(*GeneralConfig.AgentMultimodalKey, "")
	})
}

// stringOr 返回第一个非空字符串，全空则返回默认值。
func stringOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// normV1 确保模型请求地址以 /v1 结尾，缺省时自动补全。
func normV1(raw string) string {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	if strings.HasSuffix(u, "/v1") {
		return u
	}
	return u + "/v1"
}

// autoChat 向模型发起一次带工具的对话请求，返回助手消息（含可能的工具调用）。
func autoChat(messages []chatMessage, tools []ltpToolDef) (chatMessage, error) {
	autoConfig()
	body := ltpChatRequest{Model: autoModel, Messages: messages, Tools: tools, Stream: false}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return chatMessage{}, fmt.Errorf("序列化聊天请求失败: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, autoURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return chatMessage{}, fmt.Errorf("创建聊天请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if autoKey != "" {
		req.Header.Set("Authorization", "Bearer "+autoKey)
	}
	resp, err := autoHTTPClient.Do(req)
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
	var chatResp ltpChatResponse
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

// autoChatText 向模型发起纯文本对话并返回助手消息的文本内容。
func autoChatText(messages []chatMessage) (string, error) {
	r, err := autoChat(messages, nil)
	if err != nil {
		return "", err
	}
	return messageText(r.Content), nil
}

// messageText 从消息 content（字符串或片段数组）中提取纯文本。
func messageText(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, p := range v {
			if m, ok := p.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// truncateStr 以 rune 为单位截断字符串，超长部分以省略号结尾。
func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
