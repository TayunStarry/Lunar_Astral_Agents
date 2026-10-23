package lunar

import (
	"fmt"
	"strings"
	"time"
)

// DumpAgentContext 导出月华主上下文或子智能体上下文
func DumpAgentContext(agent string, includeImages bool) (string, error) {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		agent = "lunar"
	}
	if !IsReady() {
		return "", fmt.Errorf("月华智能体尚未就绪，暂时无法导出上下文")
	}
	payload := dumpPayload(agent, includeImages)
	if payload == nil {
		return "", fmt.Errorf("未知导出目标: %s", agent)
	}
	return safeJSON(payload, "{}"), nil
}

func dumpPayload(agent string, includeImages bool) map[string]any {
	overview := map[string]any{}
	unreadMu.Lock()
	overview["lunar"] = len(unreadContext)
	unread := append([]PostMessage(nil), unreadContext...)
	unreadMu.Unlock()

	var (
		target   string
		label    string
		messages []PostMessage
		prompt   string
	)
	switch agent {
	case "lunar":
		target = "lunar"
		label = "月华主上下文（未读队列 + 长期记忆摘要）"
		messages = unread
	case "dialogue":
		if dialogueRole == nil {
			return nil
		}
		target, label = "dialogue", "对话者"
		messages = dialogueRole.Messages
		prompt = dialogueRole.SystemPrompt
	case "painter":
		if painterRole == nil {
			return nil
		}
		target, label = "painter", "绘制者"
		messages = painterRole.Messages
		prompt = painterRole.SystemPrompt
	case "musician":
		if musicianRole == nil {
			return nil
		}
		target, label = "musician", "演奏者"
		messages = musicianRole.Messages
		prompt = musicianRole.SystemPrompt
	case "screenshoter":
		if screenshoterRole == nil {
			return nil
		}
		target, label = "screenshoter", "截图者"
		messages = screenshoterRole.Messages
		prompt = screenshoterRole.SystemPrompt
	case "memorizer":
		target, label = "memorizer", "记忆者"
		messages = nil
		prompt = "记忆者不维护对话上下文：只负责写入长期记忆与按需生成检索摘要"
	default:
		return nil
	}
	for name, entry := range roleEntries {
		overview[name] = len(entry.Messages)
	}
	return map[string]any{
		"kind":             "lunar-agent-context",
		"generated_at":     time.Now().Format(time.RFC3339),
		"target":           target,
		"label":            label,
		"include_images":   includeImages,
		"message_count":    len(messages),
		"context_overview": overview,
		"system_prompt":    prompt,
		"messages":         dumpMessages(messages, includeImages),
	}
}

func dumpMessages(messages []PostMessage, includeImages bool) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		out = append(out, map[string]any{
			"role":         message.Role,
			"content":      shrinkDumpContent(message.Content, includeImages),
			"tool_call_id": message.ToolCallID,
			"tool_calls":   message.ToolCalls,
		})
	}
	return out
}

func shrinkDumpContent(content any, includeImages bool) any {
	items := asItems(content)
	if len(items) == 0 {
		return content
	}
	out := make([]ContentItem, 0, len(items))
	for _, item := range items {
		if item.Type != ContentImageURL || includeImages || item.ImageURL == nil {
			out = append(out, item)
			continue
		}
		url := item.ImageURL.URL
		head := url
		if len(head) > 64 {
			head = head[:64]
		}
		out = append(out, ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: head + "…（base64 已省略）"}})
	}
	return out
}

var roleEntries map[string]*ModelBuilder

func initRoleEntries() {
	roleEntries = map[string]*ModelBuilder{}
	if dialogueRole != nil {
		roleEntries["dialogue"] = dialogueRole.ModelBuilder
	}
	if painterRole != nil {
		roleEntries["painter"] = painterRole.ModelBuilder
	}
	if musicianRole != nil {
		roleEntries["musician"] = musicianRole.ModelBuilder
	}
	if screenshoterRole != nil {
		roleEntries["screenshoter"] = screenshoterRole.ModelBuilder
	}
}
