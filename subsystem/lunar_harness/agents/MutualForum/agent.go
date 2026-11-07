package MutualForum

// agent.go Mutual Forum 智能体的对外入口：自然语言需求 → 多轮工具调用 → 中文答复。
//
// 设计要点：
//   - 本包只负责模型调用、工具声明与实现、实况上报；
//   - 独立上下文（最近 40 条「需求 + 答复」）、串行化与整体超时由论坛链
//     chain/forum 负责，RunRound 以入参形式接收历史；
//   - 多轮工具循环：模型通过 function calling 逐步取证/操作，上限 8 轮；
//   - 条件可用：桥接层 BridgeReady() 为 false 时工具不暴露，RunRound 也会拒绝执行。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Ready 返回 Mutual Forum 是否应暴露给月华。
func Ready() bool {
	return BridgeReady()
}

// RunRound 执行单轮互助论坛模型调用：状态检查 → 系统提示 → 历史 + 指令 → 多轮工具循环。
// 上下文、串行化与整体超时由 chain/forum 负责；本函数只做模型调用、工具执行与实况上报。
func RunRound(ctx context.Context, instruction string, history []agentbase.Message) (string, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", errors.New("请用自然语言说明要在论坛里做什么")
	}

	cfg, err := LoadConfig()
	if err != nil {
		return "", fmt.Errorf("互助论坛不可用：%w（配置文件：%s）", err, ConfigPath())
	}
	if err := cfg.Validate(); err != nil {
		return "", fmt.Errorf("互助论坛不可用：%w（请检查 %s 的 enabled 与 base_url）", err, ConfigPath())
	}

	if !BridgeReady() {
		return "", errors.New("互助论坛当前不可用：论坛健康检查未通过，或尚未配置可用的服务地址")
	}

	status, statusErr := InspectStatus(ctx)
	if statusErr != nil {
		return "", fmt.Errorf("互助论坛状态检查失败: %w", statusErr)
	}
	if status == nil || !status.Reachable {
		reason := "论坛健康检查未通过"
		if status != nil && status.Error != "" {
			reason = status.Error
		}
		return "", fmt.Errorf("互助论坛当前不可用：%s", reason)
	}

	systemPrompt, err := buildSystemPrompt(status)
	if err != nil {
		return "", err
	}

	messages := make([]agentbase.Message, 0, len(history)+8)
	messages = append(messages, agentbase.Message{Role: "system", Content: systemPrompt})
	messages = append(messages, history...)
	messages = append(messages, agentbase.Message{Role: "user", Content: instruction})

	lastSummary := ""
	toolCalls := 0
	// 整轮只在 Run 开头/结尾各标记一次实况，多轮工具调用合并成同一轮；
	// 否则每个工具轮次都会留下一条只有标题、没有内容的空实况。
	observeBegin := func() { agentbase.BeginLive(roleLabel) }
	observeEnd := func() { agentbase.EndLive(roleLabel) }
	observeBegin()
	defer observeEnd()

	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 "mutual_forum",
		Label:              roleLabel,
		MaxRounds:          maxToolRounds,
		MaxToolResultRunes: -1, // 工具结果原样回填 JSON
	})
	result, callErr := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Messages: messages,
		Tools:    forumTools(),
		Stream: agentbase.StreamRequest{
			Agent:      "mutual_forum",
			Role:       roleLabel,
			Timeout:    modelTimeout,
			ManageLive: true,
		},
		Execute: func(ctx context.Context, call agentbase.ToolCall) (string, bool, error) {
			toolCalls++
			logger.Info(roleLabel, "[实况] 工具 %s（%s）", call.Function.Name, previewText(call.Function.Arguments, 200))
			args, parseErr := agentbase.ParseToolArgs(call.Function.Arguments)
			if parseErr != nil {
				outcome := toolError(fmt.Errorf("工具 %s 的参数不是合法 JSON: %w", call.Function.Name, parseErr))
				lastSummary += outcome.Summary + "；"
				return outcome.JSON, false, nil
			}
			outcome := executeTool(ctx, call.Function.Name, args)
			lastSummary += outcome.Summary + "；"
			return outcome.JSON, false, nil
		},
		// 保持原有「先日志、再上报」的时机。
		DisableReport: true,
	})
	if callErr != nil {
		if errors.Is(callErr, agentbase.ErrMaxRounds) {
			final := "互助论坛在限定轮次内没有完成这次请求。"
			if lastSummary != "" {
				final += "已执行的步骤：" + strings.TrimSuffix(lastSummary, "；") + "。请换一种更具体的说法再试。"
			}
			recordLastRun(result.Rounds, toolCalls, "")
			logger.Warn(roleLabel, "工具轮次达到上限（%d），已停止继续调用", maxToolRounds)
			return final, nil
		}
		recordLastRun(result.Rounds+1, toolCalls, callErr.Error())
		logger.Warn(roleLabel, "模型调用失败（第 %d 轮）: %v", result.Rounds+1, callErr)
		// 交给智能修复：论坛上下文过长或含脏数据时会持续复现
		agentbase.ReportRun("mutual_forum", callErr)
		return "", fmt.Errorf("互助论坛模型调用失败: %w", callErr)
	}

	final := strings.TrimSpace(result.Reply.Text)
	if final == "" {
		if lastSummary != "" {
			final = "互助论坛已执行：" + strings.TrimSuffix(lastSummary, "；")
		} else {
			final = "互助论坛没有生成可用的答复，请换一种说法再试一次。"
		}
	}
	recordLastRun(result.Rounds, toolCalls, "")
	return final, nil
}

/* ---------------- 最近一次模型轮次（诊断用） ---------------- */

// recordLastRun 记录最近一次模型轮次的诊断信息，供 agentdump / agentrepair 使用。
func recordLastRun(rounds, toolCalls int, errText string) {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	lastRun = map[string]any{
		"at":         time.Now().Format(time.RFC3339),
		"rounds":     rounds,
		"tool_calls": toolCalls,
		"error":      errText,
	}
}

// snapshotLastRun 返回最近一次模型轮次诊断信息的副本。
func snapshotLastRun() map[string]any {
	lastRunMu.Lock()
	defer lastRunMu.Unlock()
	out := make(map[string]any, len(lastRun))
	for key, value := range lastRun {
		out[key] = value
	}
	return out
}

// resetLastRun 清空最近一次模型轮次诊断信息。
func resetLastRun() {
	lastRunMu.Lock()
	lastRun = nil
	lastRunMu.Unlock()
}

/* ---------------- 独立上下文已上移到 chain/forum ---------------- */
