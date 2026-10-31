package WebSearch

// model.go 模型调用（OpenAI v1 协议，复用月华多模态模型配置）

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/model/loopguard"
)

// resolveModel 解析主模型端点（只解析一次）
func resolveModel() {
	modelOnce.Do(func() {
		modelName = config.AgentMultimodalModel
		modelURL = strings.TrimRight(config.AgentMultimodalURL, "/")
		if !strings.HasSuffix(modelURL, "/v1") {
			modelURL += "/v1"
		}
	})
}

// chatWithOptions 带思考监管的对话调用：命中重复时中断并重试一次。
// 重试后仍失败就返回错误，由各调用点的既有降级逻辑兜底
// （检索词提炼回退原指令、分片摘要跳过、最终报告走纯文本兜底）。
func chatWithOptions(messages []chatMessage, opts chatOptions) (string, error) {
	for attempt := 0; ; attempt++ {
		text, err := chatOnce(messages, opts)
		if err == nil || !errors.Is(err, loopguard.ErrLoop) || attempt >= webSearchLoopRetries {
			return text, err
		}
		loggerWarn("模型输出陷入重复，已中断并重试（%d/%d）", attempt+1, webSearchLoopRetries)
	}
}

// chatOnce 单次对话调用（流式，思考与输出同步进实况屏）。
// opts.Model 非空时覆盖模型名；采样参数与输出上限按需附加。
func chatOnce(messages []chatMessage, opts chatOptions) (string, error) {
	if !opts.Silent {
		observeBegin()
		defer observeEnd()
	}
	resolveModel()
	name := modelName
	if strings.TrimSpace(opts.Model) != "" {
		name = strings.TrimSpace(opts.Model)
	}
	payload := map[string]any{
		"model":    name,
		"messages": messages,
		"stream":   true,
		// 让服务端在流末回传 usage，模型计量才拿得到真实 token
		"stream_options": map[string]any{"include_usage": true},
	}
	if opts.Temperature != nil {
		payload["temperature"] = *opts.Temperature
	}
	if opts.TopP != nil {
		payload["top_p"] = *opts.TopP
	}
	if opts.RepeatPenalty != nil {
		payload["repeat_penalty"] = *opts.RepeatPenalty
	}
	if opts.MaxTokens > 0 {
		payload["max_tokens"] = opts.MaxTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}
	// 命中重复输出时由思考监管取消本次请求，服务端随连接断开停止生成
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, modelURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("模型 API 返回状态 %d: %s", resp.StatusCode, truncateRunes(string(respBody), 200))
	}
	// 服务端可能忽略 stream 参数直接返回 JSON：两种形态都要兼容
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		guard := loopguard.New(cancel, loopguard.DefaultOptions())
		guard.Start()
		text, reasoning, usage, readErr := readModelStream(resp.Body, opts.Silent, guard)
		guard.Stop()
		if guard.Hit() {
			return "", loopguard.ErrLoop
		}
		if readErr != nil {
			return "", readErr
		}
		reportModelCall(body, text+reasoning, usage, time.Since(start))
		return text, nil
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *modelUsage `json:"usage"`
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
	text := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if !opts.Silent {
		observeDelta("content", text)
	}
	// 非流式响应同样做一次静态重复检查
	if loopguard.Detected(chatResp.Choices[0].Message.ReasoningContent+text, loopguard.DefaultOptions()) {
		return "", loopguard.ErrLoop
	}
	reportModelCall(body, text+chatResp.Choices[0].Message.ReasoningContent, chatResp.Usage, time.Since(start))
	return text, nil
}

// readModelStream 解析 OpenAI 兼容 SSE，边收边送实况；同时取回思考文本与用量
func readModelStream(body io.Reader, silent bool, guard *loopguard.Guard) (string, string, *modelUsage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var content strings.Builder
	var reasoning strings.Builder
	var usage *modelUsage
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *modelUsage `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return "", "", nil, fmt.Errorf("模型 API 错误: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.ReasoningContent != "" {
				reasoning.WriteString(choice.Delta.ReasoningContent)
				guard.Feed(loopguard.KindThink, choice.Delta.ReasoningContent)
				if !silent {
					observeDelta("think", choice.Delta.ReasoningContent)
				}
			}
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				guard.Feed(loopguard.KindContent, choice.Delta.Content)
				if !silent {
					observeDelta("content", choice.Delta.Content)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", nil, fmt.Errorf("读取模型流失败: %w", err)
	}
	text := strings.TrimSpace(content.String())
	if text == "" {
		return "", "", nil, fmt.Errorf("模型 API 返回空响应")
	}
	return text, reasoning.String(), usage, nil
}

// searchChatOptions 搜索摘要模型的采样参数：低温、窄核采样，减少要点漂移
func searchChatOptions(model string) chatOptions {
	temperature := 0.2
	topP := 0.9
	repeatPenalty := 1.05
	return chatOptions{
		Model:         model,
		Temperature:   &temperature,
		TopP:          &topP,
		RepeatPenalty: &repeatPenalty,
	}
}

// profileChatOptions 按摘要档位生成调用选项（主模型沿用服务端默认采样）
func profileChatOptions(profile summaryProfile) chatOptions {
	if strings.TrimSpace(profile.Model) == "" {
		return chatOptions{}
	}
	return searchChatOptions(profile.Model)
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
