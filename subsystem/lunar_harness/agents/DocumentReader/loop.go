package DocumentReader

// loop.go 文献解析的工具循环：直读流程与扫描归并共用同一套回填逻辑。
//
// 首次调用失败时退化为无工具回答（部分服务不支持 function calling），
// 其余轮次失败直接返回错误。循环编排、ctx 透传与失败上报统一由
// protocol/agentbase 的 RunToolLoop 承担。

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// runToolLoop 执行最多 maxRounds 轮「模型 → 工具 → 模型」循环
func runToolLoop(ctx context.Context, doc *docContext, messages []agentbase.Message, tools []agentbase.ToolDefinition, maxRounds int) (string, error) {
	last := ""
	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 "document_reader",
		Label:              roleLabel,
		MaxRounds:          maxRounds,
		MaxRepeats:         -1, // 迁移前无跨轮重复检测，保持原有行为
		MaxToolResultRunes: -1, // 工具结果由 executeFileTool 自行截断
	})
	_, err := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Messages: messages,
		Tools:    tools,
		Stream: agentbase.StreamRequest{
			Agent: "document_reader",
			Role:  roleLabel,
		},
		Call: func(ctx context.Context, round int, messages []agentbase.Message, tools []agentbase.ToolDefinition) (agentbase.Reply, error) {
			reply, chatErr := agentbase.Stream(ctx, agentbase.StreamRequest{
				Agent:    "document_reader",
				Role:     roleLabel,
				Messages: messages,
				Tools:    tools,
			})
			if chatErr != nil && round == 1 {
				logger.Warn("CrystalAstral", "文献解析：首轮失败，退化为无工具回答: %v", chatErr)
				fallback, fallbackErr := agentbase.ChatStreamContext(ctx, roleLabel, firstTwoMessages(messages), nil)
				if fallbackErr != nil {
					return agentbase.Reply{}, fallbackErr
				}
				if text := strings.TrimSpace(agentbase.StringOf(fallback.Content)); text != "" {
					last = text
					return agentbase.Reply{Message: fallback, Text: text}, nil
				}
				return agentbase.Reply{}, chatErr
			}
			if chatErr != nil {
				return agentbase.Reply{}, chatErr
			}
			if text := strings.TrimSpace(reply.Text); text != "" {
				last = text
			}
			return reply, nil
		},
		Execute: func(_ context.Context, call agentbase.ToolCall) (string, bool, error) {
			return executeFileTool(doc, call), false, nil
		},
	})
	if err != nil {
		if errors.Is(err, agentbase.ErrMaxRounds) {
			if last != "" {
				return last, nil
			}
			return "", fmt.Errorf("文献解析探查轮次超限")
		}
		return "", err
	}
	if last == "" {
		return "", fmt.Errorf("文献解析没有返回内容")
	}
	return last, nil
}

// firstTwoMessages 取系统消息与首条用户消息，用于无工具兜底
func firstTwoMessages(messages []agentbase.Message) []agentbase.Message {
	out := make([]agentbase.Message, 0, 2)
	for _, message := range messages {
		if len(out) >= 2 {
			break
		}
		if message.Role == "tool" || len(message.ToolCalls) > 0 {
			continue
		}
		out = append(out, message)
	}
	return out
}
