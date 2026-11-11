//go:build windows

// Package desktopchain 桌控链：桌面操控的多阶段编排、上下文与运行状态。
//
// 分层职责：
//   - 本链保留：初始构建（编纂→启动）+ 执行循环（感知→规划→执行）、
//     跨轮交接记录、贴滞判定、启动/中断/状态快照；
//   - 模型调用（各成员）与桌面原语留在 agents/desktop 及其成员包，本链不内嵌任何模型调用。
package desktopchain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/desktop"
	desktopexecute "LunarSubsystem/LunarHarness/agents/desktop/execute"
	desktopperceive "LunarSubsystem/LunarHarness/agents/desktop/perceive"
	desktopplan "LunarSubsystem/LunarHarness/agents/desktop/plan"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	desktopos "LunarSubsystem/LunarHarness/engine/desktop"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// reportFailure 把链级失败上报给智能修复（人工中断不算故障）。
func reportFailure(err error) {
	agentbase.ReportRun(chainScopeID, err)
}

// Run 运行桌面操纵，处理一条用户指令，返回结果文本。
// ctx 取消时立即中断：每个阶段开始前检查，在途模型请求随请求上下文一并取消。
func Run(ctx context.Context, instruction string) (resultText string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf(chainScopeLabel+"内部异常: %v", r)
		}
		// 人工中断不算故障，其余失败交给智能修复
		if errors.Is(err, desktop.ErrCanceled) {
			return
		}
		reportFailure(err)
	}()

	if err := ctx.Err(); err != nil {
		return "", desktop.ErrCanceled
	}

	// —— 阶段 1：桌面规划（编纂） ——
	plan, err := desktopplan.Draft(ctx, instruction)
	if err != nil {
		return "", err
	}
	var handoff desktop.HandoffRecord

	// —— 阶段 2：桌面执行（启动目标应用） ——
	if err := desktopexecute.Launch(ctx, plan, &handoff); err != nil {
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
			logger.Warn("LunarHarness", chainScopeLabel+"第 %d 轮目标窗口未就绪: %v", round, err)
			return "目标窗口未就绪，任务暂停：" + err.Error(), nil
		}
		handoff.Round = round
		handoff.Foreground = desktopos.ForegroundTitle()
		handoff.ForegroundProcess = desktopos.ForegroundProcess()
		handoff.VisualSummary = "" // 视觉摘要已移除：避免错误画面描述干扰规划

		// 3.1 桌面感知（读 UI 树/定向查）：读取或理解失败都上抛，不再伪装成「读到了空树」
		treeSummary, elements, perceiveErr := desktopperceive.Summarize(ctx, plan)
		if err := ctx.Err(); err != nil {
			return "", desktop.ErrCanceled
		}
		if perceiveErr != nil {
			if errors.Is(perceiveErr, desktop.ErrCanceled) {
				return "", desktop.ErrCanceled
			}
			logger.Warn("LunarHarness", chainScopeLabel+"第 %d 轮感知失败: %v", round, perceiveErr)
			return "", fmt.Errorf("桌面感知失败: %v", perceiveErr)
		}
		// 3.2 桌面规划（决策）：必须由 submit_decision 工具调用提交，失败直接上抛
		decision, planErr := desktopplan.Decide(ctx, plan, handoff, treeSummary, elements)
		if err := ctx.Err(); err != nil {
			return "", desktop.ErrCanceled
		}
		if planErr != nil {
			if errors.Is(planErr, desktop.ErrCanceled) {
				return "", desktop.ErrCanceled
			}
			return "", fmt.Errorf("桌面规划失败: %v", planErr)
		}

		// 规划侧没有下一步：这是显式状态，不等于任务完成。
		// 这里做一次确定性收尾核对，把三态回填到最后一个动作上。
		if decision.Decision == desktopplan.DecisionDone {
			reason := strings.TrimSpace(decision.Action)
			if reason == "" {
				reason = "规划者未给出原因"
			}
			logger.Info("LunarHarness", chainScopeLabel+"规划侧没有下一步：%s", reason)
			state, summary := verifyOutcome(ctx, plan, handoff)
			if err := ctx.Err(); err != nil {
				return "", desktop.ErrCanceled
			}
			if n := len(handoff.Attempts); n > 0 {
				handoff.Attempts[n-1].VerifyState = string(state)
				handoff.Attempts[n-1].VerifySummary = summary
			}
			return closingText(state, summary, reason), nil
		}

		// 3.3 桌面执行：仅执行一步动作；记录本轮尝试与是否产生可见变化
		attempt := desktopexecute.Act(ctx, decision, handoff)
		attempt.Round = round
		if attempt.Duplicate {
			logger.Info("LunarHarness", chainScopeLabel+"检测到重复输入：%s，已跳过", attempt.InputText)
			return "已跳过重复输入：窗口标题已包含「" + attempt.InputText + "」；未执行新动作，任务是否完成尚未核对。", nil
		}
		handoff.LastAction = attempt.Result
		handoff.Attempts = desktop.AppendAttempt(handoff.Attempts, attempt)
		if attempt.Changed {
			stagnant = 0
		} else {
			stagnant++
		}
		if stagnant >= 2 {
			logger.Warn("LunarHarness", chainScopeLabel+"连续 %d 轮操作未产生可见变化，判定停滞", stagnant)
			return "连续两轮操作未产生可见变化，任务停滞，已暂停并等待人工介入", nil
		}

		// 3.4 确定性推进交接记录（不再调用“书记者”模型，避免无依据的状态臆测）
		handoff = desktop.AdvanceHandoff(handoff, attempt)
	}
	logger.Warn("LunarHarness", chainScopeLabel+"已达到最大执行轮次，任务暂停")
	return "已达到最大执行轮次，任务暂停", nil
}
