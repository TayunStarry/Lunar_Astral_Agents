package viewbase

// viewbase 观影者系列智能体的共用模型调用基座。
//
// 四个观影者（阅片 / 审查 / 评论 / 分享）的调用形态相同：纯文本或图文混排、
// 统一的循环监管、瞬时错误重试、推理实况登记与智能修复上报。
// 这里只保留这一层共用能力，不承载任何观影业务判断。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

const (
	// defaultAttempts 文本类调用的默认尝试次数（含首次）
	defaultAttempts = 3
	// defaultTimeout 单次调用超时
	defaultTimeout = 5 * time.Minute
	// retryBackoff 瞬时错误重试的基础退避
	retryBackoff = time.Second
	// loggerCategory 日志分类（沿用观影链既有分类）
	loggerCategory = "BilibiliViewer"
)

// ChatRequest 一次观影者模型调用。
type ChatRequest struct {
	// Agent 故障归属 ID（智能修复按此去重）
	Agent string
	// Role 推理实况角色标签
	Role string
	// Model 模型名（空 = 主多模态模型）
	Model string
	// System 系统提示词
	System string
	// User 用户输入
	User string
	// Images 图文混排时附带的图片（data URL 或 http 地址）
	Images []string
	// Attempts 总尝试次数（含首次；<1 视为 1）
	Attempts int
}

// chatMessage OpenAI 兼容消息。
type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// imageURL 图片地址。
type imageURL struct {
	URL string `json:"url"`
}

// contentPart 图文混排片段。
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

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
