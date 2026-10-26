package lunar

// data_dump.go 月华核心人格的上下文导出。
//
// 新的导出口径把对话者 / 绘制者 / 演奏者 / 截图者 / 记忆者与主上下文合并为
// 一份「核心人格」快照，不再为每个角色单独开一个导出分支：排查时真正需要的是
// 它们共享的上下文全貌，而不是逐份下载再人工拼接。

import (
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "core",
		Label:       "核心人格",
		Group:       "Go 智能体",
		Description: "对话者 + 绘制者 + 演奏者 + 截图者 + 月华主上下文",
		Export: func(includeImages bool) (any, error) {
			return dumpCoreContext(includeImages)
		},
	})
}

// DumpAgentContext 保留旧入口：核心人格统一导出；旧的分角色目标不再单独提供。
func DumpAgentContext(agent string, includeImages bool) (string, error) {
	agent = strings.TrimSpace(agent)
	if agent == "" || agent == "lunar" || agent == "core" {
		return agentdump.Export("core", includeImages)
	}
	return "", fmt.Errorf("分角色导出已合并为「核心人格」：请使用 agent=core（原目标 %s 已移除）", agent)
}

// dumpCoreContext 组装核心人格全部角色的上下文。
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
		addRole("dialogue", "对话者", dialogueRole.ModelBuilder)
	}
	if painterRole != nil {
		addRole("painter", "绘制者", painterRole.ModelBuilder)
	}
	if musicianRole != nil {
		addRole("musician", "演奏者", musicianRole.ModelBuilder)
	}
	if screenshoterRole != nil {
		addRole("screenshoter", "截图者", screenshoterRole.ModelBuilder)
	}
	sections["memory"] = map[string]any{
		"label":         "记忆入库与检索",
		"message_count": 0,
		"note":          "记忆者不是独立智能体：它只负责写入长期记忆与生成检索摘要，能力通过对话者的 search_memory 工具暴露",
	}

	return map[string]any{
		"kind":             "lunar-core-persona-context",
		"generated_at":     time.Now().Format(time.RFC3339),
		"label":            "月华核心人格（对话者 + 绘制者 + 演奏者 + 截图者 + 主上下文）",
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
		if len(head) > 64 {
			head = head[:64]
		}
		out = append(out, ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: head + "…（base64 已省略）"}})
	}
	return out
}
