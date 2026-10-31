package Responder

// model.go 模型调用：直连 llama.cpp 的流式 /chat/completions。
//
// 显示与返回刻意分开：
//   - 「推理实况」里按流式增量实时显示思考与正文（console.StreamDelta）；
//   - 流式结束后把完整内容拼接成非流式 Result 返回给调用源（外部工具链智能体）。
//
// 请求参数仍来自外部请求，只覆盖模型名与流式开关（强制 true）。

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
	"LunarSubsystem/LunarHarness/engine/console"
)

// httpStatusError 模型服务返回非 200 状态。
type httpStatusError struct {
	Status int
	Body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("模型 HTTP %d: %s", e.Status, previewRunes(e.Body, 200))
}

// retryable 判定是否值得重试：循环输出、网络错误或 5xx。
func retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errLoopOutput) {
		return true
	}
	var statusErr *httpStatusError
	if errors.As(err, &statusErr) {
		return statusErr.Status >= 500
	}
	return true
}

// buildBody 组装请求体：外部参数原样透传，模型名与流式开关强制覆盖。
func buildBody(req *Request) ([]byte, error) {
	body := map[string]json.RawMessage{}
	for key, value := range req.Params {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "model", "stream", "stream_options", "messages":
			continue
		}
		body[key] = value
	}
	modelJSON, _ := json.Marshal(config.AgentMultimodalModel)
	body["model"] = modelJSON
	body["stream"] = json.RawMessage("true")
	body["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	body["messages"] = req.Messages
	return json.Marshal(body)
}

// completeOnce 发起一次流式补全，边收边推送实况，最后拼成完整结果。
func completeOnce(ctx context.Context, req *Request) (Result, error) {
	body, err := buildBody(req)
	if err != nil {
		return Result{}, fmt.Errorf("序列化请求失败: %w", err)
	}
	callCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	url := strings.TrimRight(config.AgentMultimodalURL, "/") + llamaChatPath
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("构造模型请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	console.StreamBegin(roleLabel)
	started := time.Now()
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		console.StreamEnd(roleLabel)
		return Result{}, fmt.Errorf("模型请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		console.StreamEnd(roleLabel)
		return Result{}, &httpStatusError{Status: resp.StatusCode, Body: string(raw)}
	}

	content := &strings.Builder{}
	reasoning := &strings.Builder{}
	accumulator := newToolAccumulator()
	finishReason := ""
	usage := (*sseUsage)(nil)
	dataLines := 0
	head := &strings.Builder{}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			if dataLines == 0 && line != "" {
				head.WriteString(line)
				head.WriteString("\n")
			}
			continue
		}
		dataLines++
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk sseChunk
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			delta := choice.Delta
			if think := delta.ReasoningContent + delta.Reasoning; think != "" {
				reasoning.WriteString(think)
				console.StreamDelta(roleLabel, console.KindThink, think)
			}
			if delta.Content != "" {
				content.WriteString(delta.Content)
				console.StreamDelta(roleLabel, console.KindContent, delta.Content)
			}
			accumulator.ingest(delta.ToolCalls)
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finishReason = *choice.FinishReason
			}
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	if err := scanner.Err(); err != nil {
		console.StreamEnd(roleLabel)
		return Result{}, fmt.Errorf("读取流式响应失败: %w", err)
	}

	// 极少数后端忽略 stream：按普通 JSON 响应解析，不能当成空回答
	if dataLines == 0 {
		completion, parseErr := parseNonStreaming(head.String())
		if parseErr != nil {
			console.StreamEnd(roleLabel)
			return Result{}, parseErr
		}
		message := completion.Choices[0].Message
		console.StreamDelta(roleLabel, console.KindThink, message.ReasoningContent)
		console.StreamDelta(roleLabel, console.KindContent, message.Content)
		console.StreamEnd(roleLabel)
		return Result{
			Content:      strings.TrimSpace(message.Content),
			Reasoning:    strings.TrimSpace(message.ReasoningContent),
			ToolCalls:    normalizeToolCalls(message.ToolCalls),
			FinishReason: completion.Choices[0].FinishReason,
		}, nil
	}

	toolCalls := accumulator.calls()
	accumulator.report(roleLabel)
	console.StreamEnd(roleLabel)

	if usage != nil {
		console.RecordModelCall(usage.PromptTokens, usage.CompletionTokens, usage.cachedTokens(), time.Since(started), false)
	} else {
		console.RecordModelCall(int64(len([]rune(string(body)))/3),
			int64(len([]rune(content.String()))+len([]rune(reasoning.String()))), 0, time.Since(started), true)
	}

	rawCalls, _ := json.Marshal(toolCalls)
	if len(toolCalls) == 0 {
		rawCalls = nil
	}
	return Result{
		Content:      strings.TrimSpace(content.String()),
		Reasoning:    strings.TrimSpace(reasoning.String()),
		ToolCalls:    rawCalls,
		FinishReason: finishReason,
	}, nil
}

