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
			args, parseErr := agentbase.ParseToolArgs(call.Function.Arguments)
			if parseErr != nil {
				outcome := Outcome{JSON: `{"error":"参数不是合法 JSON"}`, Summary: "参数解析失败"}
				lastSummary += outcome.Summary + "；"
				return toolResultContent(outcome), false, nil
			}
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
		recordLastRun(req.Role, result.Rounds, err)
		agentbase.ReportRun(AgentID, err)
		if errors.Is(err, agentbase.ErrMaxRounds) {
			return "", fmt.Errorf("%s 在 %d 轮内没有完成，已执行的步骤：%s", req.Role, limit, strings.TrimSuffix(lastSummary, "；"))
		}
		return "", err
	}
	recordLastRun(req.Role, result.Rounds, nil)
	return strings.TrimSpace(result.Reply.Text), nil
}

// recordLastRun 记录最近一次推理循环，供数据导出与修复复位。
func recordLastRun(role string, rounds int, err error) {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	lastRun = map[string]any{
		"agent":      AgentID,
		"role":       role,
		"rounds":     rounds,
		"error":      errorText(err),
		"updated_at": time.Now().Format(time.RFC3339),
	}
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
