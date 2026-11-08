package MemoryFuser

// agent.go 记忆融合智能体：把多条高相似度记忆整合为一条完整记忆。
// 模型调用与推理实况统一走 protocol/agentbase。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Merge 把多条记忆整合为一条正文。
func Merge(ctx context.Context, contents []string) (merged string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	if ctx == nil {
		ctx = context.Background()
	}
	if len(contents) == 0 {
		return "", fmt.Errorf("无可整合的记忆")
	}

	clipped := make([]string, len(contents))
	for i, content := range contents {
		clipped[i] = fmt.Sprintf("%d. %s", i+1, clip(content, maxContentChars))
	}

	temp := float64(temperature)
	reply, err := agentbase.Stream(ctx, agentbase.StreamRequest{
		Agent: AgentID,
		Role:  modelRole,
		Messages: []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": "待合并的记忆：\n" + strings.Join(clipped, "\n\n")},
		},
		MaxTokens:   maxTokens,
		Temperature: &temp,
		// 与标签生成一致：显式关闭思考，避免推理模型把 token 预算耗在 reasoning 上
		Extra:   map[string]any{"chat_template_kwargs": map[string]any{"enable_thinking": false}},
		Timeout: callTimeout,
	})
	if err != nil {
		return "", err
	}

	content := strings.TrimSpace(reply.Text)
	if content == "" {
		return "", fmt.Errorf("整合模型返回空内容")
	}
	return content, nil
}

// clip 按字符截断单条内容。
func clip(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
