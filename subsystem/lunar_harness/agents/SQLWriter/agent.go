package SQLWriter

// agent.go 语句生成：把自然语言需求翻译成一条只读 SQL。
// 模型调用、推理实况与循环监管统一走 protocol/agentbase；本智能体只生成、不执行。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Generate 按当前库的表结构把自然语言需求翻译成一条只读 SQL。
// 返回文本可能是 SQL，也可能是以「无法生成」开头的说明，由调用方校验后再交给前端。
func Generate(ctx context.Context, schema, question string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	system := loadPrompt(promptName)
	if system == "" {
		return "", fmt.Errorf("读取语句生成提示词失败")
	}
	system = renderPrompt(system, "{schema}", schema)
	reply, err := agentbase.StreamWithRetry(ctx, agentbase.StreamRequest{
		Agent: AgentID,
		Role:  modelRole,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: question},
		},
		Timeout: requestTimeout,
	}, agentbase.RetryOptions{
		MaxAttempts: loopRetries + 1,
		RetryLoop:   true,
		OnRetry: func(attempt int, retryErr error) {
			logger.Warn(module, "["+modelRole+"] 输出陷入重复，已中断并重试（%d/%d）", attempt, loopRetries)
		},
	})
	agentbase.ReportRun(AgentID, err)
	if err != nil {
		return "", err
	}
	text := normalizeSQL(reply.Text)
	if text == "" {
		return "", fmt.Errorf("模型未返回 SQL")
	}
	return text, nil
}

// normalizeSQL 去掉模型偶尔附加的 Markdown 代码块与首尾空白。
func normalizeSQL(text string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	trimmed = strings.TrimPrefix(trimmed, "```")
	if newline := strings.IndexByte(trimmed, '\n'); newline >= 0 {
		// 去掉 ```sql 这类语言标记行
		trimmed = trimmed[newline+1:]
	}
	trimmed = strings.TrimSuffix(strings.TrimSpace(trimmed), "```")
	return strings.TrimSpace(trimmed)
}
