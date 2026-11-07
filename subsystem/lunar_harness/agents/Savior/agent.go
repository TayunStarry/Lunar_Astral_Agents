package Savior

// agent.go 后勤 Tick 内的诊断流程：取故障、读状态、生成维修建议。
//
// 拯救者只诊断不操作：不调用任何会改变目标状态的入口，
// 输出是一份结构化建议，交由人或其他机制执行。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Process 在后勤 Tick 内处理一个故障（一次只诊断一个，避免单 Tick 过长）。
func Process(ctx context.Context) {
	failure := take()
	if failure == nil {
		return
	}
	advice, err := diagnose(ctx, failure)
	if err == nil {
		publishAdvice(failure, advice)
		return
	}
	logger.Warn(roleLabel, "["+roleLabel+"] 故障 #%d（%s/%s）诊断失败：%v",
		failure.ID, failure.Agent, failure.Kind, err)
	failure.Attempts++
	failure.Conclusions = appendBounded(failure.Conclusions, previewRunes(err.Error(), 300))
	if failure.Escalated || failure.Attempts >= maxAttempts {
		escalateFailure(failure, fmt.Sprintf("自动诊断 %d 次未成功，需要人工查看", maxAttempts))
		return
	}
	requeue(failure.Key)
}

// publishAdvice 输出建议：同类故障反复出现时仍出建议，但不再自动重试。
func publishAdvice(failure *Failure, advice *Advice) {
	storeAdvice(failure, advice)
	report := formatAdvice(failure, advice)
	if failure.Escalated {
		logger.Info(roleLabel, "%s", report)
		escalateFailure(failure, fmt.Sprintf("同类故障连续出现 %d 次，已给出维修建议，等待人工执行", failure.Occurrences))
		return
	}
	resolve(failure.Key)
	logger.Info(roleLabel, "%s", report)
}

// diagnose 让模型带着只读工具做一次诊断，返回结构化建议。
func diagnose(ctx context.Context, failure *Failure) (*Advice, error) {
	system, err := buildSystemPrompt(failure)
	if err != nil {
		return nil, err
	}
	session := &diagnoseSession{}
	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 "savior",
		Label:              roleLabel,
		MaxRounds:          maxToolRounds,
		MaxToolResultRunes: -1, // 工具结果已由工具侧限量，原样回填
	})
	result, err := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Messages: []agentbase.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: fmt.Sprintf("请诊断故障 #%d：%s 报错「%s」。先看简报，按需查看目标状态，最后用 finish 提交结构化维修建议（不要尝试执行修复）。",
				failure.ID, failure.Agent, previewRunes(failure.Error, maxBriefRunes))},
		},
		Tools: diagnoseTools(),
		Stream: agentbase.StreamRequest{
			Agent:       "savior",
			Role:        roleLabel,
			Timeout:     modelTimeout,
			ReportTools: true,
		},
		Execute: func(_ context.Context, call agentbase.ToolCall) (string, bool, error) {
			outcome := executeTool(session, failure, call.Function.Name, call.Function.Arguments)
			return outcome.JSON, outcome.Done, nil
		},
		// 维修器自身不上报，避免自修复递归。
		DisableReport: true,
	})
	if err != nil {
		if errors.Is(err, agentbase.ErrMaxRounds) {
			return nil, fmt.Errorf("在 %d 轮内没有给出诊断建议（已执行 %d 个只读动作）", maxToolRounds, session.actions)
		}
		if errors.Is(err, agentbase.ErrLoop) {
			return nil, fmt.Errorf("%w（模型输出陷入重复，已执行 %d 个只读动作）", agentbase.ErrLoop, session.actions)
		}
		return nil, err
	}
	if session.advice != nil {
		return session.advice, nil
	}
	text := strings.TrimSpace(result.Reply.Text)
	if text == "" {
		return nil, fmt.Errorf("拯救者没有给出结论（%s）", previewRunes(result.Reply.Reasoning, maxBriefRunes))
	}
	// 模型没调 finish 时，仍把这段文字作为判断，避免建议丢失。
	return &Advice{
		FailureID:  failure.ID,
		Agent:      failure.Agent,
		Kind:       failure.Kind,
		Error:      failure.Error,
		Diagnosis:  previewRunes(text, maxAdviceRunes),
		Confidence: confidenceLow,
		Basis:      "模型直接作答，未调用 finish",
		At:         time.Now(),
	}, nil
}

