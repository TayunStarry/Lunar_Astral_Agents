package thinking

// data_dump.go 对话者的上下文导出。
//
// 对话者与月华主上下文合并为一份快照；绘制者与截图者已是独立扁平智能体，
// 不再并入对话者的导出内容。

import (
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "dialogue",
		Label:       Dialogue.Role,
		Group:       "Go 智能体",
		Description: Dialogue.Role + " + 月华主上下文",
		Export: func(includeImages bool) (any, error) {
			return dumpCoreContext(includeImages)
		},
	})
}

// DumpAgentContext 保留旧入口：对话者统一导出；旧目标 core / lunar 作为兼容别名。
func DumpAgentContext(agent string, includeImages bool) (string, error) {
	agent = strings.TrimSpace(agent)
	if agent == "" || agent == "lunar" || agent == "core" || agent == "dialogue" {
		return agentdump.Export("dialogue", includeImages)
	}
	return "", fmt.Errorf("分角色导出已合并为「%s」：请使用 agent=dialogue（原目标 %s 已移除）", Dialogue.Role, agent)
}

// dumpCoreContext 组装对话者与主上下文。
func dumpCoreContext(includeImages bool) (any, error) {
	if !IsReady() {
		return nil, fmt.Errorf("月华智能体尚未就绪，暂时无法导出上下文")
	}

	unreadMu.Lock()
	unread := append([]PostMessage(nil), unreadContext...)
	unreadMu.Unlock()

	sections := map[string]any{}
	overview := map[string]int{}
	total := 0

	sections["lunar"] = map[string]any{
		"label":         "月华主上下文（未读队列）",
		"message_count": len(unread),
		"messages":      dumpMessages(unread, includeImages),
	}
	overview["lunar"] = len(unread)
	total += len(unread)

	addRole := func(id, label string, builder *ModelBuilder) {
		if builder == nil {
			return
		}
		messages := builder.Messages
		sections[id] = map[string]any{
			"label":         label,
			"message_count": len(messages),
			"system_prompt": builder.SystemPrompt,
			"messages":      dumpMessages(messages, includeImages),
		}
		overview[id] = len(messages)
		total += len(messages)
	}
	if dialogueRole != nil {
		addRole("dialogue", Dialogue.Role, dialogueRole.ModelBuilder)
	}
	sections["memory"] = map[string]any{
		"label":         "记忆入库与检索",
		"message_count": 0,
		"note":          "记忆者不是独立智能体：它只负责写入长期记忆与生成检索摘要，能力通过" + Dialogue.Role + "的 fetch_resources 工具暴露",
	}

	return map[string]any{
		"kind":             "lunar-dialogue-context",
		"generated_at":     time.Now().Format(time.RFC3339),
		"label":            Dialogue.Role + " + 月华主上下文",
		"include_images":   includeImages,
		"message_count":    total,
		"context_overview": overview,
		"sections":         sections,
	}, nil
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
		if len(head) > 512 {
			head = head[:512]
		}
		out = append(out, ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: head + "…（base64 已省略）"}})
	}
	return out
}
