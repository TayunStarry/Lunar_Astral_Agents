package Screenshoter

// loop.go 截图者的工具推理循环：轮次、回填与失败语义与月华原实现一致。

import (
	"context"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// run 执行工具推理循环并返回截图汇总。
//
// 循环编排、轮次上限与失败上报统一由 protocol/agentbase 的 RunToolLoop 承担；
// 模型失败仍沿用原语义：记录日志后带着已有截图继续收尾。
func (s *session) run() string {
	s.shots = nil
	s.primary = ""
	rejection := ""
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	messages := make([]baseMessage, 0, 1+len(s.history))
	messages = append(messages, baseMessage{Role: "system", Content: s.system})
	messages = append(messages, s.history...)
	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 agentID,
		Label:              roleLabel,
		MaxRounds:          maxIterations,
		MaxToolResultRunes: -1, // 工具结果原样回填
	})
	result, err := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Messages: messages,
		Tools:    toolDefinitions(),
		BeforeRound: func(_ int, messages []agentbase.Message) []agentbase.Message {
			return trimHistory(messages, contextLimit)
		},
		Call: func(ctx context.Context, round int, messages []agentbase.Message, tools []agentbase.ToolDefinition) (agentbase.Reply, error) {
			logger.Info(module, "["+roleLabel+"] 第 %d 轮推理", round)
			reply, streamErr := s.stream(ctx, messages, tools)
			if streamErr != nil {
				logger.Warn(module, "["+roleLabel+"] 第 %d 轮推理失败: %v", round, streamErr)
			}
			return reply, streamErr
		},
		Execute: func(_ context.Context, call agentbase.ToolCall) (string, bool, error) {
			text := s.executeTool(toolCall{Name: call.Function.Name, Arguments: call.Function.Arguments})
			if s.pending != nil {
				s.shots = append(s.shots, *s.pending)
				s.pending = nil
			}
			return text, false, nil
		},
	})
	if err == nil && len(result.Reply.ToolCalls) == 0 && strings.TrimSpace(result.Reply.Text) != "" {
		rejection = result.Reply.Text
	}
	if len(result.Messages) > 0 {
		trimmed := trimHistory(result.Messages, contextLimit)
		if len(trimmed) > 1 {
			s.history = trimmed[1:] // 去掉系统消息，保留最近上下文
		}
	}

	if len(s.shots) > 0 {
		s.primary = s.shots[len(s.shots)-1].Base64
	}
	summary := buildScreenshotSummary(s.shots)
	if summary == "" {
		if rejection != "" {
			return rejection
		}
		return "月华认为此次无需进行创作"
	}
	return summary
}

// stream 发起一轮模型请求（工具启用、循环监管与重试与原实现一致）。
func (s *session) stream(ctx context.Context, messages []baseMessage, tools []agentbase.ToolDefinition) (agentbase.Reply, error) {
	timeout := time.Duration(config.SyncFetchTimeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return agentbase.StreamWithRetry(ctx, agentbase.StreamRequest{
		Agent:    agentID,
		Role:     roleLabel,
		Messages: messages,
		Tools:    tools,
		Extra: map[string]any{
			"chat_template_kwargs": map[string]any{"reasoning_strength": "low"},
		},
		Headers:     map[string]string{"X-Lunar-Role": roleLabel},
		InsecureTLS: true,
		Timeout:     timeout,
		ReportTools: true,
	}, agentbase.RetryOptions{
		MaxAttempts: retryAttempts,
		RetryLoop:   true,
		OnRetry: func(attempt int, err error) {
			logger.Warn(module, "["+roleLabel+"] 输出陷入重复，已中断并重试（%d/%d）", attempt, retryAttempts-1)
		},
	})
}
