package FileEditor

// model.go 模型调用：OpenAI 兼容 /chat/completions + function calling。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console"
)

var (
	modelOnce sync.Once
	modelName string
	modelURL  string
)

// errLoopOutput 推理循环：同一输出或工具调用重复过多。
var errLoopOutput = errors.New("检测到循环输出")

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

// chatWithTools 发起一次带工具的模型调用（流式转发到「推理实况」，最终返回完整结果）。
func chatWithTools(ctx context.Context, messages []chatMessage, tools []toolDefinition) (*modelResponse, error) {
	resolveModel()
	body, err := json.Marshal(map[string]any{
		"model":       modelName,
		"messages":    messages,
		"tools":       tools,
		"tool_choice": "auto",
		"stream":      false,
	})
	if err != nil {
		return nil, fmt.Errorf("序列化模型请求失败: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, modelTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, "POST", modelURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造模型请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 文件编辑的推理过程同样进入「推理实况」屏
	console.StreamBegin(Module)
	defer console.StreamEnd(Module)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取模型响应失败: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("模型 HTTP %d: %s", resp.StatusCode, previewRunes(string(raw), 240))
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
	for i := range result.ToolCalls {
		if strings.TrimSpace(result.ToolCalls[i].ID) == "" {
			result.ToolCalls[i].ID = fmt.Sprintf("call_%d", i)
		}
		if strings.TrimSpace(result.ToolCalls[i].Type) == "" {
			result.ToolCalls[i].Type = "function"
		}
	}
	console.StreamDelta(Module, console.KindThink, result.Reasoning)
	console.StreamDelta(Module, console.KindContent, result.Content)
	return result, nil
}

// parseCompletion 解析补全响应。
func parseCompletion(raw []byte) (*chatCompletion, error) {
	var completion chatCompletion
	if err := json.Unmarshal(raw, &completion); err != nil {
		return nil, fmt.Errorf("解析模型响应失败: %w（响应：%s）", err, previewRunes(string(raw), 200))
	}
	if completion.Error != nil && strings.TrimSpace(completion.Error.Message) != "" {
		return nil, fmt.Errorf("模型错误: %s", completion.Error.Message)
	}
	return &completion, nil
}

// runToolLoop 工具推理循环：模型通过 function calling 逐步调用工具。
// 返回本轮简述与实际涉及的资源句柄（去重、保持首次出现顺序）。
func runToolLoop(ctx context.Context, systemPrompt, instruction string) (string, []string, error) {
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: instruction},
	}
	seen := map[string]int{}
	handles := make([]string, 0, 8)
	handleSeen := map[string]bool{}
	for round := 0; round < maxRounds; round++ {
		response, err := chatWithTools(ctx, messages, definitionsFor())
		if err != nil {
			return "", handles, err
		}
		messages = append(messages, chatMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})
		if len(response.ToolCalls) == 0 {
			return strings.TrimSpace(response.Content), handles, nil
		}

		key := signatureOf(response)
		seen[key]++
		if seen[key] >= maxRepeats {
			return "", handles, fmt.Errorf("%w（第 %d 轮出现重复的工具调用）", errLoopOutput, round+1)
		}

		for _, call := range response.ToolCalls {
			args, parseErr := parseToolArgs(call.Function.Arguments)
			var outcome OpResult
			if parseErr != nil {
				outcome = failResult(call.Function.Name, "", fmt.Errorf("参数不是合法 JSON"))
			} else {
				outcome = dispatchTool(ctx, call.Function.Name, args)
			}
			for _, handle := range outcome.Handles {
				if handle != "" && !handleSeen[handle] {
					handleSeen[handle] = true
					handles = append(handles, handle)
				}
			}
			console.StreamTool(Module, call.Function.Name, call.Function.Arguments)
			console.StreamToolResult(Module, outcome.Summary)
			messages = append(messages, chatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    truncateToolResult(outcome.JSON()),
			})
		}
	}
	return "", handles, fmt.Errorf("在 %d 轮内没有完成文件编辑任务", maxRounds)
}

// signatureOf 生成一轮响应的签名，用于循环检测。
func signatureOf(response *modelResponse) string {
	var b strings.Builder
	b.WriteString(response.Content)
	b.WriteByte('|')
	for _, call := range response.ToolCalls {
		b.WriteString(call.Function.Name)
		b.WriteByte(':')
		b.WriteString(call.Function.Arguments)
		b.WriteByte(';')
	}
	return b.String()
}

// parseToolArgs 解析模型给出的 JSON 参数；空串视为空对象。
func parseToolArgs(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

// truncateToolResult 单条工具结果回填模型时的长度上限。
func truncateToolResult(text string) string {
	if len([]rune(text)) <= toolMessageMaxRunes {
		return text
	}
	return previewRunes(text, toolMessageMaxRunes) + "…（结果过长，已截断；需要更多内容请用 offset 分批读取）"
}
