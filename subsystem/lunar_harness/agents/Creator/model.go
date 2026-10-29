package Creator

// model.go 模型调用：OpenAI 兼容 /chat/completions + function calling。
// 同时兼容原始响应与 {success,data} 代理包装响应，并把 HTTP 状态保留到错误中，
// 供执行层判断是否属于可重试的 500 类异常。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

// errLoopOutput 推理循环：同一输出或工具调用重复过多。
var errLoopOutput = errors.New("检测到循环输出")

// httpStatusError 模型服务返回非 200 状态。
type httpStatusError struct {
	Status int
	Body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("模型 HTTP %d: %s", e.Status, previewRunes(e.Body, 240))
}

// retryable 判定循环输出或 500 类异常。
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
	return false
}

// resolveModel 解析主多模态模型端点（只解析一次）。
func resolveModel() {
	modelOnce.Do(func() {
		modelName = strings.TrimSpace(config.AgentMultimodalModel)
		if modelName == "" {
			modelName = defaultModelName
		}
		modelURL = strings.TrimRight(strings.TrimSpace(config.AgentMultimodalURL), "/")
		if modelURL == "" {
			modelURL = config.LocalV1URL()
		}
		if !strings.HasSuffix(modelURL, "/v1") {
			modelURL += "/v1"
		}
		modelKey = strings.TrimSpace(config.AgentMultimodalKey)
	})
}

// chatWithTools 发起一次带工具的模型调用。
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
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, modelURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造模型请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if modelKey != "" {
		req.Header.Set("Authorization", "Bearer "+modelKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取模型响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{Status: resp.StatusCode, Body: string(raw)}
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
	return result, nil
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
		return nil, fmt.Errorf("解析模型响应失败: %w（响应：%s）", err, previewRunes(string(raw), 200))
	}
	if wrapper.Success != nil && !*wrapper.Success {
		message := "未知错误"
		if wrapper.Error != nil && strings.TrimSpace(wrapper.Error.Message) != "" {
			message = wrapper.Error.Message
		}
		return nil, fmt.Errorf("模型返回失败: %s", message)
	}
	if len(wrapper.Data) == 0 {
		return nil, fmt.Errorf("解析模型响应失败：%s", previewRunes(string(raw), 200))
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

// runToolLoop 通用工具推理循环：模型通过 function calling 逐步调用工具，
// 同一输出/工具调用重复到上限即返回循环错误。
func (s *session) runToolLoop(ctx context.Context, role, systemPrompt, instruction string, tools []toolDefinition, handler toolHandlerFunc) (string, error) {
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: instruction},
	}
	seen := map[string]int{}
	lastSummary := ""
	limit := maxExecutorRounds
	if role == plannerLabel {
		limit = maxPlannerRounds
	}
	for round := 0; round < limit; round++ {
		response, err := chatWithTools(ctx, messages, tools)
		if err != nil {
			return "", err
		}
		messages = append(messages, chatMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})
		if len(response.ToolCalls) == 0 {
			return strings.TrimSpace(response.Content), nil
		}

		key := signatureOf(response)
		seen[key]++
		if seen[key] >= maxRepeats {
			return "", fmt.Errorf("%w（%s 第 %d 轮出现重复的工具调用）", errLoopOutput, role, round+1)
		}

		for _, call := range response.ToolCalls {
			args, parseErr := parseToolArgs(call.Function.Arguments)
			if parseErr != nil {
				outcome := toolOutcome{JSON: `{"error":"参数不是合法 JSON"}`, Summary: "参数解析失败"}
				messages = append(messages, chatMessage{Role: "tool", ToolCallID: call.ID, Content: outcome.JSON})
				lastSummary += outcome.Summary + "；"
				continue
			}
			outcome, done, execErr := handler(call.Function.Name, args)
			if execErr != nil {
				outcome = toolOutcome{
					JSON:    fmt.Sprintf(`{"error":%q}`, execErr.Error()),
					Summary: call.Function.Name + " 失败",
				}
			}
			if outcome.Reference != "" {
				s.addReference(outcome.Reference)
			}
			messages = append(messages, chatMessage{Role: "tool", ToolCallID: call.ID, Content: outcome.JSON})
			lastSummary += outcome.Summary + "；"
			if done {
				return response.Content, nil
			}
		}
	}
	return "", fmt.Errorf("%s 在 %d 轮内没有完成，已执行的步骤：%s", role, limit, strings.TrimSuffix(lastSummary, "；"))
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

// previewRunes 按字符截断。
func previewRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

// jsonText 把任意值序列化为回填给模型的 JSON 文本。
func jsonText(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(raw)
}
