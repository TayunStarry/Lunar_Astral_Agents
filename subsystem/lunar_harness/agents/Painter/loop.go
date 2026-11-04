package Painter

// loop.go 绘制者的工具推理循环：与月华原实现的轮次、回填与失败语义一致。

import (
	"context"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// run 执行工具推理循环并返回作品说明。
//
// 循环编排、重复检测与失败上报统一由 protocol/agentbase 的 RunToolLoop 承担；
// 模型失败仍沿用原语义：记录日志后带着已有作品继续收尾。
func (s *session) run(system string, tools []agentbase.ToolDefinition, history *[]chatMessage) string {
	details := make([]PaintingDetail, 0)
	rejection := ""
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	messages := make([]chatMessage, 0, 1+len(s.runtime)+len(*history))
	messages = append(messages, chatMessage{Role: "system", Content: system})
	messages = append(messages, s.runtime...)
	messages = append(messages, (*history)...)

	prefixLen := 1 + len(s.runtime)
	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 agentID,
		Label:              roleLabel,
		MaxRounds:          maxIterations,
		MaxRepeats:         -1, // 内容循环由 StreamWithRetry 负责，跨轮不重复检测
		MaxToolResultRunes: -1, // 工具结果原样回填
	})
	result, err := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Messages: messages,
		Tools:    tools,
		BeforeRound: func(_ int, messages []agentbase.Message) []agentbase.Message {
			return trimPainterHistory(messages, prefixLen, contextLimit)
		},
		Call: func(ctx context.Context, round int, messages []agentbase.Message, tools []agentbase.ToolDefinition) (agentbase.Reply, error) {
			logger.Info(module, "[绘制者] 第 %d 轮推理", round)
			reply, streamErr := s.stream(ctx, messages, tools)
			if streamErr != nil {
				logger.Warn(module, "[绘制者] 第 %d 轮推理失败: %v", round, streamErr)
			}
			return reply, streamErr
		},
		Execute: func(_ context.Context, call agentbase.ToolCall) (string, bool, error) {
			text := s.executeTool(chatCall{Name: call.Function.Name, Arguments: call.Function.Arguments})
			collectPaintingDetail(chatCall{Name: call.Function.Name, Arguments: call.Function.Arguments}, &details)
			return text, false, nil
		},
	})
	if err == nil && len(result.Reply.ToolCalls) == 0 {
		rejection = result.Reply.Text
	}
	if len(result.Messages) > prefixLen {
		*history = append([]chatMessage(nil), result.Messages[prefixLen:]...)
		if len(*history) > contextLimit {
			*history = (*history)[len(*history)-contextLimit:]
		}
	}

	resultText := buildPaintingSummary(details)
	if resultText == "" {
		if rejection != "" {
			resultText = rejection
		} else {
			resultText = "月华认为此次无需进行创作"
		}
	}
	return resultText
}

// stream 发起一轮模型请求（工具启用、循环监管、重试与原实现一致）。
func (s *session) stream(ctx context.Context, messages []chatMessage, tools []agentbase.ToolDefinition) (agentbase.Reply, error) {
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
			logger.Warn(module, "[绘制者] 输出陷入重复，已中断并重试（%d/%d）", attempt, retryAttempts-1)
		},
	})
}

// trimPainterHistory 保留系统消息、运行时消息与最近 limit 条历史。
func trimPainterHistory(messages []chatMessage, prefixLen, limit int) []chatMessage {
	if len(messages) <= prefixLen+limit {
		return messages
	}
	out := make([]chatMessage, 0, prefixLen+limit)
	out = append(out, messages[:prefixLen]...)
	out = append(out, messages[len(messages)-limit:]...)
	return out
}
