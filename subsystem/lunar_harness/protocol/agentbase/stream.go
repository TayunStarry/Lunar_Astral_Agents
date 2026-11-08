package agentbase

// stream.go 统一模型调用实现：把消息送进 OpenAI 兼容端点，边收边登记
// 「推理实况」，同时执行循环输出监管、用量计量与失败上报。
//
// 所有 Go 智能体的流式调用都应经过这里；各智能体只需准备可序列化的
// messages / tools，并在拿到 Reply 后映射成自己的局部类型。

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/protocol/loopguard"
	"LunarSubsystem/LunarHarness/protocol/modellease"
)

// resolveModel 解析模型端点（仅首次调用）。
//
// 端点由装配层注入；这里只做 URL 归一化与缓存，不读取任何配置。
func resolveModel() {
	modelOnce.Do(func() {
		endpoint, ok := currentModelEndpoint()
		if !ok {
			return
		}
		modelName = endpoint.Model
		modelURL = strings.TrimRight(endpoint.URL, "/")
		if !strings.HasSuffix(modelURL, "/v1") {
			modelURL += "/v1"
		}
	})
}

// BeginLive 由调用方开启一段合并的推理实况。
func BeginLive(role string) {
	observeBegin(role)
}

// EndLive 由调用方结束一段合并的推理实况。
func EndLive(role string) {
	observeEnd(role)
}

// ResetModelCache 清空模型端点缓存（测试切换配置时使用）。
func ResetModelCache() {
	modelOnce = sync.Once{}
	modelName = ""
	modelURL = ""
}

// normalizeToolCalls 补齐工具调用的类型与 ID。
func normalizeToolCalls(calls []ToolCall) {
	for i := range calls {
		if calls[i].Type == "" {
			calls[i].Type = "function"
		}
		if calls[i].ID == "" {
			calls[i].ID = fmt.Sprintf("call_%d", toolCallSeq.Add(1))
		}
	}
}

// Stream 发起一次统一模型调用。
func Stream(ctx context.Context, req StreamRequest) (Reply, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	resolveModel()
	if modelURL == "" {
		return Reply{}, fmt.Errorf("模型端点未配置：请由装配层注入模型端点提供者")
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "智能体"
	}
	if !req.HoldLease {
		release := modellease.AcquireCall()
		defer release()
	}
	if !req.Silent && !req.ManageLive {
		observeBegin(role)
		defer observeEnd(role)
	}

	name := modelName
	if override := strings.TrimSpace(req.Model); override != "" {
		name = override
	}
	payload := map[string]any{
		"model":    name,
		"messages": req.Messages,
	}
	if req.NonStream {
		payload["stream"] = false
	} else {
		payload["stream"] = true
		payload["stream_options"] = map[string]any{"include_usage": true}
	}
	if !isNilValue(req.Tools) {
		payload["tools"] = req.Tools
		choice := strings.TrimSpace(req.ToolChoice)
		if choice == "" {
			choice = "auto"
		}
		payload["tool_choice"] = choice
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		payload["top_p"] = *req.TopP
	}
	if req.RepeatPenalty != nil {
		payload["repeat_penalty"] = *req.RepeatPenalty
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	for key, value := range req.Extra {
		switch key {
		case "model", "messages", "tools", "tool_choice", "stream", "stream_options":
			continue
		}
		payload[key] = value
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Reply{}, fmt.Errorf("序列化模型请求失败: %w", err)
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultStreamTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := modelURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Reply{}, fmt.Errorf("创建模型请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if !req.NonStream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	for key, value := range req.Headers {
		if strings.TrimSpace(key) != "" {
			httpReq.Header.Set(key, value)
		}
	}

	client := httpClient
	if req.InsecureTLS {
		client = &http.Client{Transport: &http.Transport{
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
			ResponseHeaderTimeout: 5 * time.Minute,
		}}
	}
	start := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		return Reply{}, fmt.Errorf("模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
		return Reply{}, &StatusError{Status: resp.StatusCode, Body: string(raw)}
	}

	if !req.NonStream && strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		guard := loopguard.New(cancel, req.LoopOptions)
		reply, readErr := readStreamReply(resp.Body, role, guard, req)
		guard.Stop()
		if guard.Hit() {
			return Reply{}, loopguard.ErrLoop
		}
		if readErr != nil {
			return Reply{}, readErr
		}
		reportReplyCall(body, reply, time.Since(start))
		return reply, nil
	}

	reply, err := readWholeReply(resp.Body, role, req)
	if err != nil {
		return Reply{}, err
	}
	reportReplyCall(body, reply, time.Since(start))
	return reply, nil
}

// StreamWithRetry 按统一策略执行重试。
func StreamWithRetry(ctx context.Context, req StreamRequest, opts RetryOptions) (Reply, error) {
	attempts := opts.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		reply, err := Stream(ctx, req)
		if err == nil {
			return reply, nil
		}
		lastErr = err
		if attempt >= attempts || ctx.Err() != nil || !shouldRetry(err, opts) {
			break
		}
		if opts.OnRetry != nil {
			opts.OnRetry(attempt, err)
		}
		if opts.Backoff != nil {
			wait := opts.Backoff(attempt)
			if wait > 0 {
				select {
				case <-ctx.Done():
					return Reply{}, ctx.Err()
				case <-time.After(wait):
				}
			}
		}
	}
	return Reply{}, lastErr
}

