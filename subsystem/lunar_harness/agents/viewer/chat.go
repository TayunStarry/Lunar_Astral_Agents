// Package viewer 观影族：哔哩观影链驱动的五个观影者智能体与共用调用基座。
//
// 族根只承载成员共用的数据载体（ChatRequest）、族级 ID / 显示名 / 超时与调用基座，
// 不 import 任何成员包；成员包 import 族根，依赖方向单向。
//
// 五个成员（阅片 / 审查 / 评论 / 观感 / 分享）的调用形态相同：纯文本或图文混排、
// 统一的循环监管、瞬时错误重试、推理实况登记与智能修复上报。
// 这里只保留这一层共用能力，不承载任何观影业务判断。
package viewer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Chat 执行一次观影者模型调用：统一重试、实况、循环监管与修复上报。
func Chat(ctx context.Context, req ChatRequest) (string, error) {
	attempts := req.Attempts
	if attempts < 1 {
		attempts = 1
	}
	reply, err := agentbase.StreamWithRetry(ctx, agentbase.StreamRequest{
		Agent:     req.Agent,
		Role:      req.Role,
		Messages:  buildMessages(req.System, req.User, req.Images),
		Model:     req.Model,
		Timeout:   defaultTimeout,
		HoldLease: false,
	}, agentbase.RetryOptions{
		MaxAttempts:  attempts,
		RetryLoop:    true,
		RetryStatus:  true,
		RetryNetwork: true,
		Backoff: func(attempt int) time.Duration {
			return time.Duration(attempt) * retryBackoff
		},
		OnRetry: func(attempt int, err error) {
			logger.Warn(loggerCategory, "%s第 %d 次调用失败，准备重试: %v", req.Role, attempt, err)
		},
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(reply.Text)
	if text == "" {
		return "", fmt.Errorf("模型返回空内容")
	}
	return text, nil
}

// buildMessages 组装纯文本或图文混排消息。
func buildMessages(system, user string, images []string) []chatMessage {
	clean := make([]string, 0, len(images))
	for _, image := range images {
		if strings.TrimSpace(image) != "" {
			clean = append(clean, image)
		}
	}
	if len(clean) == 0 {
		return []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		}
	}
	parts := make([]contentPart, 0, len(clean)+1)
	parts = append(parts, contentPart{Type: "text", Text: user})
	for _, image := range clean {
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: image}})
	}
	return []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: parts},
	}
}
