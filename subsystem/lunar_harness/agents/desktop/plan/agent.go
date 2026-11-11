//go:build windows

package plan

// agent.go 「桌面规划」的唯一模型入口：结构化任务编纂 + 下一步决策。
//
// 本成员不持执行类工具；决策必须经 submit_decision 工具调用提交，
// 因此「模型没交决策 / 交了解析不了的决策」都会成为显式失败，
// 不会再被正文 JSON 解析失败兜底成一次“成功完成”。

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// base 构建本成员的智能体基座（统一超时与轮次）。
func base() *agentbase.Agent {
	return agentbase.NewAgent(agentbase.AgentConfig{
		ID: AgentID, Label: modelRole, Timeout: modelTimeout, MaxRounds: maxRounds,
	})
}

// retryOptions 本成员统一的循环监管重试策略。
func retryOptions() agentbase.RetryOptions {
	return agentbase.RetryOptions{
		MaxAttempts: autoLoopMaxRetries + 1,
		RetryLoop:   true,
		OnRetry: func(attempt int, err error) {
			logger.Warn("LunarHarness", modelRole+"检测到模型重复输出，已中断并重新生成（重试 %d/%d）", attempt, autoLoopMaxRetries)
		},
	}
}

// chat 发起一次纯文本模型调用。
func chat(ctx context.Context, messages []desktop.ChatMessage) (string, error) {
	if ctx.Err() != nil {
		return "", desktop.ErrCanceled
	}
	reply, err := base().StreamWithRetry(ctx, agentbase.StreamRequest{
		Agent:       AgentID,
		Role:        modelRole,
		Messages:    messages,
		ReportTools: true,
	}, retryOptions())
	if err != nil {
		return "", wrapModelError(ctx, err)
	}
	return reply.Text, nil
}

// chatDecision 发起一次必填工具调用的模型请求，返回第一条决策调用的参数。
// 工具调用行由基座统一上报；这里为未被采用的调用补齐结果行，避免悬空。
func chatDecision(ctx context.Context, messages []desktop.ChatMessage) (desktop.ToolCall, error) {
	if ctx.Err() != nil {
		return desktop.ToolCall{}, desktop.ErrCanceled
	}
	reply, err := base().StreamWithRetry(ctx, agentbase.StreamRequest{
		Agent:       AgentID,
		Role:        modelRole,
		Messages:    messages,
		Tools:       []desktop.ToolDef{decisionTool},
		ToolChoice:  "required",
		ReportTools: true,
	}, retryOptions())
	if err != nil {
		return desktop.ToolCall{}, wrapModelError(ctx, err)
	}
	var chosen *desktop.ToolCall
	for _, call := range reply.ToolCalls {
		if call.Function.Name != decisionTool.Function.Name {
			agentbase.ReportToolResult(modelRole, call.Function.Name, agentbase.ToolUnknown, "未执行：规划成员只接受 submit_decision")
			continue
		}
		if chosen != nil {
			agentbase.ReportToolResult(modelRole, call.Function.Name, agentbase.ToolUnknown, "未执行：同一回复提交了多个决策，只采用第一条")
			continue
		}
		chosen = &desktop.ToolCall{ID: call.ID, Type: call.Type, Function: desktop.ToolFunc{
			Name: call.Function.Name, Arguments: call.Function.Arguments,
		}}
	}
	if chosen == nil {
		return desktop.ToolCall{}, errors.New("规划者未提交决策")
	}
	return *chosen, nil
}

// wrapModelError 统一取消语义与循环截断日志。
func wrapModelError(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, desktop.ErrCanceled) {
		return desktop.ErrCanceled
	}
	if errors.Is(err, agentbase.ErrLoop) {
		logger.Error("LunarHarness", modelRole+"模型连续 %d 次陷入重复输出，跳过本次调用", autoLoopMaxRetries+1)
	}
	return err
}

// Draft 提示词编纂：将用户原始指令解析为结构化任务。
func Draft(ctx context.Context, instruction string) (task desktop.EditorTask, err error) {
	defer func() { base().ReportRun(err) }()

	msg := desktop.ChatMessage{Role: "user", Content: "用户请求：「" + instruction + "」\n\n请输出结构化任务 JSON。"}
	text, err := chat(ctx, []desktop.ChatMessage{{Role: "system", Content: promptEditor}, msg})
	if err != nil {
		return desktop.EditorTask{}, err
	}
	var t desktop.EditorTask
	if err := json.Unmarshal(desktop.ExtractJSON(text), &t); err != nil {
		// 编纂只做信息整理，解析失败时保留原始指令继续，不阻塞任务
		return desktop.EditorTask{Task: instruction, Goal: instruction, Acceptance: []string{}}, nil
	}
	return t, nil
}

// Decide 任务规划：综合 UIA 情报与真实窗口状态，输出可立即执行的一步决策。
// 决策必须来自 submit_decision 工具调用；失败一律返回错误，绝不以「完成」兜底。
func Decide(ctx context.Context, plan desktop.EditorTask, handoff desktop.HandoffRecord, treeSummary string, elements []string) (d desktop.PlanDecision, err error) {
	defer func() { base().ReportRun(err) }()

	ctxIn := plannerContext{plan: plan, handoff: handoff, treeSummary: treeSummary, elements: elements}

	var sb strings.Builder
	sb.WriteString(desktop.BuildPlanContext(ctxIn.plan))
	sb.WriteString("\n")
	sb.WriteString("【进度】")
	sb.WriteString(desktop.StringifyHandoff(ctxIn.handoff))
	sb.WriteString("\n")
	sb.WriteString("【窗口状态】前台=")
	sb.WriteString(ctxIn.handoff.Foreground)
	sb.WriteString("（进程 ")
	sb.WriteString(ctxIn.handoff.ForegroundProcess)
	sb.WriteString("）；目标应用=")
	sb.WriteString(ctxIn.handoff.TargetApp)
	sb.WriteString("\n")
	sb.WriteString("【UIA 情报】树摘要:")
	sb.WriteString(ctxIn.treeSummary)
	sb.WriteString("\n元素:")
	sb.WriteString(strings.Join(ctxIn.elements, "；"))
	sb.WriteString("\n")
	sb.WriteString("请调用 submit_decision 提交下一步决策。")

	call, err := chatDecision(ctx, []desktop.ChatMessage{{Role: "system", Content: promptPlanner}, {Role: "user", Content: sb.String()}})
	if err != nil {
		return desktop.PlanDecision{}, err
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &d); err != nil {
		agentbase.ReportToolResult(modelRole, call.Function.Name, agentbase.ToolNotApplied, "决策参数无法解析")
		return desktop.PlanDecision{}, errors.New("规划决策参数无法解析: " + err.Error())
	}
	switch d.Decision {
	case "keyboard", "uia", "mouse", DecisionDone:
		agentbase.ReportToolResult(modelRole, call.Function.Name, agentbase.ToolApplied, "决策已接受："+d.Decision)
		return d, nil
	default:
		agentbase.ReportToolResult(modelRole, call.Function.Name, agentbase.ToolNotApplied, "决策非法："+d.Decision)
		return desktop.PlanDecision{}, errors.New("规划决策非法: " + d.Decision)
	}
}
