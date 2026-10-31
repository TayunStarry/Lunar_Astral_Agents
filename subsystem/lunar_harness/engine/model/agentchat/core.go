package agentchat

// core.go 流式 / 非流式模型调用与工具调用解析。
//
// 与具体的视觉、文献、归档能力无关：这里只负责把消息送进模型、把 SSE 增量
// 汇成完整消息、把工具调用拼装完整并回填计量。

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/loopguard"
	"LunarSubsystem/LunarHarness/engine/model/modellease"
)

// resolveModel 解析模型配置（仅首次调用）
func resolveModel() {
	modelOnce.Do(func() {
		modelName = config.AgentMultimodalModel
		modelURL = strings.TrimRight(config.AgentMultimodalURL, "/")
		if !strings.HasSuffix(modelURL, "/v1") {
			modelURL += "/v1"
		}
	})
}

// ChatStream 发起一次流式对话并返回助手消息（可能携带工具调用）。
// role 为实况屏显示的角色标签（如「视觉理解」「文献解析」）。
func ChatStream(role string, messages []Message, tools []ToolDefinition) (Message, error) {
	return ChatStreamWithOptions(role, messages, tools, ChatOptions{})
}

// ChatStreamWithOptions 带调用选项的流式对话：支持切换模型与跳过模型租约。
func ChatStreamWithOptions(role string, messages []Message, tools []ToolDefinition, opts ChatOptions) (Message, error) {
	resolveModel()
	if !opts.HoldLease {
		releaseCall := modellease.AcquireCall()
		defer releaseCall()
	}
	observeBegin(role)
	defer observeEnd(role)

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
	if opts.MaxTokens > 0 {
		payload["max_tokens"] = opts.MaxTokens
	}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Message{}, fmt.Errorf("序列化模型请求失败: %w", err)
	}
	// 思考监管命中重复时取消本次请求，由 ChatWithRetry 决定是否重试
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, modelURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Message{}, fmt.Errorf("创建模型请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return Message{}, fmt.Errorf("模型 HTTP %d: %s", resp.StatusCode, TruncateRunes(string(raw), 240))
	}

	var reply Reply
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		guard := loopguard.New(cancel, loopguard.DefaultOptions())
		guard.Start()
		reply, err = readStream(role, resp.Body, guard)
		guard.Stop()
		if guard.Hit() {
			return Message{}, loopguard.ErrLoop
		}
	} else {
		reply, err = readWhole(role, resp.Body)
		if err == nil && loopguard.Detected(reply.Reasoning+StringOf(reply.Message.Content), loopguard.DefaultOptions()) {
			return Message{}, loopguard.ErrLoop
		}
	}
	if err != nil {
		return Message{}, err
	}
	reportModelCall(body, reply, time.Since(start))
	return reply.Message, nil
}

// ChatWithRetry 发起一次对话并返回非空文本（失败自动重试一次）
func ChatWithRetry(role string, messages []Message, tools []ToolDefinition, label string) (string, error) {
	return ChatWithRetryOptions(role, messages, tools, label, ChatOptions{})
}

// ChatWithRetryOptions 带调用选项的对话重试：允许指定模型与租约状态。
func ChatWithRetryOptions(role string, messages []Message, tools []ToolDefinition, label string, opts ChatOptions) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		message, err := ChatStreamWithOptions(role, messages, tools, opts)
		if err == nil {
			if text := strings.TrimSpace(StringOf(message.Content)); text != "" {
				return text, nil
			}
			lastErr = fmt.Errorf("模型返回空内容")
		} else {
			lastErr = err
		}
		if attempt == 0 {
			logger.Warn("CrystalAstral", "%s：%s 推理失败，稍后重试: %v", role, label, lastErr)
			time.Sleep(retryWait)
		}
	}
	return "", lastErr
}

// readWhole 解析非流式响应
func readWhole(role string, body io.Reader) (Reply, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return Reply{}, fmt.Errorf("读取模型响应失败: %w", err)
	}
	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return Reply{}, fmt.Errorf("解析模型响应失败: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return Reply{}, fmt.Errorf("模型错误: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return Reply{}, fmt.Errorf("模型返回空 choices")
	}
	message := out.Choices[0].Message
	if message.Role == "" {
		message.Role = "assistant"
	}
	normalizeToolCalls(message.ToolCalls)
	text := StringOf(message.Content)
	if text != "" {
		observeDelta(role, "content", text)
	}
	return Reply{Message: message, Usage: out.Usage}, nil
}

// readStream 解析 OpenAI 兼容 SSE，边收边送实况；同时取回思考文本与用量
func readStream(role string, body io.Reader, guard *loopguard.Guard) (Reply, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var content strings.Builder
	var reasoning strings.Builder
	calls := map[int]*ToolCall{}
	order := make([]int, 0, 2)
	var usage *Usage

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return Reply{}, fmt.Errorf("模型错误: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			if delta := choice.Delta.ReasoningContent; delta != "" {
				reasoning.WriteString(delta)
				guard.Feed(loopguard.KindThink, delta)
				observeDelta(role, "think", delta)
			}
			if delta := choice.Delta.Content; delta != "" {
				content.WriteString(delta)
				guard.Feed(loopguard.KindContent, delta)
				observeDelta(role, "content", delta)
			}
			for _, item := range choice.Delta.ToolCalls {
				acc, ok := calls[item.Index]
				if !ok {
					acc = &ToolCall{Type: "function"}
					calls[item.Index] = acc
					order = append(order, item.Index)
				}
				if item.ID != "" {
					acc.ID = item.ID
				}
				if item.Type != "" {
					acc.Type = item.Type
				}
				if item.Function.Name != "" {
					acc.Function.Name = item.Function.Name
				}
				if item.Function.Arguments != "" {
					acc.Function.Arguments += item.Function.Arguments
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Reply{}, fmt.Errorf("读取模型流失败: %w", err)
	}

	sort.Ints(order)
	result := Message{Role: "assistant", Content: content.String()}
	for _, index := range order {
		if call := calls[index]; call != nil {
			result.ToolCalls = append(result.ToolCalls, *call)
		}
	}
	normalizeToolCalls(result.ToolCalls)
	return Reply{Message: result, Reasoning: reasoning.String(), Usage: usage}, nil
}

// normalizeToolCalls 补齐工具调用的类型与 ID
func normalizeToolCalls(calls []ToolCall) {
	for i := range calls {
		if calls[i].Type == "" {
			calls[i].Type = "function"
		}
		if calls[i].ID == "" {
			calls[i].ID = fmt.Sprintf("call_%d", atomic.AddInt64(&toolCallSeq, 1))
		}
	}
}
