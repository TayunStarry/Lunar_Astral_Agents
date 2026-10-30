package textcall

// textcall.go 生活体系与记忆整理共用的纯文本模型调用。
//
// 只做一次非流式 chat/completions：调用方需要的是完整结构化结果，
// 不需要把逐字输出推给 TUI。模型地址与密钥沿用 lunar_config.json 的 agent 分组。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/model/loopguard"
)

// Complete 用指定模型跑一次 system+user 文本对话。
func Complete(ctx context.Context, model, system, user string) (string, error) {
	return CompleteMessages(ctx, model, []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
}

// CompleteMessages 用指定模型跑一次多消息文本对话。
func CompleteMessages(ctx context.Context, model string, messages []Message) (string, error) {
	if strings.TrimSpace(model) == "" {
		model = config.AgentMultimodalModel
	}
	baseURL := config.AgentMultimodalURL
	body, err := json.Marshal(completionRequest{Model: model, Messages: messages, Stream: false})
	if err != nil {
		return "", fmt.Errorf("序列化模型请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("创建模型请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
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
		return "", fmt.Errorf("模型 HTTP %d: %s", resp.StatusCode, truncate(string(raw), maxErrorBody))
	}
	var out completionResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("解析模型响应失败: %w", err)
	}
	if out.Error != nil && strings.TrimSpace(out.Error.Message) != "" {
		return "", fmt.Errorf("模型错误: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("模型返回空 choices")
	}
	text := strings.TrimSpace(out.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("模型返回空内容")
	}
	if loopguard.Detected(text, loopguard.Options{}) {
		return "", loopguard.ErrLoop
	}
	return text, nil
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
