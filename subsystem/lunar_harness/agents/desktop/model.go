//go:build windows

package desktop

// model.go 桌面操纵的模型调用：统一走 protocol/agentbase 的流式调用、
// 推理实况、循环监管与智能修复，不再维护本地的 SSE / 观察者实现。

import (
	"context"
	"errors"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// autoChat 带思考监管的模型调用：输出卡循环时中断并重新生成，
// 最多重试 autoLoopMaxRetries 次；仍失败则返回错误，由调用方决定是否中止本轮。
//
// 重试策略统一由 protocol/agentbase 的 StreamWithRetry 执行，本函数只保留
// 桌面操纵的日志文案、取消语义与本地消息类型映射。
func autoChat(ctx context.Context, role string, messages []chatMessage, tools []ltpToolDef) (chatMessage, error) {
	if ctx.Err() != nil {
		return chatMessage{}, ErrCanceled
	}
	reply, err := agentbase.StreamWithRetry(ctx, agentbase.StreamRequest{
		Agent:    "desktop",
		Role:     role,
		Messages: messages,
		Tools:    tools,
		Timeout:  120 * time.Second,
	}, agentbase.RetryOptions{
		MaxAttempts: autoLoopMaxRetries + 1,
		RetryLoop:   true,
		OnRetry: func(attempt int, err error) {
			logger.Warn("LunarHarness", "桌面操纵检测到模型重复输出，已中断并重新生成（重试 %d/%d）", attempt, autoLoopMaxRetries)
		},
	})
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, ErrCanceled) {
			return chatMessage{}, ErrCanceled
		}
		if errors.Is(err, agentbase.ErrLoop) {
			logger.Error("LunarHarness", "桌面操纵模型连续 %d 次陷入重复输出，跳过本次调用", autoLoopMaxRetries+1)
		}
		return chatMessage{}, err
	}
	calls := make([]ltpToolCall, 0, len(reply.ToolCalls))
	for _, call := range reply.ToolCalls {
		item := ltpToolCall{ID: call.ID, Type: call.Type}
		item.Function.Name = call.Function.Name
		item.Function.Arguments = call.Function.Arguments
		calls = append(calls, item)
	}
	message := chatMessage{Role: "assistant", Content: reply.Text, ToolCalls: calls}
	return message, nil
}

// autoChatText 向模型发起纯文本对话并返回助手消息的文本内容。
func autoChatText(ctx context.Context, role string, messages []chatMessage) (string, error) {
	r, err := autoChat(ctx, role, messages, nil)
	if err != nil {
		return "", err
	}
	return StringOfChatContent(r.Content), nil
}

// StringOfChatContent 从消息 content（字符串或片段数组）中提取纯文本。
func StringOfChatContent(content any) string {
	return agentbase.StringOf(content)
}

// truncateStr 以 rune 为单位截断字符串，超长部分以省略号结尾。
func truncateStr(s string, n int) string {
	return agentbase.TruncateRunes(s, n)
}
