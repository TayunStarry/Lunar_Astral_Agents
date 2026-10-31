package Savior

// model.go 智能修复的模型调用：流式 /chat/completions（全程推送「推理实况」）。

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console"
)

// httpStatusError 模型服务返回非 200 状态。
type httpStatusError struct {
	Status int
	Body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("模型 HTTP %d: %s", e.Status, previewRunes(e.Body, 160))
}

// chatWithTools 发起一次带工具的修复推理（流式显示，结束后返回完整结果）。
func chatWithTools(ctx context.Context, messages []chatMessage, tools []toolDefinition) (*modelResponse, error) {
	body, err := json.Marshal(map[string]any{
		"model":       config.AgentMultimodalModel,
		"messages":    messages,
		"tools":       tools,
		"tool_choice": "auto",
		"stream":      true,
	})
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}
	callCtx, cancel := context.WithTimeout(ctx, modelTimeout)
	defer cancel()
	url := strings.TrimRight(config.AgentMultimodalURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造模型请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	console.StreamBegin(roleLabel)
	defer console.StreamEnd(roleLabel)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, &httpStatusError{Status: resp.StatusCode, Body: string(raw)}
	}

	content := &strings.Builder{}
	reasoning := &strings.Builder{}
	accumulator := newToolAccumulator()
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
		var chunk saviorChunk
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
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取流式响应失败: %w", err)
	}

	// 非 SSE 的完整 JSON 响应：按普通响应解析
	if dataLines == 0 {
		return parseCompletion([]byte(head.String()))
	}

	calls := accumulator.calls()
	accumulator.report(roleLabel)
	return &modelResponse{
		Content:   strings.TrimSpace(content.String()),
		Reasoning: strings.TrimSpace(reasoning.String()),
		ToolCalls: calls,
	}, nil
}

// parseCompletion 解析非流式补全响应。
func parseCompletion(raw []byte) (*modelResponse, error) {
	var completion struct {
		Choices []struct {
			Message struct {
				Content          string     `json:"content"`
				ReasoningContent string     `json:"reasoning_content"`
				ToolCalls        []toolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &completion); err != nil {
		return nil, fmt.Errorf("解析模型响应失败: %w（响应：%s）", err, previewRunes(string(raw), 160))
	}
	if completion.Error != nil && strings.TrimSpace(completion.Error.Message) != "" {
		return nil, fmt.Errorf("模型返回错误: %s", completion.Error.Message)
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
	for index := range result.ToolCalls {
		if strings.TrimSpace(result.ToolCalls[index].ID) == "" {
			result.ToolCalls[index].ID = fmt.Sprintf("call_%d", index)
		}
		if strings.TrimSpace(result.ToolCalls[index].Type) == "" {
			result.ToolCalls[index].Type = "function"
		}
	}
	return result, nil
}

// ==== 流式工具调用聚合 ====

type saviorToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type saviorChunk struct {
	Choices []struct {
		Delta struct {
			Content          string           `json:"content"`
			ReasoningContent string           `json:"reasoning_content"`
			Reasoning        string           `json:"reasoning"`
			ToolCalls        []saviorToolCall `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

type saviorAccum struct {
	ID   string
	Name string
	Args strings.Builder
}

type saviorAccumulator struct {
	items map[int]*saviorAccum
	order []int
}

func newToolAccumulator() *saviorAccumulator {
	return &saviorAccumulator{items: map[int]*saviorAccum{}}
}

func (a *saviorAccumulator) ingest(calls []saviorToolCall) {
	for _, call := range calls {
		item, ok := a.items[call.Index]
		if !ok {
			item = &saviorAccum{ID: call.ID}
			a.items[call.Index] = item
			a.order = append(a.order, call.Index)
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

func (a *saviorAccumulator) calls() []toolCall {
	out := make([]toolCall, 0, len(a.order))
	for i, index := range a.order {
		item := a.items[index]
		if item.Name == "" {
			continue
		}
		call := toolCall{ID: item.ID, Type: "function"}
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%d", i)
		}
		call.Function.Name = item.Name
		call.Function.Arguments = item.Args.String()
		out = append(out, call)
	}
	return out
}

func (a *saviorAccumulator) report(role string) {
	for _, call := range a.calls() {
		console.StreamTool(role, call.Function.Name, call.Function.Arguments)
	}
}
