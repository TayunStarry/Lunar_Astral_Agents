package life

// memory_batch.go 记忆整理的分批与批次入口：模型调用与输出解析由 agents/LifeMemorizer 承担。

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/agents/LifeMemorizer"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/memory"
)

// listAllTextMemories 枚举全部文本记忆，排除哔哩观影记忆。
func listAllTextMemories() []memoryItem {
	const pageSize = 200
	out := make([]memoryItem, 0)
	for offset := 0; ; offset += pageSize {
		entries, total := memory.List("text", offset, pageSize)
		for _, entry := range entries {
			if entry.Source == "bilibili_viewer" {
				continue
			}
			content := strings.TrimSpace(entry.Content)
			if content == "" || strings.HasPrefix(content, "[B站视频]") {
				continue
			}
			out = append(out, memoryItem{
				ID:        entry.ID,
				Content:   content,
				Role:      entry.Role,
				Timestamp: entry.Timestamp,
			})
		}
		if len(entries) == 0 || offset+pageSize >= total {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out
}

// buildMemoryBatches 按 1 万字符分批，单条记忆不跨批、不截断。
func buildMemoryBatches(items []memoryItem) []memoryBatch {
	batches := make([]memoryBatch, 0, len(items)/10+1)
	current := memoryBatch{}
	runes := 0
	for _, item := range items {
		n := utf8.RuneCountInString(item.Content)
		if runes > 0 && runes+n > batchMaxRunes {
			batches = append(batches, current)
			current = memoryBatch{}
			runes = 0
		}
		current.Items = append(current.Items, item)
		runes += n
		if n > batchMaxRunes {
			logger.Warn(Module, "[记忆整理] 单条记忆 %s 超过 %d 字符，独占一批且不截断", item.ID, batchMaxRunes)
			batches = append(batches, current)
			current = memoryBatch{}
			runes = 0
		}
	}
	if len(current.Items) > 0 {
		batches = append(batches, current)
	}
	return batches
}

// consolidateBatch 调用生活记忆整理者处理一个批次。
func consolidateBatch(ctx context.Context, batch memoryBatch) ([]string, error) {
	if len(batch.Items) == 0 {
		return nil, nil
	}
	contents := make([]string, 0, len(batch.Items))
	for _, item := range batch.Items {
		contents = append(contents, item.Content)
	}
	return LifeMemorizer.Consolidate(ctx, contents)
}
