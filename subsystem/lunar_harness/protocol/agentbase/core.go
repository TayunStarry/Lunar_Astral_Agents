package agentbase

// core.go 面向视觉 / 文献 / 归档等智能体的兼容入口。
//
// 所有流式、实况、循环监管、用量计量与失败上报都在 stream.go；
// 这里只把带类型消息的调用适配到统一 Stream，避免各智能体重复实现。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ChatStream 发起一次流式对话并返回助手消息（可能携带工具调用）。
// role 为实况屏显示的角色标签（如「视觉理解」「文献解析」）。
//
// Deprecated: 使用 ChatStreamContext；本入口固定 context.Background()，无法随调用方取消。
func ChatStream(role string, messages []Message, tools []ToolDefinition) (Message, error) {
	return ChatStreamWithOptions(role, messages, tools, ChatOptions{})
}

// ChatStreamContext 带调用上下文的流式对话；取消与超时随 ctx 传递。
func ChatStreamContext(ctx context.Context, role string, messages []Message, tools []ToolDefinition) (Message, error) {
	return ChatStreamWithOptionsContext(ctx, role, messages, tools, ChatOptions{})
}

// ChatStreamWithOptions 带调用选项的流式对话：支持切换模型与跳过模型租约。
//
// Deprecated: 使用 ChatStreamWithOptionsContext；本入口固定 context.Background()。
func ChatStreamWithOptions(role string, messages []Message, tools []ToolDefinition, opts ChatOptions) (Message, error) {
	return ChatStreamWithOptionsContext(context.Background(), role, messages, tools, opts)
}

// ChatStreamWithOptionsContext 带上下文与调用选项的流式对话。
func ChatStreamWithOptionsContext(ctx context.Context, role string, messages []Message, tools []ToolDefinition, opts ChatOptions) (Message, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	reply, err := Stream(ctx, StreamRequest{
		Role:        role,
		Messages:    messages,
		Tools:       tools,
		Model:       opts.Model,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		HoldLease:   opts.HoldLease,
	})
	if err != nil {
		return Message{}, err
	}
	return reply.Message, nil
}

// ChatWithRetry 发起一次对话并返回非空文本（失败自动重试一次）
func ChatWithRetry(role string, messages []Message, tools []ToolDefinition, label string) (string, error) {
	return ChatWithRetryOptions(role, messages, tools, label, ChatOptions{})
}

// ChatWithRetryOptions 带调用选项的对话重试：允许指定模型与租约状态。
func ChatWithRetryOptions(role string, messages []Message, tools []ToolDefinition, label string, opts ChatOptions) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		message, err := ChatStreamWithOptionsContext(context.Background(), role, messages, tools, opts)
		if err == nil {
			if text := strings.TrimSpace(StringOf(message.Content)); text != "" {
				return text, nil
			}
			lastErr = fmt.Errorf("模型返回空内容")
		} else {
			lastErr = err
		}
		if attempt == 0 {
			logger.Warn("CrystalAstral", "%s：%s 推理失败，稍后重试: %v", role, label, lastErr)
			time.Sleep(retryWait)
		}
	}
	// 重试仍失败：交给智能修复（循环截断与主动取消由接收方过滤）
	reportFailure(role, lastErr)
	return "", lastErr
}
