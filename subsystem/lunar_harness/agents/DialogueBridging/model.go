package DialogueBridging

// model.go 规划模型调用：统一走 protocol/agentbase 的流式调用、
// 推理实况、循环监管与智能修复，不再维护本地的 SSE / 观察者实现。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// chatText 带思考监管的规划调用：命中重复时中断并重试。
func chatText(system, user string) (string, error) {
	reply, err := agentbase.StreamWithRetry(context.Background(), agentbase.StreamRequest{
		Agent: "dialogue_bridging",
		Role:  roleLabel,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Timeout: 90 * time.Second,
	}, agentbase.RetryOptions{
		MaxAttempts: loopMaxRetries + 1,
		RetryLoop:   true,
		OnRetry: func(attempt int, err error) {
			logger.Warn("DialogueBridging", "规划模型陷入重复输出，已中断并重试（%d/%d）", attempt, loopMaxRetries)
		},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(reply.Text) == "" {
		return "", fmt.Errorf("模型返回空内容")
	}
	return reply.Text, nil
}

// truncate 按字符截断并追加省略号
func truncate(text string, limit int) string {
	return agentbase.TruncateRunes(text, limit)
}
