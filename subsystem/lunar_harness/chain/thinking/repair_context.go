package thinking

// repair_context.go 对话者上下文的按条剔除与按水位裁剪。
//
// 这两类动作都只改「模型上下文」，不动前端展示用的 messages.json 缓存：
// 剔除污染消息后，下一次请求不会再携带它，但界面历史仍然可查。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// dropCoreMessages 按文本片段剔除污染/失败消息。
func dropCoreMessages(contains string) (string, error) {
	pattern := strings.TrimSpace(contains)
	if pattern == "" {
		return "", fmt.Errorf("drop_messages 需要 contains 参数：要剔除的文本片段")
	}
	removed := 0
	for _, role := range coreRoleSnapshots() {
		kept, count := filterMessagesByText(role.Builder.Messages, pattern)
		role.Builder.Messages = kept
		removed += count
	}
	unreadMu.Lock()
	kept, count := filterMessagesByText(unreadContext, pattern)
	unreadContext = kept
	unreadMu.Unlock()
	removed += count
	persistConversation()
	logger.Warn(lunarCoreModule, "[拯救] 已按片段剔除 %d 条消息：%s", removed, pattern)
	return fmt.Sprintf("已从模型上下文剔除 %d 条包含「%s」的消息", removed, pattern), nil
}

// compactCoreContext 裁剪到目标字数水位，同时保留最近 keepLast 条。
func compactCoreContext(target, keepLast int) (string, error) {
	removed := 0
	for _, role := range coreRoleSnapshots() {
		kept, count := compactMessages(role.Builder.Messages, target, keepLast)
		role.Builder.Messages = kept
		removed += count
	}
	unreadMu.Lock()
	kept, count := compactMessages(unreadContext, target, keepLast)
	unreadContext = kept
	unreadMu.Unlock()
	removed += count
	persistConversation()
	logger.Warn(lunarCoreModule, "[拯救] 上下文已裁剪到 %d 字水位，丢弃最早的 %d 条", target, removed)
	return fmt.Sprintf("已把对话者上下文裁剪到约 %d 字，丢弃最早的 %d 条消息（最近 %d 条保留）", target, removed, keepLast), nil
}

// compactMessages 从最早的消息开始丢弃，直到正文总量不超过 target。
func compactMessages(messages []PostMessage, target, keepLast int) ([]PostMessage, int) {
	if target <= 0 || len(messages) == 0 {
		return messages, 0
	}
	if keepLast < 0 {
		keepLast = 0
	}
	total := messageRunes(messages)
	removed := 0
	for total > target && len(messages) > keepLast {
		total -= messageRunes(messages[:1])
		messages = messages[1:]
		removed++
	}
	return messages, removed
}

// messageRunes 统计一组消息的正文长度。
func messageRunes(messages []PostMessage) int {
	total := 0
	for _, message := range messages {
		total += len([]rune(messageText(message)))
	}
	return total
}

// filterMessagesByText 剔除正文包含指定片段的消息。
func filterMessagesByText(messages []PostMessage, pattern string) ([]PostMessage, int) {
	if len(messages) == 0 {
		return messages, 0
	}
	kept := make([]PostMessage, 0, len(messages))
	removed := 0
	for _, message := range messages {
		if strings.Contains(messageText(message), pattern) {
			removed++
			continue
		}
		kept = append(kept, message)
	}
	return kept, removed
}

// compactTargetFrom 读取目标字数水位（默认 6000，夹在 [1000, 20000]）。
func compactTargetFrom(params map[string]any) int {
	target := compactDefaultRunes
	switch value := params["target_runes"].(type) {
	case int:
		target = value
	case float64:
		target = int(value)
	}
	if target < compactMinRunes {
		target = compactMinRunes
	}
	if target > compactMaxRunes {
		target = compactMaxRunes
	}
	return target
}
