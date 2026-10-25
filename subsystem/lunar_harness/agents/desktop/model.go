//go:build windows

package desktop

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/loopguard"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// autoConfig 惰性加载模型配置（模型名/URL/Key），仅首次调用时解析一次。
func autoConfig() {
	autoConfigOnce.Do(func() {
		autoModel = stringOr(config.AgentMultimodalModel, "system-multimodal")
		autoURL = normV1(stringOr(config.AgentMultimodalURL, config.LocalV1URL()))
		autoKey = stringOr(config.AgentMultimodalKey, "")
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

// autoChat 带思考监管的模型调用：输出卡循环时中断并重新生成，
// 最多重试 autoLoopMaxRetries 次；仍失败则返回错误，由调用方决定是否中止本轮。
func autoChat(ctx context.Context, role string, messages []chatMessage, tools []ltpToolDef) (chatMessage, error) {
	for attempt := 0; ; attempt++ {
		msg, err := autoChatOnce(ctx, role, messages, tools)
		if err == nil {
			return msg, nil
		}
		if ctx.Err() != nil || errors.Is(err, ErrCanceled) {
			return chatMessage{}, ErrCanceled
		}
		if !errors.Is(err, loopguard.ErrLoop) {
			return chatMessage{}, err
		}
		if attempt >= autoLoopMaxRetries {
			logger.Error("LunarHarness", "桌面操纵模型连续 %d 次陷入重复输出，跳过本次调用", attempt+1)
			return chatMessage{}, err
		}
		logger.Warn("LunarHarness", "桌面操纵检测到模型重复输出，已中断并重新生成（重试 %d/%d）", attempt+1, autoLoopMaxRetries)
	}
}

// autoChatOnce 单次模型调用（流式输出受思考监管约束，并同步到推理实况）。
func autoChatOnce(ctx context.Context, role string, messages []chatMessage, tools []ltpToolDef) (chatMessage, error) {
	if ctx.Err() != nil {
		return chatMessage{}, ErrCanceled
	}
	autoConfig()
	body, err := json.Marshal(ltpChatRequest{
		Model:         autoModel,
		Messages:      messages,
		Tools:         tools,
		Stream:        true,
		StreamOptions: &ltpStreamOptions{IncludeUsage: true},
	})
	if err != nil {
		return chatMessage{}, fmt.Errorf("序列化聊天请求失败: %w", err)
	}
	// 命中重复输出时由思考监管取消本次请求，服务端随连接断开停止生成
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, autoURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chatMessage{}, fmt.Errorf("创建聊天请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if autoKey != "" {
		req.Header.Set("Authorization", "Bearer "+autoKey)
	}

	observeBegin(role)
	defer observeEnd(role)

	start := time.Now()
	resp, err := autoHTTPClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return chatMessage{}, ErrCanceled
		}
		return chatMessage{}, fmt.Errorf("聊天请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return chatMessage{}, fmt.Errorf("模型 API 返回状态 %d: %s", resp.StatusCode, truncateStr(string(respBody), 300))
	}

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		guard := loopguard.New(cancel, loopguard.DefaultOptions())
		guard.Start()
		msg, usage, readErr := readAutoStream(resp.Body, guard, role)
		guard.Stop()
		if guard.Hit() {
			return chatMessage{}, loopguard.ErrLoop
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return chatMessage{}, ErrCanceled
			}
			return chatMessage{}, readErr
		}
		reportModelCall(body, messageText(msg.Content), usage, time.Since(start))
		return msg, nil
	}

	// 非流式兜底（少数后端忽略 stream）：同样做一次静态重复检查
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return chatMessage{}, ErrCanceled
		}
		return chatMessage{}, fmt.Errorf("读取聊天响应失败: %w", err)
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
	text := messageText(m.Content)
	if loopguard.Detected(text, loopguard.DefaultOptions()) {
		return chatMessage{}, loopguard.ErrLoop
	}
	observeDelta(role, streamKindContent, text)
	reportModelCall(body, text, chatResp.Usage, time.Since(start))
	return chatMessage{Role: m.Role, Content: m.Content, ToolCalls: m.ToolCalls}, nil
}

// autoChatText 向模型发起纯文本对话并返回助手消息的文本内容。
func autoChatText(ctx context.Context, role string, messages []chatMessage) (string, error) {
	r, err := autoChat(ctx, role, messages, nil)
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