// shouldRetry 判断本次错误是否符合重试策略。
func shouldRetry(err error, opts RetryOptions) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, loopguard.ErrLoop) {
		return opts.RetryLoop
	}
	var status *StatusError
	if errors.As(err, &status) {
		return opts.RetryStatus && status.Status >= 500
	}
	return opts.RetryNetwork
}

// readStreamReply 解析 SSE 流。
func readStreamReply(body io.Reader, role string, guard *loopguard.Guard, req StreamRequest) (Reply, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var content, reasoning strings.Builder
	partials := map[int]*ToolCall{}
	order := make([]int, 0, 4)
	var usage *Usage
	finishReason := ""
	dataLines := 0
	head := &strings.Builder{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			if dataLines == 0 && line != "" && head.Len() < 64*1024 {
				head.WriteString(line)
				head.WriteString("\n")
			}
			continue
		}
		dataLines++
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
			return Reply{}, fmt.Errorf("模型错误: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			deltaContent := choice.Delta.Content
			think := choice.Delta.ReasoningContent + choice.Delta.Reasoning
			if think != "" {
				reasoning.WriteString(think)
				feedGuard(guard, loopguard.KindThink, think)
				observeDelta(role, "think", think)
				notifyDelta(req.OnDelta, "think", think)
			}
			if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
				finishReason = *choice.FinishReason
			}
			if deltaContent != "" {
				content.WriteString(deltaContent)
				feedGuard(guard, loopguard.KindContent, deltaContent)
				notifyDelta(req.OnDelta, "content", deltaContent)
				if !req.DisplayAtEnd {
					observeDelta(role, "content", displayText(req.Display, deltaContent))
				}
			}
			for _, item := range choice.Delta.ToolCalls {
				acc, ok := partials[item.Index]
				if !ok {
					acc = &ToolCall{Type: "function"}
					partials[item.Index] = acc
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
				acc.Function.Arguments += item.Function.Arguments
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Reply{}, fmt.Errorf("读取模型流失败: %w", err)
	}

	// 非 SSE 的整包 JSON（少数后端忽略 stream）：按普通响应解析
	if dataLines == 0 {
		return readWholeReply(strings.NewReader(head.String()), role, req)
	}

	sort.Ints(order)
	text := strings.TrimSpace(content.String())
	if req.DisplayAtEnd && text != "" {
		observeDelta(role, "content", displayText(req.Display, text))
	}
	toolCalls := make([]ToolCall, 0, len(order))
	for _, index := range order {
		if call := partials[index]; call != nil && strings.TrimSpace(call.Function.Name) != "" {
			toolCalls = append(toolCalls, *call)
		}
	}
	normalizeToolCalls(toolCalls)
	if req.ReportTools {
		reportToolCalls(role, toolCalls)
	}
	message := Message{Role: "assistant", Content: text}
	if len(toolCalls) > 0 {
		message.ToolCalls = toolCalls
	}
	return Reply{
		Message:      message,
		Text:         text,
		Reasoning:    strings.TrimSpace(reasoning.String()),
		ToolCalls:    toolCalls,
		Usage:        usage,
		FinishReason: finishReason,
	}, nil
}

// wholeMessage 整包响应中的助手消息（含思考字段）。
type wholeMessage struct {
	// Role 角色
	Role string `json:"role"`
	// Content 正文（字符串或片段数组）
	Content any `json:"content"`
	// ReasoningContent 思考正文
	ReasoningContent string `json:"reasoning_content"`
	// Reasoning 部分服务端使用的思考字段
	Reasoning string `json:"reasoning"`
	// ToolCalls 工具调用
	ToolCalls []ToolCall `json:"tool_calls"`
}

// wholeResponse 整包响应。
type wholeResponse struct {
	// Choices 选择项
	Choices []struct {
		// Message 助手消息
		Message wholeMessage `json:"message"`
		// FinishReason 结束原因
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	// Usage token 用量
	Usage *Usage `json:"usage"`
	// Error 错误信息
	Error *ChatError `json:"error"`
}

// readWholeReply 解析非流式整包响应（兼容 {success,data} 代理包装）。
func readWholeReply(reader io.Reader, role string, req StreamRequest) (Reply, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxResponseBodyBytes))
	if err != nil {
		return Reply{}, fmt.Errorf("读取模型响应失败: %w", err)
	}
	var direct wholeResponse
	if err := json.Unmarshal(raw, &direct); err == nil && len(direct.Choices) > 0 {
		return buildWholeReply(direct, role, req)
	}
	var wrapper struct {
		Success *bool           `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return Reply{}, fmt.Errorf("解析模型响应失败: %w（响应：%s）", err, TruncateRunes(string(raw), 512))
	}
	if wrapper.Success != nil && !*wrapper.Success {
		message := "未知错误"
		if wrapper.Error != nil && strings.TrimSpace(wrapper.Error.Message) != "" {
			message = wrapper.Error.Message
		}
		return Reply{}, fmt.Errorf("模型返回失败: %s", message)
	}
	if len(wrapper.Data) == 0 {
		return Reply{}, fmt.Errorf("解析模型响应失败：%s", TruncateRunes(string(raw), 512))
	}
	var completion wholeResponse
	if err := json.Unmarshal(wrapper.Data, &completion); err != nil {
		return Reply{}, fmt.Errorf("解析模型 data 失败: %w", err)
	}
	return buildWholeReply(completion, role, req)
}

// buildWholeReply 把整包响应整理成统一 Reply，并登记实况与循环检查。
func buildWholeReply(completion wholeResponse, role string, req StreamRequest) (Reply, error) {
	if completion.Error != nil && strings.TrimSpace(completion.Error.Message) != "" {
		return Reply{}, fmt.Errorf("模型错误: %s", completion.Error.Message)
	}
	if len(completion.Choices) == 0 {
		return Reply{}, fmt.Errorf("模型响应缺少 choices")
	}
	message := completion.Choices[0].Message
	text := strings.TrimSpace(StringOf(message.Content))
	normalizeToolCalls(message.ToolCalls)
	reasoning := strings.TrimSpace(message.ReasoningContent + message.Reasoning)
	if loopguard.Detected(reasoning+text, req.LoopOptions) {
		return Reply{}, loopguard.ErrLoop
	}
	observeDelta(role, "think", reasoning)
	observeDelta(role, "content", displayText(req.Display, text))
	notifyDelta(req.OnDelta, "think", reasoning)
	notifyDelta(req.OnDelta, "content", text)
	if req.ReportTools {
		reportToolCalls(role, message.ToolCalls)
	}
	baseMessage := Message{Role: "assistant", Content: text}
	if len(message.ToolCalls) > 0 {
		baseMessage.ToolCalls = message.ToolCalls
	}
	return Reply{
		Message:      baseMessage,
		Text:         text,
		Reasoning:    reasoning,
		ToolCalls:    message.ToolCalls,
		Usage:        completion.Usage,
		FinishReason: completion.Choices[0].FinishReason,
	}, nil
}

// reportReplyCall 把一次调用计入「运行状态」的模型计量。
func reportReplyCall(requestBody []byte, reply Reply, elapsed time.Duration) {
	reportModelCall(requestBody, reply, elapsed)
}

// reportToolCalls 把工具调用登记到推理实况（由装配层注入的 Observer.Tool 消费）。
func reportToolCalls(role string, calls []ToolCall) {
	for _, call := range calls {
		observeTool(role, call.Function.Name, call.Function.Arguments)
	}
}

// displayText 应用实况显示变换。
func displayText(display func(string) string, text string) string {
	if display == nil {
		return text
	}
	return display(text)
}

// notifyDelta 把增量原文交给调用方的解析回调（回调可为空）。
func notifyDelta(onDelta func(kind, text string), kind, text string) {
	if onDelta == nil || text == "" {
		return
	}
	onDelta(kind, text)
}

// feedGuard 向循环监管送入一段增量（guard 可能为 nil）。
func feedGuard(guard *loopguard.Guard, kind, text string) {
	if guard != nil {
		guard.Feed(kind, text)
	}
}

// isNilValue 判断接口内是否为 nil 切片 / 指针等。
func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Func:
		return rv.IsNil()
	}
	return false
}

// ErrLoop 循环输出错误（统一导出）。
var ErrLoop = loopguard.ErrLoop
