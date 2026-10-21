package BilibiliLife

// 生活链 · B 站真实浏览器版：模型调用（OpenAI 兼容 /chat/completions）。
//
// 与 goja 版不同，这里直接由 Go 发请求：文本用于评论/动态/回复，
// 图像序列帧用于 60 秒切片理解。模型地址与多模态模型沿用 lunar_config.json。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/GeneralConfig"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type imageURL struct {
	URL string `json:"url"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

var (
	modelOnce  sync.Once
	modelName  string
	modelURL   string
	modelKey   string
	httpClient = &http.Client{Timeout: 5 * time.Minute}
)

func resolveModel() {
	modelOnce.Do(func() {
		modelName = strings.TrimSpace(*GeneralConfig.AgentMultimodalModel)
		if modelName == "" {
			modelName = "system-multimodal"
		}
		modelURL = strings.TrimRight(strings.TrimSpace(*GeneralConfig.AgentMultimodalURL), "/")
		if modelURL == "" {
			modelURL = GeneralConfig.LocalV1URL()
		}
		if !strings.HasSuffix(modelURL, "/v1") {
			modelURL += "/v1"
		}
		modelKey = strings.TrimSpace(*GeneralConfig.AgentMultimodalKey)
	})
}

func chatCompletion(messages []chatMessage) (string, error) {
	resolveModel()
	body, err := json.Marshal(map[string]any{
		"model":    modelName,
		"messages": messages,
		"stream":   false,
	})
	if err != nil {
		return "", fmt.Errorf("序列化模型请求失败: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, modelURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("创建模型请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if modelKey != "" {
		req.Header.Set("Authorization", "Bearer "+modelKey)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取模型响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("模型 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 240))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("解析模型响应失败: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("模型错误: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("模型返回空 choices")
	}
	text := strings.TrimSpace(out.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("模型返回空内容")
	}
	return text, nil
}

func chatText(system, user string) (string, error) {
	return chatCompletion([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
}

func chatWithImages(system, prompt string, images []string) (string, error) {
	parts := make([]contentPart, 0, len(images)+1)
	parts = append(parts, contentPart{Type: "text", Text: prompt})
	for _, image := range images {
		if strings.TrimSpace(image) == "" {
			continue
		}
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: image}})
	}
	return chatCompletion([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: parts},
	})
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
