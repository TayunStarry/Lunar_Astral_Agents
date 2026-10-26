package lunar

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
	"strings"
	"time"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/loopguard"
)

// newModelBuilder 创建模型构建器
func newModelBuilder(prompt, roleTag string, contextLimit int) *ModelBuilder {
	builder := &ModelBuilder{
		RoleTag:           roleTag,
		ReasoningStrength: "low",
		EnableTools:       true,
		SystemPrompt:      prompt,
		ContextLimit:      contextLimit,
	}
	builder.SystemPrompt = builder.promptCompletion(prompt)
	return builder
}

// promptCompletion 替换系统提示词中的当前位置占位符
//
// 占位符有两种写法（提示词文件用 `{current-address}`，模型/历史文档里也出现过
// `{current_address}`），两种都替换，避免任何一处残留导致「当前位置」为空。
func (builder *ModelBuilder) promptCompletion(prompt string) string {
	addressText := strings.Join(resolveAddress(), " ")
	prompt = strings.ReplaceAll(prompt, "{current-address}", addressText)
	return strings.ReplaceAll(prompt, "{current_address}", addressText)
}

// writeContext 写入上下文（自动剥离 reasoning_content 与 source）
func (builder *ModelBuilder) writeContext(message PostMessage) {
	cleaned := stripRuntimeFields(message)
	limit := builder.ContextLimit
	if limit <= 0 {
		limit = 48
	}
	if len(builder.Messages) >= limit {
		discarded := builder.Messages[:len(builder.Messages)-limit+1]
		builder.Messages = append(append([]PostMessage{}, builder.Messages[len(builder.Messages)-limit+1:]...), cleaned)
		if limit == 48 {
			unreadMu.Lock()
			unreadRecords = append(unreadRecords, discarded...)
			unreadMu.Unlock()
		}
		return
	}
	builder.Messages = append(builder.Messages, cleaned)
}

// coverContext 覆写上下文
func (builder *ModelBuilder) coverContext(messages []PostMessage) {
	builder.Messages = messages
}

// appendPrompt 追加一段文本到系统提示词末尾（已包含则跳过）
func (builder *ModelBuilder) appendPrompt(content string) {
	block := "\n" + strings.Trim(content, "\n")
	if strings.Contains(builder.SystemPrompt, block) {
		return
	}
	builder.SystemPrompt = strings.TrimRight(builder.SystemPrompt, " \t\r\n") + block
}

// run 运行模型
func (builder *ModelBuilder) run(appendContext []PostMessage, tools []ToolCall) (*ModelResponse, error) {
	rawMessages := make([]PostMessage, 0, 2+len(builder.RuntimeMessages)+len(appendContext)+len(builder.Messages))
	rawMessages = append(rawMessages, PostMessage{Role: RoleSystem, Content: builder.SystemPrompt})
	rawMessages = append(rawMessages, builder.RuntimeMessages...)
	rawMessages = append(rawMessages, appendContext...)
	rawMessages = append(rawMessages, builder.Messages...)
	// 出站图片闸门：任何非视觉读取产物的图片一律替换为文本占位（见 docs/adr/0008）
	rawMessages, _ = guardRequestImages(rawMessages)

	requestBody := map[string]any{
		"model":    config.AgentMultimodalModel,
		"messages": rawMessages,
		"stream":   true,
		// 与旧链路一致：要求流式响应带回 usage，否则 token 计量只能靠估算
		"stream_options": map[string]any{"include_usage": true},
		"chat_template_kwargs": map[string]any{
			"reasoning_strength": builder.ReasoningStrength,
		},
	}
	if builder.DisableThinking {
		requestBody["chat_template_kwargs"].(map[string]any)["enable_thinking"] = false
	}
	if builder.EnableTools && len(tools) > 0 {
		requestBody["tools"] = tools
		requestBody["tool_choice"] = "auto"
	}
	bodyText := safeJSON(requestBody, "{}")
	return chatCompletion(builder.RoleTag, bodyText)
}

// stripRuntimeFields 剥离宿主路由字段（source 不属于模型会话内容）
func stripRuntimeFields(message PostMessage) PostMessage {
	message.Source = ""
	message.ToolCalls = append([]ToolCallItem(nil), message.ToolCalls...)
	return message
}

