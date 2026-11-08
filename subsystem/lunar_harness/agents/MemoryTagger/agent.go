package MemoryTagger

// agent.go 记忆标签智能体：为文本、图片与动态图生成中文标签。
// 模型调用、推理实况与失败上报统一走 protocol/agentbase。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Generate 生成标签，最多重试 maxRetries 次。
// mediaRefs 非空时按动态图采样帧生成；否则按 isImage 走文本或单图分支。
func Generate(ctx context.Context, content string, isImage bool, orientation, custom string, mediaRefs []string) (tags []string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	if ctx == nil {
		ctx = context.Background()
	}

	req := Request{
		Content:     content,
		IsImage:     isImage,
		Orientation: orientation,
		Custom:      custom,
		MediaRefs:   mediaRefs,
	}
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		tags, lastErr = generateOnce(ctx, req)
		if lastErr == nil && len(tags) > 0 {
			return tags, nil
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("标签结果为空")
		}
		if attempt < maxRetries-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryWait):
			}
		}
	}
	return nil, fmt.Errorf("标签生成失败（已重试 %d 次）: %w", maxRetries, lastErr)
}

// generateOnce 单次标签生成尝试。
func generateOnce(ctx context.Context, req Request) ([]string, error) {
	reply, err := chat(ctx, buildMessages(req))
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(reply.Text)
	if raw == "" {
		if reasoning := strings.TrimSpace(reply.Reasoning); reasoning != "" {
			return nil, fmt.Errorf("模型返回空内容（仅推理内容: %s）", truncate(reasoning, 512))
		}
		return nil, fmt.Errorf("模型返回空内容")
	}
	tags, err := parseTagsJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("解析标签失败: %w (原始响应: %s)", err, truncate(raw, 512))
	}
	return tags, nil
}

// chat 单次标签模型调用。
func chat(ctx context.Context, messages []chatMessage) (agentbase.Reply, error) {
	temp := float64(temperature)
	return agentbase.Stream(ctx, agentbase.StreamRequest{
		Agent:       AgentID,
		Role:        modelRole,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: &temp,
		// 标签生成只需要一段 JSON，不需要思考过程。推理模型默认会先输出很长的
		// reasoning_content，max_tokens 被思考耗尽后 content 为空。
		Extra:   map[string]any{"chat_template_kwargs": map[string]any{"enable_thinking": false}},
		Timeout: callTimeout,
	})
}
