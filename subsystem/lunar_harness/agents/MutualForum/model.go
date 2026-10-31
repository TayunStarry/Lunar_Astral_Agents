package MutualForum

// model.go 模型调用：OpenAI 兼容 /chat/completions + function calling。
//
// 模型名与端点为本地固定值（engine/config 的 AgentMultimodalModel/URL）；
// 同时兼容「原始 OpenAI 响应」与「代理包装响应」（success + data）两种格式。

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// SetModelObserver 注入实况观察者与模型计量（nil 表示不接入）。
func SetModelObserver(begin func(role string), delta func(role, kind, text string), end func(role string),
	meter func(prompt, completion, cached int64, generation time.Duration, estimated bool)) {
	observerMu.Lock()
	defer observerMu.Unlock()
	obsBegin, obsDelta, obsEnd, obsMeter = begin, delta, end, meter
}

func observeBegin() {
	observerMu.Lock()
	fn := obsBegin
	observerMu.Unlock()
	if fn != nil {
		fn(roleLabel)
	}
}

func observeDelta(kind, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	observerMu.Lock()
	fn := obsDelta
	observerMu.Unlock()
	if fn != nil {
		fn(roleLabel, kind, text)
	}
}

func observeEnd() {
	observerMu.Lock()
	fn := obsEnd
	observerMu.Unlock()
	if fn != nil {
		fn(roleLabel)
	}
}

func reportModelCall(requestBody []byte, completion string, usage *modelUsage, elapsed time.Duration) {
	observerMu.Lock()
	fn := obsMeter
	observerMu.Unlock()
	if fn == nil {
		return
	}
	if usage != nil && (usage.Prompt > 0 || usage.Completion > 0) {
		cached := int64(0)
		if usage.PromptTokensDetails != nil {
			cached = usage.PromptTokensDetails.CachedTokens
		}
		fn(usage.Prompt, usage.Completion, cached, elapsed, false)
		return
	}
	prompt := int64(utf8.RuneCountInString(string(requestBody))) / 3
	fn(prompt, int64(utf8.RuneCountInString(completion)), 0, elapsed, true)
}

// resolveModel 解析主多模态模型端点（只解析一次）。
func resolveModel() {
	modelOnce.Do(func() {
		modelName = config.AgentMultimodalModel
		modelURL = strings.TrimRight(config.AgentMultimodalURL, "/")
		if !strings.HasSuffix(modelURL, "/v1") {
			modelURL += "/v1"
		}
	})
}

// chatWithTools 发起一次带工具的模型调用。
//
// 优先以 SSE 流式读取：思考与正文边收边推送「推理实况」，工具调用增量按 index 合并；
// 若后端忽略 stream 直接返回整包 JSON，则退回整包解析（行为与旧实现一致）。
func chatWithTools(ctx context.Context, messages []chatMessage, tools []toolDefinition, round int) (*modelResponse, error) {
	resolveModel()

	body, err := json.Marshal(map[string]any{
		"model":          modelName,
		"messages":       messages,
		"tools":          tools,
		"tool_choice":    "auto",
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	})
	if err != nil {
		return nil, fmt.Errorf("序列化模型请求失败: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, modelTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, modelURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造模型请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		return nil, fmt.Errorf("模型 HTTP %d: %s", resp.StatusCode, previewText(string(raw), 240))
	}

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return readToolsStream(resp.Body, body, start, round)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取模型响应失败: %w", err)
	}
	completion, err := parseCompletion(raw)
	if err != nil {
		return nil, err
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("模型响应缺少 choices")
	}
	message := completion.Choices[0].Message
	result := &modelResponse{
		Content:   strings.TrimSpace(message.Content),
		Reasoning: strings.TrimSpace(message.ReasoningContent),
		ToolCalls: message.ToolCalls,
	}
	fillToolCallDefaults(result, round)
	observeDelta("think", result.Reasoning)
	observeDelta("content", result.Content)
	reportModelCall(body, result.Content+result.Reasoning, completion.Usage, time.Since(start))
	return result, nil
}

