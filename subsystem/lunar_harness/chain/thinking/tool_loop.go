package thinking

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// runToolLoop 执行一次应答周期。
//
// 轮次编排、重复签名拦截与故障上报由对话者智能体（agents/Dialogue）负责；
// 本文件只保留对话者特有的每轮逻辑（上下文重组、轮次上限收尾）。
func (role *DialogueRole) runToolLoop(cache *ChatCache) {
	Dialogue.RunToolLoop(context.Background(), Dialogue.LoopHook{
		Role:               role.RoleTag,
		MaxRounds:          maxToolRounds + 1, // 5 轮工具执行 + 1 轮收尾，对齐原 ToolRounds 判定
		MaxRepeats:         dialogueMaxRepeats,
		MaxToolResultRunes: -1, // 原手写循环对工具结果不做截断
		Call: func(ctx context.Context, round int) (agentbase.Reply, error) {
			return role.roundReply(ctx, round, cache)
		},
		Execute: func(ctx context.Context, call agentbase.ToolCall) (string, bool, error) {
			return role.executeToolCall(call)
		},
		OnLoop: func(_ int, loopErr error) {
			role.fail(loopErr)
			// 不包装 ErrLoop，否则会被 ReportRun 的循环过滤挡掉。
			reportDialogueFailure(loopErr)
		},
		OnFailure: func(err error) {
			role.fail(err)
			reportDialogueFailure(err)
		},
	})
}

// roundReply 单轮模型调用：请求模型、处理工具调用，返回基座回复。
func (role *DialogueRole) roundReply(ctx context.Context, round int, cache *ChatCache) (agentbase.Reply, error) {
	if err := ctx.Err(); err != nil {
		return agentbase.Reply{}, err
	}
	role.prepareRoundContext()
	response, err := dialogueModelRun(role.ModelBuilder, role.RagMessages, dialogueTools())
	if err != nil {
		return agentbase.Reply{}, fmt.Errorf("模型调用失败（第 %d 轮）: %w", round, err)
	}
	if len(response.Body.Error) > 0 && strings.TrimSpace(string(response.Body.Error)) != "null" {
		return agentbase.Reply{}, fmt.Errorf("模型服务错误（第 %d 轮）: %s", round, string(response.Body.Error))
	}
	if len(response.Body.Choices) == 0 {
		return agentbase.Reply{}, fmt.Errorf("模型响应缺少 choices（第 %d 轮）", round)
	}

	analyzeModelResponse(response.Body, cache)
	message := response.Body.Choices[0].Message
	content := strings.TrimSpace(message.Content)
	calls := cache.ToolCalls
	cache.ToolCalls = nil
	assistantWritten := false

	if len(calls) > 0 {
		role.writeContext(messageFromAssistant(message))
		assistantWritten = true
		if round > maxToolRounds {
			logger.Warn(lunarCoreModule, "[对话者] 工具调用轮次达到上限（%d），停止继续调用", maxToolRounds)
			role.speakAssistantText(content)
			role.rejectToolCalls(calls)
			return role.finalizeReply(), nil
		}
		if hasExecutableTool(calls) {
			// 模型一边说话一边调用工具：这段正文是模型真实产出的对话，按正常消息推送并合成语音（不并入终稿，也不当快速应答）。
			role.speakAssistantText(content)
			return replyWithCalls(message, calls), nil
		}
		// 全部工具名都不存在：与原实现一致，回填「未找到工具包」后按无工具调用处理。
		role.appendMissingToolResults(calls)
	}

	if !assistantWritten {
		role.writeContext(messageFromAssistant(message))
	}
	notifyAgentSuccess()
	stateMu.Lock()
	finalResponse = content
	stateMu.Unlock()
	return replyWithoutCalls(message), nil
}

// prepareRoundContext 取走未读队列并重建本轮上下文（与原实现每轮一致）。
func (role *DialogueRole) prepareRoundContext() {
	unreadMu.Lock()
	pending := append([]PostMessage(nil), unreadContext...)
	unreadContext = make([]PostMessage, 0)
	unreadMu.Unlock()
	// 转换是幂等的：本回合消息在 pullContext 已转换过一次，这里再走一遍兜住
	// 「转换期间新到队」的迟到消息，保证任何资源进模型上下文前都已被句柄化。
	for _, message := range ResourcesConversionHandle(pending) {
		role.writeContext(message)
	}
	role.formatHistoricalMessages()
	role.RuntimeMessages = []PostMessage{buildRuntimeGlobalMessage()}
}

// dialogueTools 组装本轮暴露给模型的工具。
func dialogueTools() []ToolCall {
	tools := dialogueToolCalls()
	return tools
}

// finalizeReply 达到轮次上限后不带工具再要一次终稿。
func (role *DialogueRole) finalizeReply() agentbase.Reply {
	response, err := dialogueModelRun(role.ModelBuilder, role.RagMessages, nil)
	if err == nil && len(response.Body.Choices) > 0 {
		content := strings.TrimSpace(response.Body.Choices[0].Message.Content)
		if content != "" {
			stateMu.Lock()
			finalResponse = content
			stateMu.Unlock()
			role.writeContext(PostMessage{Role: RoleAssistant, Content: content})
			return agentbase.Reply{
				Message: agentbase.Message{Role: RoleAssistant, Content: content},
				Text:    content,
			}
		}
	}
	if err != nil {
		reportDialogueFailure(fmt.Errorf("收尾模型调用失败: %w", err))
	}
	fallback := "这轮步骤有点多了，我先停在这里，你可以接着往下说。"
	stateMu.Lock()
	finalResponse = fallback
	stateMu.Unlock()
	return agentbase.Reply{
		Message: agentbase.Message{Role: RoleAssistant, Content: fallback},
		Text:    fallback,
	}
}

// replyWithCalls 把助手消息转成携带可执行工具调用的基座回复。
func replyWithCalls(message AssistantMessage, calls []ToolCallItem) agentbase.Reply {
	reply := replyWithoutCalls(message)
	for _, call := range calls {
		reply.ToolCalls = append(reply.ToolCalls, agentbase.ToolCall{
			ID:       call.ID,
			Type:     call.Type,
			Function: agentbase.ToolFunction{Name: call.Function.Name, Arguments: call.Function.Arguments},
		})
	}
	reply.Message.ToolCalls = reply.ToolCalls
	return reply
}

// replyWithoutCalls 把助手消息转成不带工具调用的基座回复。
func replyWithoutCalls(message AssistantMessage) agentbase.Reply {
	return agentbase.Reply{
		Message: agentbase.Message{Role: RoleAssistant, Content: message.Content},
		Text:    strings.TrimSpace(message.Content),
	}
}
