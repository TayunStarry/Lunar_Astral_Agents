package WebSearch

// model.go 网络搜索的模型调用：统一走 protocol/agentbase 的流式调用、
// 推理实况、循环监管与智能修复，不再维护本地的 SSE / 观察者实现。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// loggerWarn 统一日志入口
func loggerWarn(format string, args ...any) {
	logger.Warn("CrystalAstral", format, args...)
}

// chatWithOptions 带思考监管的对话调用：命中重复时中断并重试一次。
// 重试后仍失败就返回错误，由各调用点的既有降级逻辑兜底
// （检索词提炼回退原指令、分片摘要跳过、最终报告走纯文本兜底）。
func chatWithOptions(ctx context.Context, messages []chatMessage, opts chatOptions) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	reply, err := agentbase.StreamWithRetry(ctx, agentbase.StreamRequest{
		Agent:         "web_search",
		Role:          webSearchModelRole,
		Messages:      messages,
		Silent:        opts.Silent,
		Model:         opts.Model,
		Temperature:   opts.Temperature,
		TopP:          opts.TopP,
		RepeatPenalty: opts.RepeatPenalty,
		MaxTokens:     opts.MaxTokens,
		// prepareSearchModel 已持有模型切换租约：这里必须以「已持有」方式调用，
		// 否则 agentbase 会再次 AcquireCall，而 callGate 的写锁正被本 goroutine 持有，
		// RWMutex 不可重入 → 永久阻塞（表现为页面已打开但智能体再无任何响应）。
		HoldLease: true,
		Timeout:   120 * time.Second,
	}, agentbase.RetryOptions{
		MaxAttempts: webSearchLoopRetries + 1,
		RetryLoop:   true,
		OnRetry: func(attempt int, err error) {
			logger.Warn("CrystalAstral", "模型输出陷入重复，已中断并重试（%d/%d）", attempt, webSearchLoopRetries)
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

// searchChatOptions 搜索摘要模型的采样参数：低温、窄核采样，减少要点漂移
func searchChatOptions(model string) chatOptions {
	temperature := 0.2
	topP := 0.9
	repeatPenalty := 1.05
	return chatOptions{
		Model:         model,
		Temperature:   &temperature,
		TopP:          &topP,
		RepeatPenalty: &repeatPenalty,
	}
}

// profileChatOptions 按摘要档位生成调用选项（主模型沿用服务端默认采样）
func profileChatOptions(profile summaryProfile) chatOptions {
	if strings.TrimSpace(profile.Model) == "" {
		return chatOptions{}
	}
	return searchChatOptions(profile.Model)
}

// hardCutRunes 硬切断文本：总长不超过 max 字符（超长时以省略号收尾并标注）
func hardCutRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 1 {
		max = 1
	}
	return string(r[:max-1]) + "…（已截断）"
}

// truncateRunes 简单截断（用于日志/载荷），统一使用基座实现。
func truncateRunes(s string, n int) string {
	return agentbase.TruncateRunes(s, n)
}
