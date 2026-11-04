package SearchPlanner

// agent.go 搜索者-规划：把自然语言需求一步决策为「管线 + 检索词 + 参数 + 把握」。
// 模型调用、推理实况与循环监管统一走 protocol/agentbase。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Plan 调用规划模型产出结构化决策；调用或解析失败时返回检索管线兜底决策。
// 返回值中的 error 表示模型/解析失败（调用方据此记日志），决策本身仍可用。
func Plan(ctx context.Context, instruction, model string) (decision Decision, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	if ctx == nil {
		ctx = context.Background()
	}
	fallback := Decision{Pipeline: PipelineSearch, Query: instruction}

	system := loadPrompt(promptName)
	if system == "" {
		return fallback, fmt.Errorf("读取规划提示词失败")
	}
	system = renderPrompt(system, "{pipelines}", routeCandidates())
	text, callErr := chat(ctx, []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: instruction},
	}, model)
	if callErr != nil {
		return fallback, fmt.Errorf("路由模型调用失败，按检索管线处理: %w", callErr)
	}
	decision, parseErr := parseDecision(text)
	if parseErr != nil {
		return fallback, fmt.Errorf("路由输出解析失败，按检索管线处理: %w", parseErr)
	}
	if strings.TrimSpace(decision.Query) == "" {
		decision.Query = instruction
	}
	recordLastPlan(instruction, decision)
	return decision, nil
}

// chat 规划模型的单次调用：已在链路侧持有模型切换租约，故以 HoldLease 调用。
func chat(ctx context.Context, messages []chatMessage, model string) (string, error) {
	req := agentbase.StreamRequest{
		Agent:     AgentID,
		Role:      modelRole,
		Messages:  messages,
		Model:     model,
		HoldLease: true,
		Timeout:   120 * time.Second,
	}
	if strings.TrimSpace(model) != "" {
		temperature, topP, repeatPenalty := 0.2, 0.9, 1.05
		req.Temperature = &temperature
		req.TopP = &topP
		req.RepeatPenalty = &repeatPenalty
	}
	reply, err := agentbase.StreamWithRetry(ctx, req, agentbase.RetryOptions{
		MaxAttempts: loopRetries + 1,
		RetryLoop:   true,
		OnRetry: func(attempt int, err error) {
			logger.Warn("CrystalAstral", "模型输出陷入重复，已中断并重试（%d/%d）", attempt, loopRetries)
		},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(reply.Text) == "" {
		return "", fmt.Errorf("模型 API 返回空响应")
	}
	return reply.Text, nil
}
