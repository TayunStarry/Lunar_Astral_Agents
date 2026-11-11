//go:build windows

package perceive

// agent.go 「桌面感知」的唯一模型入口：读取 UIA 树并整理与任务相关的元素。
//
// 视觉摘要已移除：每轮 10-20 秒的画面描述经常与 UIA/前台状态矛盾，反而误导规划。
// 截图只在真正需要坐标的鼠标操作时由执行成员直接取用。

import (
	"context"
	"errors"

	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	desktopos "LunarSubsystem/LunarHarness/engine/desktop"
	"LunarSubsystem/LunarHarness/internal/textutil"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// base 构建本成员的智能体基座（统一超时与轮次）。
func base() *agentbase.Agent {
	return agentbase.NewAgent(agentbase.AgentConfig{
		ID: AgentID, Label: modelRole, Timeout: modelTimeout, MaxRounds: maxRounds,
	})
}

// chat 发起一次带循环监管的模型调用，返回助手文本。
func chat(ctx context.Context, messages []desktop.ChatMessage) (string, error) {
	if ctx.Err() != nil {
		return "", desktop.ErrCanceled
	}
	reply, err := base().StreamWithRetry(ctx, agentbase.StreamRequest{
		Agent:       AgentID,
		Role:        modelRole,
		Messages:    messages,
		ReportTools: true,
	}, agentbase.RetryOptions{
		MaxAttempts: autoLoopMaxRetries + 1,
		RetryLoop:   true,
		OnRetry: func(attempt int, err error) {
			logger.Warn("LunarHarness", modelRole+"检测到模型重复输出，已中断并重新生成（重试 %d/%d）", attempt, autoLoopMaxRetries)
		},
	})
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, desktop.ErrCanceled) {
			return "", desktop.ErrCanceled
		}
		if errors.Is(err, agentbase.ErrLoop) {
			logger.Error("LunarHarness", modelRole+"模型连续 %d 次陷入重复输出，跳过本次调用", autoLoopMaxRetries+1)
		}
		return "", err
	}
	return reply.Text, nil
}

// Summarize 读取前台窗口 UI 树，并让模型整理与任务相关的元素清单。
// 读取或理解失败都返回错误，由编排层显式降级并上报，不再伪装成“读到了空树”。
func Summarize(ctx context.Context, plan desktop.EditorTask) (treeSummary string, elements []string, err error) {
	defer func() { base().ReportRun(err) }()

	result, err := desktop.CallDesktop(ctx, desktop.UIADumpTimeout, "读取 UIA 树", func() (treeResult, error) {
		tree, ok, dumpErr := desktopos.UIADump()
		return treeResult{tree: tree, ok: ok}, dumpErr
	})
	if err != nil {
		return "（UIA 树读取失败）", nil, err
	}
	if !result.ok {
		return "（UIA 树读取失败）", nil, errors.New("UIA 树读取失败")
	}
	msg := desktop.ChatMessage{Role: "user", Content: desktop.BuildPlanContext(plan) + "\n\n【UI 树】\n" + textutil.TruncateRunes(result.tree, 4096) + "\n\n请整理与任务相关的元素清单与摘要 JSON。"}
	text, err := chat(ctx, []desktop.ChatMessage{{Role: "system", Content: promptUIAReader}, msg})
	if err != nil {
		return textutil.TruncateRunes(result.tree, 512), []string{"(UIA 理解失败)"}, err
	}
	elements = []string{text}
	return textutil.TruncateRunes(result.tree, 512), elements, nil
}