// parseNonStreaming 解析非流式的完整 JSON 响应。
func parseNonStreaming(text string) (*chatCompletion, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return nil, fmt.Errorf("模型响应为空")
	}
	var completion chatCompletion
	if err := json.Unmarshal([]byte(trimmed), &completion); err != nil {
		return nil, fmt.Errorf("解析模型响应失败: %w（响应：%s）", err, previewRunes(trimmed, 200))
	}
	if completion.Error != nil && strings.TrimSpace(completion.Error.Message) != "" {
		return nil, fmt.Errorf("模型返回错误: %s", completion.Error.Message)
	}
	if len(completion.Choices) == 0 {
		return nil, errors.New("模型响应缺少 choices")
	}
	return &completion, nil
}

// normalizeToolCalls 把 null / 空数组归一为 nil，避免下游误判为「有工具调用」。
func normalizeToolCalls(raw json.RawMessage) json.RawMessage {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" || text == "[]" {
		return nil
	}
	return raw
}

// completeWithRetry 首次 + 三次重试；命中循环输出同样重试。
func completeWithRetry(ctx context.Context, req *Request) (Result, error) {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		result, err := completeOnce(ctx, req)
		if err == nil {
			// 带工具调用的响应正文往往只有一句引子，不做复读判定
			if hasToolCalls(result.ToolCalls) {
				logResult(result, attempt)
				return result, nil
			}
			// 空回答（无正文、无思考、无工具调用）或长度截断：重试一次拿到完整回合
			if emptyTurn(result) {
				lastErr = fmt.Errorf("%w（第 %d 次尝试：%s）", errEmptyOutput, attempt+1, finishLabel(result))
				continue
			}
			if loopDetected(result.Content) {
				lastErr = fmt.Errorf("%w（第 %d 次尝试）", errLoopOutput, attempt+1)
				continue
			}
			logResult(result, attempt)
			return result, nil
		}
		lastErr = err
		if !retryable(err) {
			return Result{}, err
		}
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 300 * time.Millisecond):
		}
	}
	return Result{}, lastErr
}

// ==== 流式工具调用聚合 ====

// sseToolCall 单个工具调用分片。
type sseToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// sseUsage token 用量（流式尾部帧带回）。
type sseUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	PromptCacheHit   int64 `json:"prompt_cache_hit_tokens"`
	CachedTokens     int64 `json:"cached_tokens"`
}

func (u *sseUsage) cachedTokens() int64 {
	if u == nil {
		return 0
	}
	if u.PromptCacheHit > 0 {
		return u.PromptCacheHit
	}
	return u.CachedTokens
}

// sseChunk 一条 SSE 数据帧。
type sseChunk struct {
	Choices []struct {
		Delta struct {
			Content          string        `json:"content"`
			ReasoningContent string        `json:"reasoning_content"`
			Reasoning        string        `json:"reasoning"`
			ToolCalls        []sseToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *sseUsage `json:"usage"`
}

// toolAccum 聚合后的单个工具调用。
type toolAccum struct {
	ID   string
	Name string
	Args strings.Builder
}

// toolAccumulator 工具调用分片聚合器。
type toolAccumulator struct {
	Items map[int]*toolAccum
	Order []int
}

// newToolAccumulator 创建聚合器。
func newToolAccumulator() *toolAccumulator {
	return &toolAccumulator{Items: map[int]*toolAccum{}}
}

// ingest 收纳一批 tool_calls 分片。
func (a *toolAccumulator) ingest(calls []sseToolCall) {
	for _, call := range calls {
		item, ok := a.Items[call.Index]
		if !ok {
			item = &toolAccum{ID: call.ID}
			a.Items[call.Index] = item
			a.Order = append(a.Order, call.Index)
		}
		if call.ID != "" {
			item.ID = call.ID
		}
		if call.Function.Name != "" {
			item.Name = call.Function.Name
		}
		if call.Function.Arguments != "" {
			item.Args.WriteString(call.Function.Arguments)
		}
	}
}

// calls 组装 OpenAI tool_calls 结构。
func (a *toolAccumulator) calls() []toolCallOut {
	out := make([]toolCallOut, 0, len(a.Order))
	for i, index := range a.Order {
		item := a.Items[index]
		if item.Name == "" {
			continue
		}
		id := item.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		call := toolCallOut{ID: id, Type: "function"}
		call.Function.Name = item.Name
		call.Function.Arguments = item.Args.String()
		out = append(out, call)
	}
	return out
}

// report 在实况屏登记工具调用（名称 + 参数）。
func (a *toolAccumulator) report(role string) {
	for _, call := range a.calls() {
		console.StreamTool(role, call.Function.Name, call.Function.Arguments)
	}
}

// toolCallOut 非流式返回用的工具调用结构。
type toolCallOut struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// httpClient 共享 HTTP 客户端（无全局超时，由每次请求的 context 控制）。
var httpClient = &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 2}}

// nowMillis 当前毫秒时间戳。
func nowMillis() int64 {
	return time.Now().UnixMilli()
}
