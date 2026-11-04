package creation

// model.go 创作管线的模型调用编排：工具推理循环由 agents/CreatorPlanner 承担，
// 本文件只保留链路侧的失败判定与文本工具函数。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/CreatorPlanner"
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

// runToolLoop 调用创作规划者执行一次工具推理循环，并把链侧的工具结果类型转换为智能体结果类型。
//
// 循环轮次、重复阈值、上下文预算、超限重试与收尾文案都在智能体内保持不变；
// 本函数只负责注入链侧的工具执行入口与参考资料回写。
func (s *session) runToolLoop(ctx context.Context, role, systemPrompt, instruction string, tools []toolDefinition, handler toolHandlerFunc) (string, error) {
	return CreatorPlanner.RunToolLoop(ctx, CreatorPlanner.Request{
		Role:         role,
		SystemPrompt: systemPrompt,
		Instruction:  instruction,
		Tools:        tools,
		Handler: func(name string, args map[string]any) (CreatorPlanner.Outcome, bool, error) {
			outcome, done, err := handler(name, args)
			return CreatorPlanner.Outcome{
				JSON:      outcome.JSON,
				Summary:   outcome.Summary,
				Reference: outcome.Reference,
			}, done, err
		},
		OnReference: s.addReference,
	})
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
