//go:build windows

// Package desktop 桌面操纵：多角色协作的桌面自动化智能体。
//
// 编纂者先把自然语言请求收敛成结构化任务，启动者负责把目标应用带到前台，
// 之后由 UIA 理解者 / 规划者 / 单一操作者（键盘、鼠标或 UIA）循环推进，
// 直到达成验收标准或判定停滞。提示词统一放在 prompts/desktop*.md。
package desktop

// ==== 宿主循环调度器 ====
// 编排桌面操纵全流程：初始构建（编者→启动者）+ 执行循环
//（视觉→UIA→规划→仅一个操作者→进度书记→下一轮），
// 每个角色独立全新上下文，工具白名单物理隔离。
//
// 编排层职责（不属于任何一个模型角色）：
//   - 每轮开始确保目标应用在前台，避免对着控制台/资源管理器空转；
//   - 记录每轮尝试与是否产生可见变化，回灌给规划者；
//   - 连续两轮无变化即判定停滞并暂停，而不是耗尽轮次。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Run 运行桌面操纵，处理一条用户指令，返回结果文本。
// ctx 取消时立即中断：每个阶段开始前检查，在途模型请求随请求上下文一并取消。
func Run(ctx context.Context, instruction string) (resultText string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("桌面操纵内部异常: %v", r)
		}
	}()

	if err := ctx.Err(); err != nil {
		return "", ErrCanceled
	}

	// —— 阶段 1：提示词编纂者（无工具） ——
	plan, err := editorPhase(ctx, instruction)
	if err != nil {
		return "", err
	}
	var handoff HandoffRecord

	// —— 阶段 2：软件启动者 ——
	if err := launcherPhase(ctx, plan, &handoff); err != nil {
		if errors.Is(err, ErrCanceled) {
			return "", ErrCanceled
		}
		return "", fmt.Errorf("软件启动失败: %v", err)
	}

	// —— 阶段 3：执行循环 ——
	stagnant := 0
	for round := 1; round <= autoMaxLoops; round++ {
		if err := ctx.Err(); err != nil {
			return "", ErrCanceled
		}
		// 目标窗口保活：每轮开始先确认目标应用在前台，避免对着控制台/资源管理器空转
		if err := ensureTargetForeground(ctx, handoff.TargetApp); err != nil {
			if errors.Is(err, ErrCanceled) {
				return "", ErrCanceled
			}
			logger.Warn("LunarHarness", "桌面操纵第 %d 轮目标窗口未就绪: %v", round, err)
			return "目标窗口未就绪，任务暂停：" + err.Error(), nil
		}
		handoff.Round = round
		handoff.Foreground = DTForegroundTitle()
		handoff.ForegroundProcess = dtForegroundProcess()
		handoff.VisualSummary = "" // 视觉摘要已移除：避免错误画面描述干扰规划

		// 3.1 UIA 理解者（读 UI 树/定向查）
		treeSummary, elements := uiaReaderPhase(ctx, plan)
		if err := ctx.Err(); err != nil {
			return "", ErrCanceled
		}
		// 3.2 任务规划者（无工具，决策）
		decision := plannerPhase(ctx, plan, handoff, treeSummary, elements)
		if err := ctx.Err(); err != nil {
			return "", ErrCanceled
		}

		if decision.Decision == "complete" {
			return decision.Action, nil
		}

		// 3.3 仅启用一个操作者；记录本轮尝试与是否产生可见变化
		attempt := operationPhase(ctx, decision, handoff)
		attempt.Round = round
		if attempt.Duplicate {
			logger.Info("LunarHarness", "桌面操纵检测到重复输入：%s，已跳过并判定完成", attempt.InputText)
			return "任务完成：窗口标题已包含输入内容「" + attempt.InputText + "」，已跳过重复输入", nil
		}
		handoff.LastAction = attempt.Result
		handoff.Attempts = appendAttempt(handoff.Attempts, attempt)
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
		handoff = advanceHandoff(handoff, attempt)
	}
	logger.Warn("LunarHarness", "桌面操纵已达到最大执行轮次，任务暂停")
	return "已达到最大执行轮次，任务暂停", nil
}

// extractJSON 从模型输出文本中提取第一组大括号包裹的 JSON 片段。
func extractJSON(s string) []byte {
	clean := strings.TrimSpace(s)
	start := strings.IndexByte(clean, '{')
	end := strings.LastIndexByte(clean, '}')
	if start < 0 || end <= start {
		return []byte(s)
	}
	return []byte(clean[start : end+1])
}

// base64Encode 将字节数据编码为标准 base64 字符串（用于内嵌图片）。
func base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