// chatCompletion 带思考监管的流式模型调用：命中重复时中断并重试，
// 仍失败则返回错误，交给对话链既有的失败处理与自愈逻辑。
func chatCompletion(roleTag, bodyText string) (*ModelResponse, error) {
	for attempt := 0; ; attempt++ {
		response, err := chatCompletionOnce(roleTag, bodyText)
		if err == nil || !errors.Is(err, loopguard.ErrLoop) || attempt >= lunarLoopRetries {
			return response, err
		}
		logger.Warn(lunarCoreModule, "[模型] %s 输出陷入重复，已中断并重试（%d/%d）", roleTag, attempt+1, lunarLoopRetries)
	}
}

// chatCompletionOnce 单次流式模型调用：解析 SSE 并推送 TUI 实况，同步返回完整响应
func chatCompletionOnce(roleTag, bodyText string) (*ModelResponse, error) {
	url := config.AgentMultimodalURL + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(bodyText))
	if err != nil {
		return nil, fmt.Errorf("创建模型请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+config.AgentMultimodalKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lunar-Role", roleTag)
	req.Header.Set("Accept", "text/event-stream")

	console.StreamBegin(roleTag)
	start := time.Now()
	deadline := time.Duration(config.SyncFetchTimeout) * time.Second
	if deadline <= 0 {
		deadline = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	req = req.WithContext(ctx)

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		ResponseHeaderTimeout: 5 * time.Minute,
	}}
	resp, err := client.Do(req)
	if err != nil {
		console.StreamEnd(roleTag)
		return nil, fmt.Errorf("发送模型请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		console.StreamEnd(roleTag)
		var parsed map[string]any
		if json.Unmarshal(raw, &parsed) == nil {
			if errValue, ok := parsed["error"]; ok {
				return nil, fmt.Errorf("模型服务错误 [%d]: %v", resp.StatusCode, errValue)
			}
		}
		return nil, fmt.Errorf("模型服务错误 [%d]: %s", resp.StatusCode, truncateRunes(string(raw), 300))
	}

	content := &strings.Builder{}
	reasoning := &strings.Builder{}
	tools := newToolAccumulator()
	var usage *TokenUsage
	finishReason := ""
	dataLines := 0
	// 保留响应头部若干字节：空响应时用它给出可诊断的原因，而不是静默返回空串
	headBuf := &limitedBuffer{limit: 64 * 1024}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	guard := loopguard.New(cancel, loopguard.DefaultOptions())
	guard.Start()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			// 只有在还没见到任何 SSE 帧时才收集：普通 JSON 响应会整段落在这里
			if dataLines == 0 && line != "" {
				headBuf.WriteString(line + "\n")
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
				guard.Feed(loopguard.KindThink, think)
				console.StreamDelta(roleTag, console.KindThink, think)
			}
			if delta.Content != "" {
				content.WriteString(delta.Content)
				guard.Feed(loopguard.KindContent, delta.Content)
				console.StreamDelta(roleTag, console.KindContent, delta.Content)
			}
			tools.ingest(delta.ToolCalls)
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finishReason = *choice.FinishReason
			}
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	guard.Stop()
	if guard.Hit() {
		console.StreamEnd(roleTag)
		return nil, loopguard.ErrLoop
	}
	if err := scanner.Err(); err != nil {
		logger.Warn(lunarCoreModule, "流式响应中断: %v", err)
	}
	if ctx.Err() == context.DeadlineExceeded {
		logger.Warn(lunarCoreModule, "流式模型调用超过 %s，已强制结束（role=%s）", deadline, roleTag)
	}
	// 非 SSE 的完整 JSON 响应（少数后端或中间层会忽略 stream）：按普通响应解析，
	// 不能当成「模型没说话」静默吞掉。
	if dataLines == 0 {
		if full, ok := parseNonStreamingResponse(headBuf.String()); ok {
			if len(full.Body.Choices) > 0 {
				message := full.Body.Choices[0].Message
				if loopguard.Detected(message.ReasoningContent+message.Content, loopguard.DefaultOptions()) {
					console.StreamEnd(roleTag)
					return nil, loopguard.ErrLoop
				}
			}
			console.StreamEnd(roleTag)
			console.RecordModelCall(0, 0, 0, time.Since(start), true)
			return full, nil
		}
	}
	if content.Len() == 0 && reasoning.Len() == 0 && len(tools.Order) == 0 {
		logger.Warn(lunarCoreModule,
			"[模型] 空响应（role=%s, finish_reason=%q, content-type=%q, 头部=%q）",
			roleTag, finishReason, resp.Header.Get("Content-Type"), truncateRunes(headBuf.String(), 300))
	}

	tools.report(roleTag)
	console.StreamEnd(roleTag)
	if usage != nil {
		console.RecordModelCall(usage.PromptTokens, usage.CompletionTokens, usage.cachedTokens(), time.Since(start), false)
	} else {
		prompt := int64(utf8.RuneCountInString(bodyText)) / 3
		completion := int64(utf8.RuneCountInString(content.String()) + utf8.RuneCountInString(reasoning.String()) + utf8.RuneCountInString(tools.argsText()))
		console.RecordModelCall(prompt, completion, 0, time.Since(start), true)
	}

	message := AssistantMessage{Role: RoleAssistant, Content: content.String(), ReasoningContent: reasoning.String()}
	if calls := tools.messageCalls(); len(calls) > 0 {
		message.ToolCalls = calls
	}
	return &ModelResponse{
		Status: resp.StatusCode,
		Body: ModelResponseBody{
			Choices: []ModelChoice{{Index: 0, Message: message, FinishReason: finishReason}},
			Usage:   usage,
		},
	}, nil
}

