package Savior

// agent.go 后勤 Tick 内的修复流程：取故障、跑一次维修、成功则清历史、失败则重试或升级。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Process 在后勤 Tick 内处理一个故障（一次只修一个，避免单 Tick 过长）。
func Process(ctx context.Context) {
	failure := take()
	if failure == nil {
		return
	}
	if failure.Escalated {
		escalateFailure(failure, fmt.Sprintf("同类故障连续出现 %d 次，不再自动重试", failure.Occurrences))
		return
	}
	result, err := repair(ctx, failure)
	if err == nil {
		finishFailure(failure, result)
		return
	}
	logger.Warn(roleLabel, "[智能修复] 故障 #%d（%s/%s）修复失败：%v",
		failure.ID, failure.Agent, failure.Kind, err)
	failure.Attempts++
	failure.Conclusions = appendBounded(failure.Conclusions, previewRunes(err.Error(), 300))
	if failure.Attempts >= maxAttempts {
		escalateFailure(failure, fmt.Sprintf("自动修复 %d 次未成功", maxAttempts))
		return
	}
	requeue(failure.Key)
}

// finishFailure 根据维修结论决定「已解决」还是「升级」。
func finishFailure(failure *Failure, result repairResult) {
	if result.Status == statusNeedsHuman || result.Status == statusUnfixable {
		escalateFailure(failure, result.Summary)
		return
	}
	resolve(failure.Key)
	logger.Info(roleLabel, "[智能修复] 故障 #%d（%s/%s）修复完成：%s",
		failure.ID, failure.Agent, failure.Kind, previewRunes(result.Summary, maxBriefRunes))
}

// repair 让模型带着工具尝试修复一次。
func repair(ctx context.Context, failure *Failure) (repairResult, error) {
	system, err := buildSystemPrompt(failure)
	if err != nil {
		return repairResult{}, err
	}
	session := &repairSession{}
	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 "savior",
		Label:              roleLabel,
		MaxRounds:          maxToolRounds,
		MaxRepeats:         2,  // 同一响应第二次出现即判定重复
		MaxToolResultRunes: -1, // 工具结果已由工具侧限量，原样回填
	})
	result, err := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Messages: []agentbase.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: fmt.Sprintf("请处理故障 #%d：%s 报错「%s」。先看简报，按需查看目标状态，动手修复后必须验证，最后 finish。",
				failure.ID, failure.Agent, previewRunes(failure.Error, maxBriefRunes))},
		},
		Tools: repairTools(),
		Stream: agentbase.StreamRequest{
			Agent:       "savior",
			Role:        roleLabel,
			Timeout:     modelTimeout,
			ReportTools: true,
		},
		Execute: func(ctx context.Context, call agentbase.ToolCall) (string, bool, error) {
			outcome := executeTool(ctx, session, failure, call.Function.Name, call.Function.Arguments)
			return outcome.JSON, outcome.Done, nil
		},
		// 修复器自身不上报，避免自修复递归。
		DisableReport: true,
	})
	if err != nil {
		if errors.Is(err, agentbase.ErrMaxRounds) {
			return repairResult{}, fmt.Errorf("在 %d 轮内没有完成修复（已执行 %d 个动作）", maxToolRounds, session.actions)
		}
		if errors.Is(err, agentbase.ErrLoop) {
			return repairResult{}, fmt.Errorf("%w（重复调用，已执行 %d 个动作）", agentbase.ErrLoop, session.actions)
		}
		return repairResult{}, err
	}
	if session.status != "" {
		return repairResult{Summary: session.summary, Status: session.status}, nil
	}
	text := strings.TrimSpace(result.Reply.Text)
	if text == "" {
		return repairResult{}, fmt.Errorf("拯救者没有给出结论（%s）", previewRunes(result.Reply.Reasoning, maxBriefRunes))
	}
	return repairResult{Summary: text, Status: statusFixed}, nil
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

// escalateFailure 放弃自动修复，记录结论并进入静默期。
func escalateFailure(failure *Failure, conclusion string) {
	conclusion = strings.TrimSpace(conclusion)
	if conclusion == "" {
		conclusion = "自动修复未能解决，需要人工介入"
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
	logger.Warn(roleLabel, "[智能修复] 故障 #%d（%s/%s）标记为需人工介入：%s；已排查结论：%s",
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
