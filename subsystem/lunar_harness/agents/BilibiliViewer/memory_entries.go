package BilibiliViewer

// memory_entries.go 统一记忆库适配：按统一格式枚举与增删改查。
//
// 这里没有任何独立存储：枚举/读取/更新/删除全部调用 engine/memory，
// 本文件只负责 [B站视频][印象/评价/习惯/问题/吐槽/...]<正文> 的格式化与解析。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/memory"
)

// MemoryEntries 逐条枚举统一记忆库中全部 [B站视频] 条目（按入库时间正序）。
func MemoryEntries() []memoryEntry {
	items := memory.ListBySource("text", memorySource, 0)
	out := make([]memoryEntry, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- { // 接口按时间倒序，这里翻正便于阅读
		entry, ok := parseMemoryContent(items[i].ID, items[i].Content)
		if !ok {
			continue
		}
		entry.Time = items[i].Timestamp
		out = append(out, entry)
	}
	return out
}

// MemoryGetEntry 按 ID 查看一条哔哩观影记忆。
func MemoryGetEntry(id string) (memoryEntry, error) {
	item, err := memory.Get(strings.TrimSpace(id))
	if err != nil {
		return memoryEntry{}, err
	}
	if item == nil || item.Source != memorySource {
		return memoryEntry{}, fmt.Errorf("不是哔哩观影记忆: %s", id)
	}
	entry, ok := parseMemoryContent(item.ID, item.Content)
	if !ok {
		return memoryEntry{}, fmt.Errorf("不是哔哩观影记忆: %s", id)
	}
	entry.Time = item.Timestamp
	return entry, nil
}

// MemoryAddEntry 新增一条哔哩观影记忆，返回记忆库 ID。
//
// 正文按统一格式写入：[B站视频][类别]<正文>。
func MemoryAddEntry(kind, text string) (string, error) {
	content, err := formatMemoryContent(kind, text)
	if err != nil {
		return "", err
	}
	id, err := memory.AddTextWithTags(context.Background(), "assistant", content, nil, memorySource)
	if err != nil {
		return "", err
	}
	logger.Info("BilibiliViewer", "已写入观影记忆 %s", truncate(content, 60))
	return id, nil
}

// MemoryUpdateEntry 改写一条哔哩观影记忆（ID 保持不变）。
func MemoryUpdateEntry(id, kind, text string) (string, error) {
	id = strings.TrimSpace(id)
	if _, err := MemoryGetEntry(id); err != nil {
		return "", err
	}
	content, err := formatMemoryContent(kind, text)
	if err != nil {
		return "", err
	}
	if err := memory.Update(context.Background(), id, content); err != nil {
		return "", err
	}
	logger.Info("BilibiliViewer", "已改写观影记忆 %s", truncate(content, 60))
	return id, nil
}

// MemoryDeleteEntry 删除一条哔哩观影记忆。
func MemoryDeleteEntry(id string) error {
	id = strings.TrimSpace(id)
	if _, err := MemoryGetEntry(id); err != nil {
		return err
	}
	return memory.Delete(context.Background(), id)
}

// formatMemoryContent 把类别与正文拼成统一记忆格式。
func formatMemoryContent(kind, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("记忆正文为空")
	}
	return fmt.Sprintf("%s[%s]%s", memoryTagPrefix, normalizeMemoryKind(kind), text), nil
}

// parseMemoryContent 从统一格式正文里解析类别与正文。
//
// 前缀本身对本智能体没有意义（只用于让对话者识别来源），因此这里不校验前缀文字，
// 直接剥掉第一段 [...]，再解析紧随其后的 [类别]；这样前缀改名也不会丢历史条目。
// 正文不是「[前缀][类别]正文」形态时返回 false。
func parseMemoryContent(id, content string) (memoryEntry, bool) {
	text := strings.TrimSpace(content)
	if !strings.HasPrefix(text, "[") {
		return memoryEntry{}, false
	}
	prefixEnd := strings.Index(text, "]")
	if prefixEnd <= 0 {
		return memoryEntry{}, false
	}
	rest := text[prefixEnd+1:]
	kind := "印象"
	body := rest
	if strings.HasPrefix(rest, "[") {
		if end := strings.Index(rest, "]"); end > 0 {
			if k := strings.TrimSpace(rest[1:end]); k != "" {
				kind = k
			}
			body = rest[end+1:]
		}
	}
	return memoryEntry{ID: id, Kind: kind, Text: strings.TrimSpace(body)}, true
}

// normalizeMemoryKind 记忆类别收敛到常用集合，避免出现乱七八糟的分类。
func normalizeMemoryKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case "印象", "感受", "评价", "习惯", "问题", "吐槽", "人物", "线索":
		return strings.TrimSpace(kind)
	default:
		return "印象"
	}
}

// cleanupMechanicalMemories 删除历史遗留的机械化记忆（切片过程文本、运行统计等）。
//
// 只处理本智能体来源的条目；返回删除条数。
func cleanupMechanicalMemories() int {
	removed := 0
	for _, entry := range MemoryEntries() {
		if !memoryLooksMechanical(entry.Kind) && !memoryLooksMechanical(entry.Text) {
			continue
		}
		if err := MemoryDeleteEntry(entry.ID); err != nil {
			logger.Warn("BilibiliViewer", "清理机械化记忆失败(%s): %v", entry.ID, err)
			continue
		}
		removed++
	}
	return removed
}
