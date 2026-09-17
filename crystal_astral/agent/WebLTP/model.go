package WebLTP

// Web-LTP 模型调用（OpenAI v1 协议，复用月华多模态模型配置）

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LoggerGeneralWarn 统一日志入口
func LoggerGeneralWarn(format string, args ...any) {
	LoggerGeneral.Warn("CrystalAstral", format, args...)
}

var (
	modelOnce  sync.Once
	modelName  string
	modelURL   string
	modelKey   string
	httpClient = &http.Client{Timeout: 120 * time.Second}
)

func resolveModel() {
	modelOnce.Do(func() {
		modelName = strings.TrimSpace(*GeneralConfig.AgentMultimodalModel)
		if modelName == "" {
			modelName = "system-multimodal"
		}
		modelURL = strings.TrimRight(strings.TrimSpace(*GeneralConfig.AgentMultimodalURL), "/")
		if modelURL == "" {
			modelURL = "http://127.0.0.1:36789/v1"
		}
		if !strings.HasSuffix(modelURL, "/v1") {
			modelURL += "/v1"
		}
		modelKey = strings.TrimSpace(*GeneralConfig.AgentMultimodalKey)
	})
}

// textWithImage 构建图文混排的用户消息内容（dataURL 为空时退化为纯文本）
func textWithImage(text, dataURL string) []contentPart {
	parts := []contentPart{{Type: "text", Text: text}}
	if dataURL != "" {
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: dataURL}})
	}
	return parts
}

// chatText 发起一次纯文本对话并返回助手文本
func chatText(messages []chatMessage) (string, error) {
	resolveModel()
	body, err := json.Marshal(map[string]any{
		"model":    modelName,
		"messages": messages,
		"stream":   false,
	})
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, modelURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if modelKey != "" {
		req.Header.Set("Authorization", "Bearer "+modelKey)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("模型 API 返回状态 %d: %s", resp.StatusCode, truncateRunes(string(respBody), 200))
	}
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}
	if chatResp.Error != nil {
		return "", fmt.Errorf("模型 API 错误: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 || strings.TrimSpace(chatResp.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("模型 API 返回空响应")
	}
	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// ExtractQuery 把月华的自然语言指令提炼为必应检索词；失败时返回原指令。
func ExtractQuery(instruction string) string {
	q, err := chatText([]chatMessage{
		{Role: "system", Content: "把用户的自然语言指令提炼为一条简洁的必应检索词：保留关键实体与意图，中文优先，去掉客套与动词指令。只输出检索词本身，不要输出任何其他内容。"},
		{Role: "user", Content: instruction},
	})
	if err != nil {
		return instruction
	}
	q = strings.TrimSpace(q)
	if q == "" || len([]rune(q)) > 120 {
		return instruction
	}
	return strings.Trim(q, "\"“”「」")
}

// hardCutRunes 硬切断文本：总长不超过 max 字符（超长时以省略号收尾并标注）
func hardCutRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 1 {
		max = 1
	}
	return string(r[:max-1]) + "…（已截断）"
}

// truncateRunes 简单截断（用于日志/载荷）
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