// readToolsStream 读取 SSE 流：正文/思考实时上报，工具调用增量按 index 合并。
func readToolsStream(body io.Reader, requestBody []byte, start time.Time, round int) (*modelResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var content, reasoning strings.Builder
	partials := map[int]*toolCallItem{}
	order := make([]int, 0, 4)
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
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
			return nil, fmt.Errorf("模型错误: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			if text := choice.Delta.ReasoningContent; text != "" {
				reasoning.WriteString(text)
				observeDelta("think", text)
			}
			if text := choice.Delta.Content; text != "" {
				content.WriteString(text)
				observeDelta("content", text)
			}
			for _, delta := range choice.Delta.ToolCalls {
				item, ok := partials[delta.Index]
				if !ok {
					item = &toolCallItem{}
					partials[delta.Index] = item
					order = append(order, delta.Index)
				}
				if delta.ID != "" {
					item.ID = delta.ID
				}
				if delta.Type != "" {
					item.Type = delta.Type
				}
				if delta.Function.Name != "" {
					item.Function.Name = delta.Function.Name
				}
				item.Function.Arguments += delta.Function.Arguments
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取模型流失败: %w", err)
	}
	result := &modelResponse{
		Content:   strings.TrimSpace(content.String()),
		Reasoning: strings.TrimSpace(reasoning.String()),
	}
	for _, index := range order {
		result.ToolCalls = append(result.ToolCalls, *partials[index])
	}
	if result.Content == "" && len(result.ToolCalls) == 0 {
		return nil, fmt.Errorf("模型返回空响应")
	}
	fillToolCallDefaults(result, round)
	reportModelCall(requestBody, result.Content+result.Reasoning, usage, time.Since(start))
	return result, nil
}

// fillToolCallDefaults 补齐工具调用的 ID / Type（流式与整包两条路径共用）
func fillToolCallDefaults(result *modelResponse, round int) {
	for i := range result.ToolCalls {
		if strings.TrimSpace(result.ToolCalls[i].ID) == "" {
			result.ToolCalls[i].ID = fmt.Sprintf("call_%d_%d", round, i)
		}
		if strings.TrimSpace(result.ToolCalls[i].Type) == "" {
			result.ToolCalls[i].Type = "function"
		}
	}
}

// parseCompletion 兼容原始响应与 {success,data} 包装响应。
func parseCompletion(raw []byte) (*chatCompletion, error) {
	var direct chatCompletion
	if err := json.Unmarshal(raw, &direct); err == nil && len(direct.Choices) > 0 {
		return &direct, nil
	}

	var wrapper struct {
		Success *bool           `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("解析模型响应失败: %w（响应：%s）", err, previewText(string(raw), 200))
	}
	if wrapper.Success != nil && !*wrapper.Success {
		message := "未知错误"
		if wrapper.Error != nil && strings.TrimSpace(wrapper.Error.Message) != "" {
			message = wrapper.Error.Message
		}
		return nil, fmt.Errorf("模型返回失败: %s", message)
	}
	if len(wrapper.Data) == 0 {
		return nil, fmt.Errorf("解析模型响应失败：%s", previewText(string(raw), 200))
	}
	var completion chatCompletion
	if err := json.Unmarshal(wrapper.Data, &completion); err != nil {
		return nil, fmt.Errorf("解析模型 data 失败: %w", err)
	}
	if completion.Error != nil && strings.TrimSpace(completion.Error.Message) != "" {
		return nil, fmt.Errorf("模型错误: %s", completion.Error.Message)
	}
	return &completion, nil
}

// previewText 按字符截断。
func previewText(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

// logTool 把工具调用写入日志，便于复盘。
func logTool(name, args, summary string, elapsed time.Duration) {
	logger.Info(roleLabel, "工具 %s（%s）→ %s，耗时 %s", name, previewText(args, 120), previewText(summary, 160), elapsed.Truncate(time.Millisecond))
}
