package Musician

// model.go 演奏者的模型调用：统一走 protocol/agentbase 的流式调用与循环监管。

import (
	"context"
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/music"
	"LunarSubsystem/LunarHarness/prompts"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// chatMessage 演奏者请求的消息项（只含角色与文本）。
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// planBrief 阶段一：主模型产出音乐内容描述（JSON）。
func planBrief(ctx context.Context, instruction string) (music.Brief, error) {
	system, err := prompts.Read(systemPromptPath)
	if err != nil {
		return music.Brief{}, err
	}
	timeout := time.Duration(config.SyncFetchTimeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	reply, err := agentbase.StreamWithRetry(ctx, agentbase.StreamRequest{
		Agent: "musician",
		Role:  roleLabel,
		Messages: []chatMessage{
			{Role: "system", Content: string(system)},
			{Role: "user", Content: instruction},
		},
		Extra: map[string]any{
			"chat_template_kwargs": map[string]any{"reasoning_strength": "low"},
		},
		Headers:     map[string]string{"X-Lunar-Role": roleLabel},
		InsecureTLS: true,
		Timeout:     timeout,
	}, agentbase.RetryOptions{
		MaxAttempts: retryAttempts,
		RetryLoop:   true,
		OnRetry: func(attempt int, retryErr error) {
			logger.Warn(module, "["+roleLabel+"] 输出陷入重复，已中断并重试（%d/%d）", attempt, retryAttempts-1)
		},
	})
	if err != nil {
		agentbase.ReportRun("musician", err)
		return music.Brief{}, err
	}
	if reply.Text == "" {
		emptyErr := fmt.Errorf("主模型未返回内容")
		agentbase.ReportRun("musician", emptyErr)
		return music.Brief{}, emptyErr
	}
	return parseBrief(reply.Text)
}
