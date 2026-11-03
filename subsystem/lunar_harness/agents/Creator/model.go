package Creator

// model.go 创作管线的模型调用：统一走 protocol/agentbase 的流式调用、
// 推理实况、循环监管与智能修复，不再维护本地的 SSE / 观察者实现。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// retryable 判定循环输出或 500 类异常。
func retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, agentbase.ErrLoop) {
		return true
	}
	var statusErr *agentbase.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.Status >= 500
	}
	return false
}

// runToolLoop 通用工具推理循环：模型通过 function calling 逐步调用工具，
// 同一输出/工具调用重复到上限即返回循环错误。
//
// 循环编排、重复检测与失败上报统一由 protocol/agentbase 的 RunToolLoop 承担；
// 本函数只保留创作链特有的上下文预算、超限重试与步骤收尾文案。
func (s *session) runToolLoop(ctx context.Context, role, systemPrompt, instruction string, tools []toolDefinition, handler toolHandlerFunc) (string, error) {
	lastSummary := ""
	limit := maxExecutorRounds
	if role == plannerLabel {
		limit = maxPlannerRounds
	}
	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 "creative",
		Label:              role,
		MaxRounds:          limit,
		MaxRepeats:         maxRepeats,
		MaxToolResultRunes: -1, // 工具结果由 toolResultContent 截断，保留原有提示文案
	})
	result, err := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Messages: []agentbase.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: instruction},
		},
		Tools: tools,
		Stream: agentbase.StreamRequest{
			Agent:       "creative",
			Role:        role,
			Timeout:     modelTimeout,
			ReportTools: true,
		},
		// 每轮请求前收紧预算：工具结果累计过长时压缩较早内容，避免单次请求超出模型上下文
		BeforeRound: func(_ int, messages []agentbase.Message) []agentbase.Message {
			compactToolResults(messages)
			return messages
		},
		// 生活系统（自由创作）的每次模型调用同样上报「推理实况」。
		// 模型仍报上下文超限时，激进压缩历史后重试一次，避免整个步骤被跳过。
		Call: func(ctx context.Context, _ int, messages []agentbase.Message, tools []agentbase.ToolDefinition) (agentbase.Reply, error) {
			reply, streamErr := agentbase.Stream(ctx, agentbase.StreamRequest{
				Agent:       "creative",
				Role:        role,
				Messages:    messages,
				Tools:       tools,
				Timeout:     modelTimeout,
				ReportTools: true,
			})
			if streamErr != nil && isContextOverflow(streamErr) {
				shrinkMessages(messages)
				reply, streamErr = agentbase.Stream(ctx, agentbase.StreamRequest{
					Agent:       "creative",
					Role:        role,
					Messages:    messages,
					Tools:       tools,
					Timeout:     modelTimeout,
					ReportTools: true,
				})
			}
			return reply, streamErr
		},
		// 工具自身的业务错误写入回填内容并继续循环（与迁移前一致）。
		Execute: func(_ context.Context, call agentbase.ToolCall) (string, bool, error) {
			args, parseErr := agentbase.ParseToolArgs(call.Function.Arguments)
			if parseErr != nil {
				outcome := toolOutcome{JSON: `{"error":"参数不是合法 JSON"}`, Summary: "参数解析失败"}
				lastSummary += outcome.Summary + "；"
				return toolResultContent(outcome), false, nil
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
			lastSummary += outcome.Summary + "；"
			return toolResultContent(outcome), done, nil
		},
		// 步骤级失败保持原有「记录并跳过」语义，不额外触发智能修复。
		DisableReport: true,
	})
	if err != nil {
		if errors.Is(err, agentbase.ErrMaxRounds) {
			return "", fmt.Errorf("%s 在 %d 轮内没有完成，已执行的步骤：%s", role, limit, strings.TrimSuffix(lastSummary, "；"))
		}
		return "", err
	}
	return strings.TrimSpace(result.Reply.Text), nil
}

// previewRunes 按字符截断。
func previewRunes(text string, limit int) string {
	return agentbase.TruncateRunes(text, limit)
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