// chatCompletionJSON 非流式模型调用（语音识别等短请求）
func chatCompletionJSON(body map[string]any) (map[string]any, error) {
	url := config.AgentMultimodalURL + "/chat/completions"
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+config.AgentMultimodalKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: time.Duration(config.SyncFetchTimeout) * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	responseRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed map[string]any
	if err := json.Unmarshal(responseRaw, &parsed); err != nil {
		return nil, fmt.Errorf("模型响应解析失败: %w", err)
	}
	if errValue, ok := parsed["error"]; ok {
		return nil, fmt.Errorf("模型服务错误 [%d]: %v", resp.StatusCode, errValue)
	}
	return parsed, nil
}

// newToolAccumulator 创建流式工具调用聚合器
func newToolAccumulator() *toolAccumulator {
	return &toolAccumulator{Items: make(map[int]*toolAccum)}
}

// limitedBuffer 有上限的字符串缓冲（记录响应头部用于诊断）
type limitedBuffer struct {
	builder strings.Builder
	limit   int
}

// WriteString 追加文本（超过上限后静默丢弃）
func (buffer *limitedBuffer) WriteString(text string) {
	if buffer.limit > 0 && buffer.builder.Len()+len(text) > buffer.limit {
		text = text[:maxInt(0, buffer.limit-buffer.builder.Len())]
	}
	buffer.builder.WriteString(text)
}

// String 返回已收集内容
func (buffer *limitedBuffer) String() string { return buffer.builder.String() }

// parseNonStreamingResponse 解析非流式的完整 JSON 响应
func parseNonStreamingResponse(text string) (*ModelResponse, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return nil, false
	}
	var body ModelResponseBody
	if json.Unmarshal([]byte(trimmed), &body) != nil {
		return nil, false
	}
	if len(body.Choices) == 0 && len(body.Error) == 0 {
		return nil, false
	}
	return &ModelResponse{Status: http.StatusOK, Body: body}, true
}

// ingest 收纳一批 tool_calls 分片
func (accumulator *toolAccumulator) ingest(calls []sseToolCall) {
	for _, call := range calls {
		item, ok := accumulator.Items[call.Index]
		if !ok {
			item = &toolAccum{ID: call.ID}
			accumulator.Items[call.Index] = item
			accumulator.Order = append(accumulator.Order, call.Index)
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

// report 上报 TUI 工具调用
func (accumulator *toolAccumulator) report(role string) {
	for _, index := range accumulator.Order {
		item := accumulator.Items[index]
		if item.Name == "" {
			continue
		}
		console.StreamTool(role, item.Name, item.Args.String())
	}
}

// argsText 返回全部工具参数拼接文本
func (accumulator *toolAccumulator) argsText() string {
	var builder strings.Builder
	for _, index := range accumulator.Order {
		builder.WriteString(accumulator.Items[index].Args.String())
	}
	return builder.String()
}

// messageCalls 组装 OpenAI tool_calls 结构
func (accumulator *toolAccumulator) messageCalls() []ToolCallItem {
	calls := make([]ToolCallItem, 0, len(accumulator.Order))
	for _, index := range accumulator.Order {
		item := accumulator.Items[index]
		if item.Name == "" {
			continue
		}
		calls = append(calls, ToolCallItem{
			ID:   item.ID,
			Type: "function",
			Function: ToolCallFunction{
				Name:      item.Name,
				Arguments: item.Args.String(),
			},
		})
	}
	return calls
}