// formatAdvice 把建议排成控制台可读的多行文本。
func formatAdvice(failure *Failure, advice *Advice) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "[%s] 故障 #%d（%s/%s）诊断建议\n", roleLabel, failure.ID, failure.Agent, failure.Kind)
	fmt.Fprintf(&builder, "判断：%s\n", orPlaceholder(advice.Diagnosis, "未能定位"))
	fmt.Fprintf(&builder, "可能原因：%s\n", orPlaceholder(advice.Causes, "未确定"))
	if len(advice.Steps) == 0 {
		builder.WriteString("建议步骤：（模型未给出可执行步骤，需人工补充）\n")
	} else {
		builder.WriteString("建议步骤：\n")
		for index, step := range advice.Steps {
			fmt.Fprintf(&builder, "  %d. %s\n", index+1, step)
		}
	}
	fmt.Fprintf(&builder, "风险点：%s\n", orPlaceholder(advice.Risks, "无"))
	fmt.Fprintf(&builder, "置信度：%s（依据：%s）\n", advice.Confidence, orPlaceholder(advice.Basis, "未说明"))
	builder.WriteString("说明：以上仅为维修建议，未对目标做任何修改。")
	return builder.String()
}

// orPlaceholder 空值时的占位文案。
func orPlaceholder(text, placeholder string) string {
	if strings.TrimSpace(text) == "" {
		return placeholder
	}
	return text
}

// take 取出最早的一条故障。
func take() *Failure {
	mu.Lock()
	defer mu.Unlock()
	for len(order) > 0 {
		key := order[0]
		order = order[1:]
		if failure, ok := pending[key]; ok {
			return failure
		}
	}
	return nil
}

// requeue 把故障放回队列末尾，等待下一次 Tick 重试。
func requeue(key string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := pending[key]; !ok {
		return
	}
	for index, item := range order {
		if item == key {
			order = append(order[:index], order[index+1:]...)
			break
		}
	}
	order = append(order, key)
}

// escalateFailure 放弃自动诊断，记录结论并进入静默期。
func escalateFailure(failure *Failure, conclusion string) {
	conclusion = strings.TrimSpace(conclusion)
	if conclusion == "" {
		conclusion = "自动诊断未能给出可用结论，需要人工介入"
	}
	mu.Lock()
	delete(pending, failure.Key)
	cooldown[failure.Agent] = time.Now().Add(salvageCooldown)
	record := &escalation{
		At:         time.Now(),
		Failure:    failure,
		Conclusion: previewRunes(conclusion, 600),
	}
	escalated[failure.Key] = record
	for len(escalated) > maxEscalated {
		oldestKey := ""
		var oldest time.Time
		for key, item := range escalated {
			if oldestKey == "" || item.At.Before(oldest) {
				oldestKey, oldest = key, item.At
			}
		}
		delete(escalated, oldestKey)
	}
	mu.Unlock()
	logger.Warn(roleLabel, "["+roleLabel+"] 故障 #%d（%s/%s）标记为需人工介入：%s；已排查结论：%s",
		failure.ID, failure.Agent, failure.Kind, previewRunes(failure.Error, 200), record.Conclusion)
}

// appendBounded 追加结论并只保留最近若干条。
func appendBounded(items []string, item string) []string {
	const limit = 4
	items = append(items, item)
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

// previewRunes 按字符截断。
func previewRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
