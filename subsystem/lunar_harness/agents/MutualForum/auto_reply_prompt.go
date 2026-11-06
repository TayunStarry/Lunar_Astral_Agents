package MutualForum

// auto_reply_prompt.go 自动回应的提示词与模型调用。
//
// 四层约束：模型调用必须发生在智能体内部，因此生成逻辑留在本包，
// 执行体/链路只调用 AutoRespondUnread，不直接接触模型。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// autoReplySystemPrompt 自动回应的角色与输出约束。
const autoReplySystemPrompt = `你是月华，一个在「月华互助论坛」里活动的开发者。
现在你要回复论坛里别人给你的回应。请只根据下面给出的原始语境写一条可以直接发布的回复。
要求：
1. 先接住对方最具体的点（问题、观点或补充），再给出你的判断、经验、追问或可执行的下一步。
2. 禁止空话，例如只写「感谢回复」「谢谢分享」「说得很好」。
3. 中文 Markdown，语气自然克制，30-300 字，不要标题、不要代码围栏、不要「回复：」前缀。
4. 不要暴露自己是程序，不要编造语境里没有的话题、编号或事实。
只输出回复正文。`

// generateAutoReply 调用模型生成一条回复正文。
func generateAutoReply(ctx context.Context, rc replyContext) (string, error) {
	reply, err := agentbase.Stream(ctx, agentbase.StreamRequest{
		Agent:     "mutual_forum",
		Role:      roleLabel,
		Messages:  autoReplyMessages(rc),
		NonStream: true,
		Silent:    true,
		Timeout:   modelTimeout,
	})
	if err != nil {
		return "", err
	}
	text := cleanReplyText(reply.Text)
	if text == "" {
		return "", errors.New("模型没有生成可用的回复正文")
	}
	if count := utf8.RuneCountInString(text); count > commentBodyMaxRunes {
		return "", fmt.Errorf("模型生成的回复过长：%d 字，上限 %d 字", count, commentBodyMaxRunes)
	}
	return text, nil
}

// autoReplyMessages 组装 system + user 两条消息。
func autoReplyMessages(rc replyContext) []agentbase.Message {
	return []agentbase.Message{
		{Role: "system", Content: autoReplySystemPrompt},
		{Role: "user", Content: buildAutoReplyUser(rc)},
	}
}

// buildAutoReplyUser 把原始语境整理成模型可读的任务描述。
func buildAutoReplyUser(rc replyContext) string {
	var builder strings.Builder
	builder.WriteString("【事件类型】")
	builder.WriteString(rc.EventType)
	if rc.ActorName != "" {
		builder.WriteString("\n【对方】")
		builder.WriteString(rc.ActorName)
	}
	builder.WriteString("\n【话题标题】")
	builder.WriteString(rc.TopicTitle)
	builder.WriteString("\n【话题正文】")
	builder.WriteString(previewText(rc.TopicBody, maxBodyPreview))
	if rc.PostBody != "" {
		builder.WriteString("\n【我被回应的发言】")
		builder.WriteString(previewText(rc.PostBody, maxPostPreview))
	}
	if rc.CommentBody != "" {
		builder.WriteString("\n【对方给我的回应】")
		builder.WriteString(previewText(rc.CommentBody, maxPostPreview))
	}
	builder.WriteString("\n\n请直接写出回复正文。")
	return builder.String()
}

// cleanReplyText 去掉模型可能附带的代码围栏与首尾空白。
func cleanReplyText(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) >= 2 {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
