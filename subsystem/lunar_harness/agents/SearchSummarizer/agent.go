package SearchSummarizer

// agent.go 搜索者-摘要：把全部来源页正文拼接为长文，一次调用按原始指令做针对性提取。
// 模型调用、推理实况与循环监管统一走 protocol/agentbase。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Digest 统一摘要入口：instruction 是原始指令，作为噪音剔除的准绳。
func Digest(ctx context.Context, instruction, query string, sources []Source, model string) (out string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	if ctx == nil {
		ctx = context.Background()
	}
	if len(sources) == 0 {
		return "", fmt.Errorf("无可摘要的页面正文")
	}

	system, userTemplate := loadPromptPair(promptName)
	user := renderPrompt(userTemplate,
		"{instruction}", instruction,
		"{query}", query,
		"{longtext}", encodeSources(sources))
	text, err := chat(ctx, []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, model)
	if err != nil {
		return "", err
	}
	if isNoRelevanceDigest(text) {
		// 模型仍可能退回「未检索到」占位句；直接改用页面正文摘要，
		// 保证调用方拿到的是内容而不是一句否定说明。
		out = pageContentDigest(sources)
	} else {
		out = text
	}
	recordLastDigest(query, out)
	return out, nil
}

// chat 摘要模型的单次调用：已在链路侧持有模型切换租约，故以 HoldLease 调用。
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
