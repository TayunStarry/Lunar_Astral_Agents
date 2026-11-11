package agentbase

// tool_loop.go 通用工具循环：模型调用 → 工具分派 → 结果回填 → 下一轮。
//
// 本文件只负责编排，不依赖任何具体智能体：工具执行由调用方以 ToolExecutor 注入，
// 模型调用默认走 Agent.Stream，也可由调用方以 Call 覆盖（首轮退化、上下文超限重试等特例）。
//
// 约定：AgentConfig 中的负值用于「显式关闭」对应能力——
//   - MaxToolResultRunes < 0：关闭工具结果截断。
//
// 工具循环不按「重复调用」提前中止：只要还在轮次额度内，
// 同一工具被调用多少次都继续执行；中止只来自轮次额度、上下文取消或模型调用失败。

import (
	"context"
	"errors"
	"fmt"
)

// ErrMaxRounds 工具循环达到轮次上限（调用方可据此保留各自的收尾文案）。
var ErrMaxRounds = errors.New("工具循环达到轮次上限")

// ToolExecutor 执行一次工具调用。
//
// content 为回填给模型的工具结果文本；done=true 表示任务已达成，循环立即结束，
// 调用方可从 ToolLoopResult.Reply 取本轮助手正文作为最终答复。
// 工具自身的业务错误应由执行方编码进 content（与迁移前的行为一致），
// 返回 error 仅表示循环必须中止。
type ToolExecutor func(ctx context.Context, call ToolCall) (content string, done bool, err error)

// ToolLoopRequest 一次工具循环的输入。
type ToolLoopRequest struct {
	// Messages 初始消息数组。
	Messages []Message
	// Tools 工具定义（空或 nil 表示不启用工具）。
	Tools []ToolDefinition
	// Stream 模型调用模板；Messages / Tools / Agent / Role / Timeout 由循环按轮次补全。
	Stream StreamRequest
	// Execute 工具执行回调（必填）。
	Execute ToolExecutor
	// MaxRounds 轮次上限；0 表示使用 Agent.Config.MaxRounds。
	MaxRounds int
	// MaxToolResultRunes 单条工具结果回填上限；0 表示使用 Agent.Config.MaxToolResultRunes。
	MaxToolResultRunes int
	// DisableReport 关闭模型调用失败时的自动上报（已有上报入口或不应触发自修复时使用）。
	DisableReport bool
	// BeforeRound 每轮请求前的消息预处理（如上下文预算压缩）；返回值替换当前消息数组。
	BeforeRound func(round int, messages []Message) []Message
	// Call 覆盖默认模型调用；nil 时使用 Agent.Stream。用于首轮无工具退化、上下文超限重试等特例。
	Call func(ctx context.Context, round int, messages []Message, tools []ToolDefinition) (Reply, error)
	// Truncate 覆盖默认的工具结果截断；nil 时使用 TruncateRunes。
	Truncate func(text string) string
}

// ToolLoopResult 一次工具循环的结果。
type ToolLoopResult struct {
	// Reply 最后一轮助手回复（正常结束时为最终答复；轮次超限时为最后一轮回复）。
	Reply Reply
	// Messages 循环结束时的完整消息数组（含 assistant / tool 回填）。
	Messages []Message
	// Rounds 实际执行的模型调用轮数。
	Rounds int
	// ToolCalls 实际执行的工具调用次数。
	ToolCalls int
}

// RunToolLoop 在基座配置下执行一次标准工具循环。
func (a *Agent) RunToolLoop(ctx context.Context, req ToolLoopRequest) (ToolLoopResult, error) {
	if req.Execute == nil {
		return ToolLoopResult{}, errors.New("工具循环缺少执行回调")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	maxRounds := req.MaxRounds
	if maxRounds <= 0 {
		maxRounds = a.MaxRounds()
	}
	resultRunes := a.resultLimit(req.MaxToolResultRunes)

	messages := append([]Message(nil), req.Messages...)
	tools := req.Tools
	if len(tools) == 0 {
		tools = nil
	}
	call := req.Call
	if call == nil {
		call = func(ctx context.Context, _ int, messages []Message, tools []ToolDefinition) (Reply, error) {
			streamReq := req.Stream
			streamReq.Messages = messages
			streamReq.Tools = toolPayload(tools)
			return a.Stream(ctx, streamReq)
		}
	}

	loopResult := ToolLoopResult{}
	for round := 1; round <= maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return loopResult, err
		}
		if req.BeforeRound != nil {
			messages = req.BeforeRound(round, messages)
		}

		reply, err := call(ctx, round, messages, tools)
		if err != nil {
			if !req.DisableReport {
				a.ReportRun(err)
			}
			return loopResult, err
		}
		loopResult.Reply = reply
		loopResult.Rounds = round
		messages = append(messages, assistantMessage(reply))

		if len(reply.ToolCalls) == 0 {
			loopResult.Messages = messages
			return loopResult, nil
		}

		for _, toolCall := range reply.ToolCalls {
			if err := ctx.Err(); err != nil {
				return loopResult, err
			}
			content, done, execErr := req.Execute(ctx, toolCall)
			if execErr != nil {
				return loopResult, execErr
			}
			if req.Truncate != nil {
				content = req.Truncate(content)
			} else if resultRunes > 0 {
				content = TruncateRunes(content, resultRunes)
			}
			messages = append(messages, Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Content:    content,
			})
			loopResult.ToolCalls++
			if done {
				loopResult.Messages = messages
				return loopResult, nil
			}
		}
	}

	loopResult.Messages = messages
	return loopResult, fmt.Errorf("%w（上限 %d 轮）", ErrMaxRounds, maxRounds)
}

// toolPayload 把空工具数组转为 nil，避免向模型发送 "tools": []。
func toolPayload(tools []ToolDefinition) any {
	if len(tools) == 0 {
		return nil
	}
	return tools
}

// assistantMessage 把 Reply 归一化为可直接回填的 assistant 消息。
func assistantMessage(reply Reply) Message {
	message := reply.Message
	if message.Role == "" {
		message.Role = "assistant"
	}
	if len(message.ToolCalls) == 0 && len(reply.ToolCalls) > 0 {
		message.ToolCalls = reply.ToolCalls
	}
	if message.Content == nil {
		message.Content = reply.Text
	}
	return message
}

// resultLimit 解析工具结果截断上限：请求显式值优先，其次配置；负值表示关闭。
func (a *Agent) resultLimit(request int) int {
	if request != 0 {
		return request
	}
	return a.Config.MaxToolResultRunes
}
