package Descriptor

// model.go 描述者的模型调用（OpenAI 兼容 /chat/completions，流式）
//
// 为什么单独实现而不复用 goja 侧的 ModelBuilder：描述者属于句柄系统、由 Go 直接调用，
// 且需要把思考与输出实时送进 TUI「核心智能 实况」。

import (
	"bufio"
	"bytes"
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
)

// SetModelObserver 注入实况观察者（app 层启动时调用）
func SetModelObserver(begin func(role string), delta func(role, kind, text string), end func(role string)) {
	observerMu.Lock()
	defer observerMu.Unlock()
	observerBegin, observerDelta, observerEnd = begin, delta, end
}

// observeBegin 通知实况开始
func observeBegin() {
	observerMu.Lock()
	fn := observerBegin
	observerMu.Unlock()
	if fn != nil {
		fn(roleLabel)
	}
}

// observeDelta 通知实况增量（kind: think/content）
func observeDelta(kind, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	observerMu.Lock()
	fn := observerDelta
	observerMu.Unlock()
	if fn != nil {
		fn(roleLabel, kind, text)
	}
}

// observeEnd 通知实况结束
func observeEnd() {
	observerMu.Lock()
	fn := observerEnd
	observerMu.Unlock()
	if fn != nil {
		fn(roleLabel)
	}
}

// resolveModel 解析模型配置（仅首次调用）
func resolveModel() {
	modelOnce.Do(func() {
		modelName = strings.TrimSpace(*config.AgentMultimodalModel)
		if modelName == "" {
			modelName = "system-multimodal"
		}
		rawURL := strings.TrimRight(strings.TrimSpace(*config.AgentMultimodalURL), "/")
		if rawURL == "" {
			rawURL = config.LocalV1URL()
		}
		if !strings.HasSuffix(rawURL, "/v1") {
			rawURL += "/v1"
		}
		modelURL = rawURL
		modelKey = strings.TrimSpace(*config.AgentMultimodalKey)
	})
}

// chatStream 发起一次流式对话并返回助手消息（可能携带工具调用）
func chatStream(messages []chatMessage, tools []toolDef) (chatMessage, error) {
	resolveModel()
	observeBegin()
	defer observeEnd()

	payload := map[string]any{
		"model":    modelName,
		"messages": messages,
		"stream":   true,
	}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return chatMessage{}, fmt.Errorf("序列化模型请求失败: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, modelURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chatMessage{}, fmt.Errorf("创建模型请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if modelKey != "" {
		req.Header.Set("Authorization", "Bearer "+modelKey)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return chatMessage{}, fmt.Errorf("模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return chatMessage{}, fmt.Errorf("模型 HTTP %d: %s", resp.StatusCode, truncateRunes(string(raw), 240))
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return readStream(resp.Body)
	}
	return readWhole(resp.Body)
}

// chatWithRetry 发起一次对话并返回非空文本（失败自动重试一次）
func chatWithRetry(messages []chatMessage, tools []toolDef, label string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		message, err := chatStream(messages, tools)
		if err == nil {
			if text := strings.TrimSpace(stringOf(message.Content)); text != "" {
				return text, nil
			}
			lastErr = fmt.Errorf("模型返回空内容")
		} else {
			lastErr = err
		}
		if attempt == 0 {
			logger.Warn("CrystalAstral", "描述者：%s 推理失败，稍后重试: %v", label, lastErr)
			time.Sleep(mediaRetryWait)
		}
	}
	return "", lastErr
}

// readWhole 解析非流式响应
func readWhole(body io.Reader) (chatMessage, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return chatMessage{}, fmt.Errorf("读取模型响应失败: %w", err)
	}
	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return chatMessage{}, fmt.Errorf("解析模型响应失败: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return chatMessage{}, fmt.Errorf("模型错误: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return chatMessage{}, fmt.Errorf("模型返回空 choices")
	}
	message := out.Choices[0].Message
	if message.Role == "" {
		message.Role = "assistant"
	}
	normalizeToolCalls(message.ToolCalls)
	text := stringOf(message.Content)
	if text != "" {
		observeDelta("content", text)
	}
	return message, nil
}

// readStream 解析 OpenAI 兼容 SSE，边收边送实况
func readStream(body io.Reader) (chatMessage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var content strings.Builder
	var reasoning strings.Builder
	calls := map[int]*toolCall{}
	order := make([]int, 0, 2)

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
			return chatMessage{}, fmt.Errorf("模型错误: %s", chunk.Error.Message)
		}
		for _, choice := range chunk.Choices {
			if delta := choice.Delta.ReasoningContent; delta != "" {
				reasoning.WriteString(delta)
				observeDelta("think", delta)
			}
			if delta := choice.Delta.Content; delta != "" {
				content.WriteString(delta)
				observeDelta("content", delta)
			}
			for _, item := range choice.Delta.ToolCalls {
				acc, ok := calls[item.Index]
				if !ok {
					acc = &toolCall{Type: "function"}
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
		return chatMessage{}, fmt.Errorf("读取模型流失败: %w", err)
	}

	sort.Ints(order)
	result := chatMessage{Role: "assistant", Content: content.String()}
	for _, index := range order {
		if call := calls[index]; call != nil {
			result.ToolCalls = append(result.ToolCalls, *call)
		}
	}
	normalizeToolCalls(result.ToolCalls)
	return result, nil
}

// normalizeToolCalls 补齐工具调用的类型与 ID
func normalizeToolCalls(calls []toolCall) {
	for i := range calls {
		if calls[i].Type == "" {
			calls[i].Type = "function"
		}
		if calls[i].ID == "" {
			calls[i].ID = fmt.Sprintf("call_%d", atomic.AddInt64(&toolCallSeq, 1))
		}
	}
}

// stringOf 把消息 content（字符串 / 片段数组）还原为纯文本
func stringOf(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if part, ok := item.(map[string]any); ok {
				if text, ok := part["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// truncateRunes 按字符截断
func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// hardCutRunes 按字符硬切断（超长时以省略号收尾）
func hardCutRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit < 1 {
		limit = 1
	}
	return string(runes[:limit-1]) + "…"
}
