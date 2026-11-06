package CreatorPlanner

// agent.go 创作规划者的模型调用实例：统一走 protocol/agentbase 的流式调用、
// 推理实况、循环监管与失败上报；上下文预算与超限重试保持迁移前语义。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// RunToolLoop 通用工具推理循环：模型通过 function calling 逐步调用工具，
// 同一输出/工具调用重复到上限即返回循环错误。
func RunToolLoop(ctx context.Context, req Request) (string, error) {
	lastSummary := ""
	beginToolRun()
	limit := MaxExecutorRounds
	if req.Role == PlannerRole {
		limit = MaxPlannerRounds
	}
	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 AgentID,
		Label:              req.Role,
		MaxRounds:          limit,
		MaxRepeats:         MaxRepeats,
		MaxToolResultRunes: -1, // 工具结果由 toolResultContent 截断，保留原有提示文案
	})
	result, err := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Messages: []agentbase.Message{
			{Role: "system", Content: req.SystemPrompt},
			{Role: "user", Content: req.Instruction},
		},
		Tools: req.Tools,
		Stream: agentbase.StreamRequest{
			Agent:       AgentID,
			Role:        req.Role,
			Timeout:     ModelTimeout,
			ReportTools: true,
		},
		// 每轮请求前收紧预算：工具结果累计过长时压缩较早内容，避免单次请求超出模型上下文
		BeforeRound: func(_ int, messages []agentbase.Message) []agentbase.Message {
			compactToolResults(messages)
			return messages
		},
		// 生活系统（自由创作）的每次模型调用同样上报「推理实况」。
		// 模型仍报上下文超限时，激进压缩历史后重试一次，避免整个步骤被跳过。
		Call: func(ctx context.Context, _ int, messages []agentbase.Message, tools []agentbase.ToolDefinition) (agentbase.Reply, error) {
			reply, streamErr := agentbase.Stream(ctx, agentbase.StreamRequest{
				Agent:       AgentID,
				Role:        req.Role,
				Messages:    messages,
				Tools:       tools,
				Timeout:     ModelTimeout,
				ReportTools: true,
			})
			if streamErr != nil && isContextOverflow(streamErr) {
				shrinkMessages(messages)
				reply, streamErr = agentbase.Stream(ctx, agentbase.StreamRequest{
					Agent:       AgentID,
					Role:        req.Role,
					Messages:    messages,
					Tools:       tools,
					Timeout:     ModelTimeout,
					ReportTools: true,
				})
			}
			return reply, streamErr
		},
		// 工具自身的业务错误写入回填内容并继续循环（与迁移前一致）。
		Execute: func(_ context.Context, call agentbase.ToolCall) (string, bool, error) {
			if reason, denied := toolDeniedReason(call.Function.Name); denied {
				outcome := Outcome{
					JSON:    fmt.Sprintf(`{"error":%q}`, "工具 "+call.Function.Name+" 已被智能修复临时禁用："+reason+"；请改用其他工具，或直接结束本轮"),
					Summary: call.Function.Name + " 已被禁用",
				}
				recordToolCall(call.Function.Name, call.Function.Arguments, "已禁用")
				lastSummary += outcome.Summary + "；"
				return toolResultContent(outcome), false, nil
			}
			args, parseErr := agentbase.ParseToolArgs(call.Function.Arguments)
			if parseErr != nil {
				recordToolCall(call.Function.Name, call.Function.Arguments, "参数解析失败")
				outcome := Outcome{JSON: `{"error":"参数不是合法 JSON"}`, Summary: "参数解析失败"}
				lastSummary += outcome.Summary + "；"
				return toolResultContent(outcome), false, nil
			}
			recordToolCall(call.Function.Name, call.Function.Arguments, "")
			outcome, done, execErr := req.Handler(call.Function.Name, args)
			if execErr != nil {
				outcome = Outcome{
					JSON:    fmt.Sprintf(`{"error":%q}`, execErr.Error()),
					Summary: call.Function.Name + " 失败",
				}
			}
			if outcome.Reference != "" && req.OnReference != nil {
				req.OnReference(outcome.Reference)
			}
			lastSummary += outcome.Summary + "；"
			return toolResultContent(outcome), done, nil
		},
		// 步骤级失败保持原有「记录并跳过」语义，不额外触发智能修复。
		DisableReport: true,
	})
	if err != nil {
		name, args, count := dominantTool()
		recordLastRunWith(req.Role, result.Rounds, err, map[string]any{
			"tool_calls":    toolRunRecords(),
			"dominant_tool": name,
			"repeat_count":  count,
			"max_rounds":    limit,
		})
		reportLoopFailure(req, err, name, args, count, limit)
		if errors.Is(err, agentbase.ErrMaxRounds) {
			return "", fmt.Errorf("%s 在 %d 轮内没有完成，已执行的步骤：%s", req.Role, limit, strings.TrimSuffix(lastSummary, "；"))
		}
		return "", err
	}
	name, _, count := dominantTool()
	recordLastRunWith(req.Role, result.Rounds, nil, map[string]any{
		"tool_calls":    toolRunRecords(),
		"dominant_tool": name,
		"repeat_count":  count,
		"max_rounds":    limit,
	})
	return strings.TrimSpace(result.Reply.Text), nil
}

// reportLoopFailure 把工具循环失败结构化上报给智能修复。
func reportLoopFailure(req Request, err error, tool, args string, repeat, limit int) {
	kind := agentbase.FailureKindModel
	criterion := "模型调用成功"
	switch {
	case errors.Is(err, agentbase.ErrMaxRounds):
		kind = agentbase.FailureKindToolLoop
		criterion = fmt.Sprintf("工具循环轮次 ≤ %d", limit)
	case errors.Is(err, agentbase.ErrLoop):
		kind = agentbase.FailureKindToolLoop
		criterion = fmt.Sprintf("同一工具调用重复次数 < %d", MaxRepeats)
	case isContextOverflow(err):
		kind = agentbase.FailureKindContext
		criterion = "单次请求不超过模型上下文"
	}
	agentbase.ReportDetail(agentbase.FailureDetail{
		Agent:     AgentID,
		Kind:      kind,
		Criterion: criterion,
		Entry:     req.Role,
		Err:       err,
		Tool:      tool,
		ToolArgs:  args,
		Metrics: map[string]int64{
			"rounds":       int64(limit),
			"repeat_count": int64(repeat),
			"tool_calls":   int64(len(toolRunRecords())),
		},
	})
}

// recordLastRun 记录最近一次推理循环，供数据导出与修复复位。
func recordLastRun(role string, rounds int, err error) {
	recordLastRunWith(role, rounds, err, nil)
}

// recordLastRunWith 记录最近一次推理循环，并追加工具序列等有界字段。
func recordLastRunWith(role string, rounds int, err error, extra map[string]any) {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	record := map[string]any{
		"agent":      AgentID,
		"role":       role,
		"rounds":     rounds,
		"error":      errorText(err),
		"updated_at": time.Now().Format(time.RFC3339),
	}
	for key, value := range extra {
		record[key] = value
	}
	lastRun = record
}

// snapshotLastRun 返回最近一次推理循环的副本。
func snapshotLastRun() map[string]any {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	out := make(map[string]any, len(lastRun))
	for key, value := range lastRun {
		out[key] = value
	}
	return out
}

// resetLastRun 清空最近一次推理循环记录。
func resetLastRun() string {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	lastRun = map[string]any{}
	return "已清空创作规划者的最近推理记录"
}

// errorText 把错误转为可导出的短文本。
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return agentbase.TruncateRunes(err.Error(), 512)
}
