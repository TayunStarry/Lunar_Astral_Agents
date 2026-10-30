package lunar

// agent_repair.go 把月华核心人格的上下文交给拯救者修复。
//
// 拯救者只通过 engine/agentrepair 注册表访问具体智能体：这里注册 ID 为 core 的
// 处理器，提供读取、裁剪、剔除图片、清空四类能力。修复直接作用在角色实例的
// 内存上下文与 messages.json 副本上，下一次 Tick 不会再带着坏上下文启动。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/agentrepair"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

func init() {
	agentrepair.Register(agentrepair.Handler{
		ID:          "core",
		Label:       "核心人格",
		Description: "对话者 / 绘制者 / 演奏者 / 截图者与月华主上下文",
		Actions:     []string{"trim", "drop_images", "clear"},
		Inspect:     inspectCoreContext,
		Repair:      repairCoreContext,
	})
}

// inspectCoreContext 读取核心人格上下文；limit<=0 表示全部。
func inspectCoreContext(limit int) (any, error) {
	if !IsReady() {
		return nil, fmt.Errorf("月华智能体尚未就绪，暂时无法读取上下文")
	}
	unreadMu.Lock()
	unread := append([]PostMessage(nil), unreadContext...)
	unreadMu.Unlock()
	out := map[string]any{
		"lunar": map[string]any{
			"label":         "月华主上下文（未读队列）",
			"message_count": len(unread),
			"messages":      dumpMessages(tailMessages(unread, limit), false),
		},
	}
	for _, role := range coreRoleSnapshots() {
		out[role.ID] = map[string]any{
			"label":         role.Label,
			"message_count": len(role.Builder.Messages),
			"messages":      dumpMessages(tailMessages(role.Builder.Messages, limit), false),
		}
	}
	return out, nil
}

// repairCoreContext 执行一次修复动作。
func repairCoreContext(action string, params map[string]any) (string, error) {
	switch action {
	case "trim":
		return trimCoreContext(keepLastFrom(params))
	case "drop_images":
		return dropCoreImages()
	case "clear":
		return clearCoreContext()
	default:
		return "", fmt.Errorf("核心人格不支持动作：%s", action)
	}
}

// trimCoreContext 只保留最近 keep 条消息。
func trimCoreContext(keep int) (string, error) {
	for _, role := range coreRoleSnapshots() {
		role.Builder.Messages = tailMessages(role.Builder.Messages, keep)
	}
	cacheMu.Lock()
	cachedMessages = tailCachedMessages(cachedMessages, keep)
	cacheMu.Unlock()
	persistConversation()
	logger.Warn(lunarCoreModule, "[拯救] 核心人格上下文已裁剪为最近 %d 条", keep)
	return fmt.Sprintf("已把核心人格上下文裁剪为最近 %d 条消息", keep), nil
}

// dropCoreImages 剔除上下文里的图片内容。
func dropCoreImages() (string, error) {
	removed := 0
	for _, role := range coreRoleSnapshots() {
		removed += stripImages(role.Builder.Messages)
	}
	unreadMu.Lock()
	removed += stripImages(unreadContext)
	removed += stripImages(unreadRecords)
	unreadMu.Unlock()
	cacheMu.Lock()
	removed += stripCachedImages()
	cacheMu.Unlock()
	persistConversation()
	logger.Warn(lunarCoreModule, "[拯救] 已从核心人格上下文剔除 %d 处图片", removed)
	return fmt.Sprintf("已从核心人格上下文剔除 %d 处图片，后续请求不再携带无法解析的图片", removed), nil
}

// clearCoreContext 清空上下文，作为最后手段。
func clearCoreContext() (string, error) {
	for _, role := range coreRoleSnapshots() {
		role.Builder.Messages = nil
		role.Builder.RuntimeMessages = nil
	}
	unreadMu.Lock()
	unreadContext = nil
	unreadRecords = nil
	unreadMu.Unlock()
	cacheMu.Lock()
	cachedMessages = nil
	cacheMu.Unlock()
	persistConversation()
	logger.Warn(lunarCoreModule, "[拯救] 核心人格上下文已整体清空")
	return "已清空核心人格上下文（可能丢失近期对话记忆）", nil
}

// coreRoleSnapshots 返回当前已实例化的核心人格角色。
func coreRoleSnapshots() []coreRoleSnapshot {
	roles := make([]coreRoleSnapshot, 0, 4)
	if dialogueRole != nil && dialogueRole.ModelBuilder != nil {
		roles = append(roles, coreRoleSnapshot{ID: "dialogue", Label: "对话者", Builder: dialogueRole.ModelBuilder})
	}
	if painterRole != nil && painterRole.ModelBuilder != nil {
		roles = append(roles, coreRoleSnapshot{ID: "painter", Label: "绘制者", Builder: painterRole.ModelBuilder})
	}
	if musicianRole != nil && musicianRole.ModelBuilder != nil {
		roles = append(roles, coreRoleSnapshot{ID: "musician", Label: "演奏者", Builder: musicianRole.ModelBuilder})
	}
	if screenshoterRole != nil && screenshoterRole.ModelBuilder != nil {
		roles = append(roles, coreRoleSnapshot{ID: "screenshoter", Label: "截图者", Builder: screenshoterRole.ModelBuilder})
	}
	return roles
}

// keepLastFrom 读取 keep_last，缺省 8。
func keepLastFrom(params map[string]any) int {
	if value, ok := params["keep_last"].(int); ok && value > 0 {
		return value
	}
	return 8
}

// stripImages 把消息里的图片内容项替换为文本占位，返回被替换的处数。
func stripImages(messages []PostMessage) int {
	removed := 0
	for index := range messages {
		items := asItems(messages[index].Content)
		if len(items) == 0 {
			continue
		}
		changed := false
		kept := make([]ContentItem, 0, len(items))
		for _, item := range items {
			if item.Type == ContentImageURL {
				kept = append(kept, ContentItem{Type: ContentText, Text: "[图片已移除]"})
				removed++
				changed = true
				continue
			}
			kept = append(kept, item)
		}
		if changed {
			messages[index].Content = kept
		}
	}
	return removed
}

// stripCachedImages 剔除缓存消息里的图片附件与图片地址，调用方需持有 cacheMu。
func stripCachedImages() int {
	removed := 0
	for index := range cachedMessages {
		record := &cachedMessages[index]
		if record.ImageSrc != "" {
			record.ImageSrc = ""
			record.ImageLabel = ""
			removed++
		}
		if len(record.Attachments) == 0 {
			continue
		}
		kept := make([]CachedAttachment, 0, len(record.Attachments))
		for _, attachment := range record.Attachments {
			if strings.EqualFold(attachment.Type, "image") {
				removed++
				continue
			}
			kept = append(kept, attachment)
		}
		record.Attachments = kept
	}
	return removed
}

// tailMessages 取最后 limit 条；limit<=0 返回原切片。
func tailMessages(messages []PostMessage, limit int) []PostMessage {
	if limit <= 0 || len(messages) <= limit {
		return messages
	}
	return messages[len(messages)-limit:]
}

// tailCachedMessages 取最后 limit 条缓存消息。
func tailCachedMessages(messages []CachedMessage, limit int) []CachedMessage {
	if limit <= 0 || len(messages) <= limit {
		return messages
	}
	return messages[len(messages)-limit:]
}
