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
	"LunarSubsystem/LunarHarness/engine/console"
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
		modelName = config.AgentMultimodalModel
		modelURL = strings.TrimRight(config.AgentMultimodalURL, "/")
		if !strings.HasSuffix(modelURL, "/v1") {
			modelURL += "/v1"
		}
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
		// 每轮请求前收紧预算：工具结果累计过长时压缩较早内容，避免单次请求超出模型上下文
		compactToolResults(messages)
		// 生活系统（自由创作）的每次模型调用同样上报「推理实况」，
		// 否则用户只能看到日志，看不到调度者与各专属智能体的思考/输出。
		response, err := chatWithToolsStreamed(ctx, role, messages, tools)
		if err != nil && isContextOverflow(err) {
			// 模型仍报上下文超限：激进压缩历史后重试一次，避免整个步骤被跳过
			shrinkMessages(messages)
			response, err = chatWithToolsStreamed(ctx, role, messages, tools)
		}
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
				console.StreamToolResult(role, outcome.Summary)
				messages = append(messages, chatMessage{Role: "tool", ToolCallID: call.ID, Content: toolResultContent(outcome)})
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
			console.StreamToolResult(role, outcome.Summary)
			if outcome.Reference != "" {
				s.addReference(outcome.Reference)
			}
			messages = append(messages, chatMessage{Role: "tool", ToolCallID: call.ID, Content: toolResultContent(outcome)})
			lastSummary += outcome.Summary + "；"
			if done {
				return response.Content, nil
			}
		}
	}
	return "", fmt.Errorf("%s 在 %d 轮内没有完成，已执行的步骤：%s", role, limit, strings.TrimSuffix(lastSummary, "；"))
}

// chatWithToolsStreamed 包装一次模型调用：把思考、输出与工具调用上报「推理实况」。
// 模型响应本身仍是非流式整包返回，因此增量在拿到完整响应后一次性推送；
// 但对实况屏而言，这仍是一轮可查看、可导出的完整推理记录。
func chatWithToolsStreamed(ctx context.Context, role string, messages []chatMessage, tools []toolDefinition) (*modelResponse, error) {
	console.StreamBegin(role)
	response, err := chatWithTools(ctx, messages, tools)
	if err != nil {
		console.StreamEnd(role)
		return nil, err
	}
	console.StreamDelta(role, console.KindThink, response.Reasoning)
	console.StreamDelta(role, console.KindContent, response.Content)
	for _, call := range response.ToolCalls {
		console.StreamTool(role, call.Function.Name, call.Function.Arguments)
	}
	console.StreamEnd(role)
	return response, nil
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

// clipAtSentence 按标点截断：优先在句末标点处收尾，其次在逗号/顿号等分句标点处收尾。
// 找不到任何标点时**不截断**（宁可保留完整内容，也不把一句话切成半句）。
// 生活链的规划日志会原样喂给下一轮模型的「历史」上下文，半句会直接损害理解。
func clipAtSentence(text string, limit int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	const sentenceEnders = "。！？!?；;…\n"
	const clauseEnders = "，,、：:"
	if idx := lastPunctBefore(runes, limit, sentenceEnders); idx >= 0 {
		return string(runes[:idx+1]) + "…"
	}
	if idx := lastPunctBefore(runes, limit, clauseEnders); idx >= 0 {
		return string(runes[:idx+1]) + "…"
	}
	return text
}

// lastPunctBefore 在 [0, limit) 范围内从后向前找最后一个标点，返回其下标（未命中返回 -1）。
func lastPunctBefore(runes []rune, limit int, marks string) int {
	if limit > len(runes) {
		limit = len(runes)
	}
	for i := limit - 1; i > 0; i-- {
		if strings.ContainsRune(marks, runes[i]) {
			return i
		}
	}
	return -1
}

// jsonText 把任意值序列化为回填给模型的 JSON 文本。
func jsonText(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(raw)
}
