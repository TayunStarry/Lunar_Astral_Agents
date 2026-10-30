package Savior

// agent.go 故障上报与后勤 Tick 内的修复流程。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/loopguard"
)

// Report 上报一次智能体故障；同一智能体的故障只保留最新一条。
func Report(agent string, err error) {
	if err == nil {
		return
	}
	agent = strings.TrimSpace(agent)
	if agent == "" {
		agent = "unknown"
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "未知错误"
	}
	mu.Lock()
	defer mu.Unlock()
	if until, ok := cooldown[agent]; ok {
		if time.Now().Before(until) {
			return
		}
		delete(cooldown, agent)
	}
	if existing, ok := pending[agent]; ok {
		existing.Error = message
		existing.At = time.Now()
		return
	}
	if len(pending) >= maxPending {
		logger.Warn(roleLabel, "[智能修复] 故障队列已满，忽略 %s 的新故障", agent)
		return
	}
	seq++
	pending[agent] = &Failure{ID: seq, Agent: agent, Error: message, At: time.Now()}
	order = append(order, agent)
	logger.Warn(roleLabel, "[智能修复] 收到故障 #%d（%s）：%s", seq, agent, previewRunes(message, 160))
}

// Pending 返回待修复故障数。
func Pending() int {
	mu.Lock()
	defer mu.Unlock()
	return len(pending)
}

// Process 在后勤 Tick 内处理一个故障（一次只修一个，避免单 Tick 过长）。
func Process(ctx context.Context) {
	failure := take()
	if failure == nil {
		return
	}
	summary, err := repair(ctx, failure)
	if err == nil {
		logger.Info(roleLabel, "[智能修复] 故障 #%d（%s）修复完成：%s", failure.ID, failure.Agent, previewRunes(summary, 120))
		remove(failure.Agent)
		return
	}
	logger.Warn(roleLabel, "[智能修复] 故障 #%d（%s）修复失败：%v", failure.ID, failure.Agent, err)
	failure.Attempts++
	if failure.Attempts >= maxAttempts {
		remove(failure.Agent)
		silence(failure.Agent)
		return
	}
	requeue(failure.Agent)
}

// repair 让模型带着工具尝试修复一次。
func repair(ctx context.Context, failure *Failure) (string, error) {
	system, err := buildSystemPrompt(failure)
	if err != nil {
		return "", err
	}
	messages := []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: fmt.Sprintf("请修复故障 #%d：%s 报错「%s」", failure.ID, failure.Agent, previewRunes(failure.Error, 300))},
	}
	seen := map[string]int{}
	for round := 0; round < maxToolRounds; round++ {
		response, callErr := chatWithTools(ctx, messages, repairTools())
		if callErr != nil {
			return "", callErr
		}
		messages = append(messages, chatMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})
		if len(response.ToolCalls) == 0 {
			text := strings.TrimSpace(response.Content)
			if text == "" {
				return "", fmt.Errorf("拯救者没有给出结论（%s）", previewRunes(response.Reasoning, 120))
			}
			return text, nil
		}
		key := signatureOf(response)
		seen[key]++
		if seen[key] >= 2 {
			return "", fmt.Errorf("%w（第 %d 轮出现重复调用）", loopguard.ErrLoop, round+1)
		}
		for _, call := range response.ToolCalls {
			outcome := executeTool(ctx, failure, call.Function.Name, call.Function.Arguments)
			messages = append(messages, chatMessage{Role: "tool", ToolID: call.ID, Content: outcome.JSON})
			if outcome.Done {
				return outcome.Summary, nil
			}
		}
	}
	return "", fmt.Errorf("在 %d 轮内没有完成修复", maxToolRounds)
}

// take 取出最早的一条故障。
func take() *Failure {
	mu.Lock()
	defer mu.Unlock()
	for len(order) > 0 {
		agent := order[0]
		order = order[1:]
		if failure, ok := pending[agent]; ok {
			return failure
		}
	}
	return nil
}

// remove 移除该智能体的故障记录。
func remove(agent string) {
	mu.Lock()
	delete(pending, agent)
	mu.Unlock()
}

// requeue 把故障放回队列末尾，等待下一次 Tick 重试。
func requeue(agent string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := pending[agent]; !ok {
		return
	}
	for index, item := range order {
		if item == agent {
			order = append(order[:index], order[index+1:]...)
			break
		}
	}
	order = append(order, agent)
}

// silence 放弃修复并进入静默期。
func silence(agent string) {
	mu.Lock()
	defer mu.Unlock()
	cooldown[agent] = time.Now().Add(salvageCooldown)
	logger.Warn(roleLabel, "[智能修复] %s 暂不修复（%d 次尝试未成功），静默 %v", agent, maxAttempts, salvageCooldown)
}

// signatureOf 生成一轮响应签名，用于循环检测。
func signatureOf(response *modelResponse) string {
	var b strings.Builder
	b.WriteString(response.Content)
	for _, call := range response.ToolCalls {
		b.WriteString("|")
		b.WriteString(call.Function.Name)
		b.WriteString(":")
		b.WriteString(call.Function.Arguments)
	}
	return b.String()
}

// previewRunes 按字符截断。
func previewRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
