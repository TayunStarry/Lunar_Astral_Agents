package viewer

// submit.go 「必填工具提交」的共用基座。
//
// 提交结果只认 tool_calls 的参数：正文既不作兜底、也不进实况，
// 从根上避免「模型在正文里写 JSON、链侧再解析」的双口径。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Submit 发起一次 ToolChoice=required 的模型调用，返回唯一的提交工具调用。
//
// 没有工具调用、调用了别的工具、同一回复提交多次、参数为空，都返回明确错误；
// 实况里只保留工具调用行与思考流，正文不再推送，避免 JSON 回显。
func Submit(ctx context.Context, req SubmitRequest) (agentbase.ToolCall, error) {
	attempts := req.Attempts
	if attempts < 1 {
		attempts = 1
	}
	reply, err := agentbase.StreamWithRetry(ctx, agentbase.StreamRequest{
		Agent:       req.Agent,
		Role:        req.Role,
		Messages:    buildMessages(req.System, req.User, req.Images),
		Model:       req.Model,
		Tools:       []agentbase.ToolDefinition{req.Tool},
		ToolChoice:  "required",
		ReportTools: true,
		Display:     func(string) string { return "" },
		Timeout:     defaultTimeout,
	}, agentbase.RetryOptions{
		MaxAttempts:  attempts,
		RetryLoop:    true,
		RetryStatus:  true,
		RetryNetwork: true,
		Backoff: func(attempt int) time.Duration {
			return time.Duration(attempt) * retryBackoff
		},
		OnRetry: func(attempt int, err error) {
			logger.Warn(loggerCategory, "%s第 %d 次提交调用失败，准备重试: %v", req.Role, attempt, err)
		},
	})
	if err != nil {
		return agentbase.ToolCall{}, err
	}
	return PickSubmitCall(req.Role, reply, req.Tool.Function.Name)
}

// PickSubmitCall 从一次回复里挑出唯一的提交工具调用。
// 正文即使长得像 JSON 也不采纳；任何不符合提交约定的情况都返回错误。
func PickSubmitCall(role string, reply agentbase.Reply, toolName string) (agentbase.ToolCall, error) {
	var chosen *agentbase.ToolCall
	for index := range reply.ToolCalls {
		call := reply.ToolCalls[index]
		if call.Function.Name != toolName {
			agentbase.ReportToolResult(role, call.Function.Name, agentbase.ToolUnknown, "未执行：只接受提交工具 "+toolName)
			continue
		}
		if chosen != nil {
			agentbase.ReportToolResult(role, toolName, agentbase.ToolNotApplied, "未采纳：同一回复提交了多次")
			return agentbase.ToolCall{}, fmt.Errorf("提交工具 %s 被调用了多次，只接受唯一一次提交", toolName)
		}
		chosen = &call
	}
	if chosen == nil {
		if strings.TrimSpace(reply.Text) != "" {
			return agentbase.ToolCall{}, fmt.Errorf("模型没有调用提交工具 %s（正文内容不予采纳）", toolName)
		}
		return agentbase.ToolCall{}, fmt.Errorf("模型没有调用提交工具 %s", toolName)
	}
	if strings.TrimSpace(chosen.Function.Arguments) == "" {
		agentbase.ReportToolResult(role, toolName, agentbase.ToolNotApplied, "提交参数为空")
		return agentbase.ToolCall{}, fmt.Errorf("提交工具 %s 没有参数", toolName)
	}
	return *chosen, nil
}
