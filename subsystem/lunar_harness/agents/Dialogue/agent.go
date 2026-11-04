package Dialogue

// agent.go 对话者：模型调用、推理实况、循环监管与故障上报。
// 会话历史、未读队列与资源转换等上下文管理留在思维链（chain/thinking）。

import (
	"context"
	"errors"
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Chat 发起一次流式模型请求：统一超时、瞬时错误重试与推理实况上报。
func Chat(ctx context.Context, req Request) (agentbase.Reply, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	attempts := retryLoopAttempts + 1
	return agentbase.StreamWithRetry(ctx, agentbase.StreamRequest{
		Agent:       AgentID,
		Role:        req.Role,
		Messages:    req.Messages,
		Tools:       req.Tools,
		Extra:       req.Extra,
		Headers:     req.Headers,
		InsecureTLS: req.InsecureTLS,
		Timeout:     timeout,
		ReportTools: req.ReportTools,
	}, agentbase.RetryOptions{
		MaxAttempts: attempts,
		RetryLoop:   true,
		OnRetry: func(attempt int, err error) {
			logger.Warn(LoggerModule, "[模型] %s 输出陷入重复，已中断并重试（%d/%d）",
				req.Role, attempt, retryLoopAttempts)
		},
	})
}

// RunToolLoop 执行一次应答周期：轮次编排、重复签名拦截与错误上报统一在此。
func RunToolLoop(ctx context.Context, hook LoopHook) {
	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 AgentID,
		Label:              hook.Role,
		MaxRounds:          hook.MaxRounds,
		MaxRepeats:         hook.MaxRepeats,
		MaxToolResultRunes: hook.MaxToolResultRunes,
	})
	result, err := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Call: func(ctx context.Context, round int, _ []agentbase.Message, _ []agentbase.ToolDefinition) (agentbase.Reply, error) {
			return hook.Call(ctx, round)
		},
		Execute: hook.Execute,
		// 统一在本函数内上报，错误里携带失败阶段与工具名
		DisableReport: true,
	})
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	if errors.Is(err, agentbase.ErrLoop) {
		loopErr := fmt.Errorf("工具循环重复（第 %d 轮）：同一响应签名连续出现，已停止本轮应答", result.Rounds)
		if hook.OnLoop != nil {
			hook.OnLoop(result.Rounds, loopErr)
		}
		return
	}
	if hook.OnFailure != nil {
		hook.OnFailure(err)
	}
}

// ReportFailure 把对话者失败上报给智能修复（故障归属 ID 为 dialogue）。
func ReportFailure(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	agentbase.ReportRun(AgentID, err)
}
