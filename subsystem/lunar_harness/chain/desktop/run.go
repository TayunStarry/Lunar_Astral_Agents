//go:build windows

// Package desktopchain 桌控链：桌面操纵的多阶段编排、上下文与运行状态。
//
// 分层职责：
//   - 本链保留：初始构建（编纂→启动）+ 执行循环（UIA→规划→单一操作者）、
//     跨轮交接记录、贴滞判定、启动/中断/状态快照；
//   - 模型调用（各阶段）与桌面原语留在 agents/desktop，本链不内嵌任何模型调用。
package desktopchain

import (
	"context"
	"errors"
	"fmt"

	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Run 运行桌面操纵，处理一条用户指令，返回结果文本。
// ctx 取消时立即中断：每个阶段开始前检查，在途模型请求随请求上下文一并取消。
func Run(ctx context.Context, instruction string) (resultText string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("桌面操纵内部异常: %v", r)
		}
		// 人工中断不算故障，其余失败交给智能修复
		if errors.Is(err, desktop.ErrCanceled) {
			return
		}
		desktop.ReportFailure(err)
	}()

	if err := ctx.Err(); err != nil {
		return "", desktop.ErrCanceled
	}

	// —— 阶段 1：提示词编纂者（无工具） ——
	plan, err := desktop.EditorPhase(ctx, instruction)
	if err != nil {
		return "", err
	}
	var handoff desktop.HandoffRecord

	// —— 阶段 2：软件启动者 ——
	if err := desktop.LauncherPhase(ctx, plan, &handoff); err != nil {
		if errors.Is(err, desktop.ErrCanceled) {
			return "", desktop.ErrCanceled
		}
		return "", fmt.Errorf("软件启动失败: %v", err)
	}

	// —— 阶段 3：执行循环 ——
	stagnant := 0
	for round := 1; round <= desktop.AutoMaxLoops; round++ {
		if err := ctx.Err(); err != nil {
			return "", desktop.ErrCanceled
		}
		// 目标窗口保活：每轮开始先确认目标应用在前台，避免对着控制台/资源管理器空转
		if err := desktop.EnsureTargetForeground(ctx, handoff.TargetApp); err != nil {
			if errors.Is(err, desktop.ErrCanceled) {
				return "", desktop.ErrCanceled
			}
			logger.Warn("LunarHarness", "桌面操纵第 %d 轮目标窗口未就绪: %v", round, err)
			return "目标窗口未就绪，任务暂停：" + err.Error(), nil
		}
		handoff.Round = round
		handoff.Foreground = desktop.DTForegroundTitle()
		handoff.ForegroundProcess = desktop.ForegroundProcess()
		handoff.VisualSummary = "" // 视觉摘要已移除：避免错误画面描述干扰规划

		// 3.1 UIA 理解者（读 UI 树/定向查）
		treeSummary, elements := desktop.UIAReaderPhase(ctx, plan)
		if err := ctx.Err(); err != nil {
			return "", desktop.ErrCanceled
		}
		// 3.2 任务规划者（无工具，决策）
		decision := desktop.PlannerPhase(ctx, plan, handoff, treeSummary, elements)
		if err := ctx.Err(); err != nil {
			return "", desktop.ErrCanceled
		}

		if decision.Decision == "complete" {
			return decision.Action, nil
		}

		// 3.3 仅启用一个操作者；记录本轮尝试与是否产生可见变化
		attempt := desktop.OperationPhase(ctx, decision, handoff)
		attempt.Round = round
		if attempt.Duplicate {
			logger.Info("LunarHarness", "桌面操纵检测到重复输入：%s，已跳过并判定完成", attempt.InputText)
			return "任务完成：窗口标题已包含输入内容「" + attempt.InputText + "」，已跳过重复输入", nil
		}
		handoff.LastAction = attempt.Result
		handoff.Attempts = desktop.AppendAttempt(handoff.Attempts, attempt)
		if attempt.Changed {
			stagnant = 0
		} else {
			stagnant++
		}
		if stagnant >= 2 {
			logger.Warn("LunarHarness", "桌面操纵连续 %d 轮操作未产生可见变化，判定停滞", stagnant)
			return "连续两轮操作未产生可见变化，任务停滞，已暂停并等待人工介入", nil
		}

		// 3.4 确定性推进交接记录（不再调用“书记者”模型，避免无依据的状态臆测）
		handoff = desktop.AdvanceHandoff(handoff, attempt)
	}
	logger.Warn("LunarHarness", "桌面操纵已达到最大执行轮次，任务暂停")
	return "已达到最大执行轮次，任务暂停", nil
}
