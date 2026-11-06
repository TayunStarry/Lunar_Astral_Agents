package forum

// repair_summary.go 论坛上下文的摘要重建。
//
// 论坛上下文是「一轮需求 + 一轮答复」的短历史，模型摘要风险低：
// 把较早的消息压成保留事实的摘要，再拼上最近若干条，避免整段历史越积越长。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// summarizePrompt 论坛历史摘要的约束。
const summarizePrompt = "你是论坛历史压缩器。把下面的历史消息压缩成要点，必须保留：" +
	"提问者、需求主题、关键条件与数字、已给出的结论、尚未解决的问题。" +
	"不要添加原文没有的信息，不要改写人格设定，不要输出无关寒暄。只输出要点正文。"

// summarizeForumContext 把较早历史压缩成摘要后重建上下文。
func summarizeForumContext(keepLast, targetRunes int) (string, error) {
	historyMu.Lock()
	snapshot := append([]agentbase.Message(nil), history...)
	historyMu.Unlock()
	if len(snapshot) <= keepLast {
		return "", fmt.Errorf("论坛上下文只有 %d 条，少于保留数 %d，无需摘要", len(snapshot), keepLast)
	}
	older := snapshot[:len(snapshot)-keepLast]
	recent := snapshot[len(snapshot)-keepLast:]
	digest, err := buildForumDigest(older, targetRunes)
	if err != nil {
		return "", err
	}
	summary := agentbase.Message{Role: "system", Content: "[论坛历史摘要]\n" + digest}
	historyMu.Lock()
	history = append([]agentbase.Message{summary}, recent...)
	remaining := len(history)
	historyMu.Unlock()
	logger.Warn("互助论坛", "[拯救] 论坛上下文已摘要重建：%d 条历史 → 摘要 + 最近 %d 条", len(older), len(recent))
	return fmt.Sprintf("已把较早的 %d 条论坛历史压缩为摘要，保留最近 %d 条（当前 %d 条）", len(older), len(recent), remaining), nil
}

// buildForumDigest 调模型生成摘要；失败时不改动原上下文。
func buildForumDigest(older []agentbase.Message, targetRunes int) (string, error) {
	transcript := flattenMessages(older, 12000)
	if transcript == "" {
		return "", fmt.Errorf("较早历史没有可压缩的正文")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	reply, err := agentbase.Stream(ctx, agentbase.StreamRequest{
		Agent:  "mutual_forum",
		Role:   "互助论坛.摘要",
		Model:  config.AgentMultimodalModel,
		Silent: true,
		Messages: []agentbase.Message{
			{Role: "system", Content: summarizePrompt},
			{Role: "user", Content: transcript},
		},
		NonStream: true,
	})
	if err != nil {
		return "", fmt.Errorf("摘要模型调用失败：%w", err)
	}
	digest := strings.TrimSpace(reply.Text)
	if digest == "" {
		return "", fmt.Errorf("摘要模型返回空内容")
	}
	return agentbase.TruncateRunes(digest, targetRunes), nil
}

// flattenMessages 把历史拼成有界文本。
func flattenMessages(messages []agentbase.Message, limit int) string {
	var builder strings.Builder
	for _, message := range messages {
		text := strings.TrimSpace(messageText(message))
		if text == "" {
			continue
		}
		builder.WriteString(message.Role)
		builder.WriteString("：")
		builder.WriteString(text)
		builder.WriteString("\n")
		if len([]rune(builder.String())) >= limit {
			break
		}
	}
	return agentbase.TruncateRunes(strings.TrimSpace(builder.String()), limit)
}

// messageText 取消息正文；非字符串内容退化为文本形式。
func messageText(message agentbase.Message) string {
	if text, ok := message.Content.(string); ok {
		return text
	}
	return fmt.Sprint(message.Content)
}
