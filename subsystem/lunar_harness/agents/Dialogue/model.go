package Dialogue

// model.go 月华核心与各角色的模型调用。
//
// 统一走 protocol/agentbase 的流式调用、推理实况、循环监管与智能修复；
// 本文件只保留会话上下文管理与模型请求组装，音频转写等短请求已归 engine/resource。

import (
	"context"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// newModelBuilder 创建模型构建器
func newModelBuilder(prompt, roleTag string, contextLimit int) *ModelBuilder {
	builder := &ModelBuilder{
		RoleTag:           roleTag,
		ReasoningStrength: "low",
		EnableTools:       true,
		SystemPrompt:      prompt,
		ContextLimit:      contextLimit,
	}
	builder.SystemPrompt = builder.promptCompletion(prompt)
	return builder
}

// promptCompletion 替换系统提示词中的当前位置占位符
func (builder *ModelBuilder) promptCompletion(prompt string) string {
	addressText := strings.Join(resolveAddress(), " ")
	prompt = strings.ReplaceAll(prompt, "{current-address}", addressText)
	return strings.ReplaceAll(prompt, "{current_address}", addressText)
}

// writeContext 写入上下文（自动剥离 reasoning_content 与 source）
func (builder *ModelBuilder) writeContext(message PostMessage) {
	cleaned := stripRuntimeFields(message)
	limit := builder.ContextLimit
	if limit <= 0 {
		limit = 48
	}
	if len(builder.Messages) >= limit {
		discarded := builder.Messages[:len(builder.Messages)-limit+1]
		builder.Messages = append(append([]PostMessage{}, builder.Messages[len(builder.Messages)-limit+1:]...), cleaned)
		if limit == 48 {
			unreadMu.Lock()
			unreadRecords = append(unreadRecords, discarded...)
			unreadMu.Unlock()
		}
		return
	}
	builder.Messages = append(builder.Messages, cleaned)
}

// coverContext 覆写上下文
func (builder *ModelBuilder) coverContext(messages []PostMessage) {
	builder.Messages = messages
}

// run 运行模型
func (builder *ModelBuilder) run(appendContext []PostMessage, tools []ToolCall) (*ModelResponse, error) {
	rawMessages := make([]PostMessage, 0, 2+len(builder.RuntimeMessages)+len(appendContext)+len(builder.Messages))
	rawMessages = append(rawMessages, PostMessage{Role: RoleSystem, Content: builder.SystemPrompt})
	rawMessages = append(rawMessages, builder.RuntimeMessages...)
	rawMessages = append(rawMessages, appendContext...)
	rawMessages = append(rawMessages, builder.Messages...)
	// 出站图片闸门：任何非视觉读取产物的图片一律替换为文本占位（见 docs/adr/0008）
	rawMessages, _ = guardRequestImages(rawMessages)

	extra := map[string]any{
		"chat_template_kwargs": map[string]any{
			"reasoning_strength": builder.ReasoningStrength,
		},
	}
	if builder.DisableThinking {
		extra["chat_template_kwargs"].(map[string]any)["enable_thinking"] = false
	}
	timeout := time.Duration(config.SyncFetchTimeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	request := agentbase.StreamRequest{
		Agent:       "core",
		Role:        builder.RoleTag,
		Messages:    rawMessages,
		Extra:       extra,
		Headers:     map[string]string{"X-Lunar-Role": builder.RoleTag},
		InsecureTLS: true,
		Timeout:     timeout,
		ReportTools: true,
	}
	if builder.EnableTools && len(tools) > 0 {
		request.Tools = tools
	}
	reply, err := agentbase.StreamWithRetry(context.Background(), request, agentbase.RetryOptions{
		MaxAttempts: lunarLoopRetries + 1,
		RetryLoop:   true,
		OnRetry: func(attempt int, err error) {
			logger.Warn(lunarCoreModule, "[模型] %s 输出陷入重复，已中断并重试（%d/%d）", builder.RoleTag, attempt, lunarLoopRetries)
		},
	})
	if err != nil {
		return nil, err
	}
	return buildModelResponse(reply), nil
}

// buildModelResponse 把统一 Reply 映射为月华内部的模型响应结构。
func buildModelResponse(reply agentbase.Reply) *ModelResponse {
	message := AssistantMessage{Role: RoleAssistant, Content: reply.Text, ReasoningContent: reply.Reasoning}
	for _, call := range reply.ToolCalls {
		message.ToolCalls = append(message.ToolCalls, ToolCallItem{
			ID:   call.ID,
			Type: call.Type,
			Function: ToolCallFunction{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return &ModelResponse{
		Body: ModelResponseBody{
			Choices: []ModelChoice{{Index: 0, Message: message, FinishReason: reply.FinishReason}},
		},
	}
}

// stripRuntimeFields 剥离宿主路由字段（source 不属于模型会话内容）
func stripRuntimeFields(message PostMessage) PostMessage {
	message.Source = ""
	message.ToolCalls = append([]ToolCallItem(nil), message.ToolCalls...)
	return message
}
