package life

// memory_batch.go 记忆整理的分批与批次入口：模型调用与输出解析由 agents/LifeMemorizer 承担。

import (
	"context"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/memory"
)

// listAllTextEntries 枚举全部文本记忆（含观影来源），供生活整理使用。
func listAllTextEntries() []memory.Entry {
	const pageSize = 200
	out := make([]memory.Entry, 0)
	for offset := 0; ; offset += pageSize {
		entries, total := memory.List("text", offset, pageSize)
		out = append(out, entries...)
		if len(entries) == 0 || offset+pageSize >= total {
			break
		}
	}
	return out
}

// listAllTextMemories 枚举参与生活整理的文本记忆。
func listAllTextMemories() []memoryItem {
	return filterLifeMemories(listAllTextEntries())
}

// filterLifeMemories 只按 Source 排除观影记忆，正文前缀原样保留。
// 观影链的旧记忆迁移由其自身负责，本链路不再做正文前缀清洗。
func filterLifeMemories(entries []memory.Entry) []memoryItem {
	out := make([]memoryItem, 0, len(entries))
	for _, entry := range entries {
		if isViewerSource(entry.Source) {
			continue
		}
		content := strings.TrimSpace(entry.Content)
		if content == "" {
			continue
		}
		out = append(out, memoryItem{
			ID:             entry.ID,
			Content:        content,
			Role:           entry.Role,
			Timestamp:      entry.Timestamp,
			Pending:        entry.Pending,
			PendingPartner: entry.PendingPartner,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out
}

// isViewerSource 观影来源判断只认 Source 字段，不再看正文前缀。
func isViewerSource(source string) bool {
	return strings.TrimSpace(source) == viewerMemorySource
}

// mergeBatch 调用生活记忆整理者对相似簇做合并改写（可输出一条或多条事实条目）。
func mergeBatch(ctx context.Context, batch memoryBatch) ([]string, error) {
	if len(batch.Items) == 0 {
		return nil, nil
	}
	return mergeHook(ctx, batchContents(batch))
}

// batchContents 取批次内各条目正文。
func batchContents(batch memoryBatch) []string {
	contents := make([]string, 0, len(batch.Items))
	for _, item := range batch.Items {
		contents = append(contents, item.Content)
	}
	return contents
}

// sameTextSet 判断整理结果是否与旧条目完全一致（仅顺序/空白差异）。
// 一致说明没有可合并内容，调用方应跳过替换，保留原条目 ID。
func sameTextSet(oldTexts, newTexts []string) bool {
	oldSet := normalizedTextSet(oldTexts)
	newSet := normalizedTextSet(newTexts)
	if len(oldSet) != len(newSet) {
		return false
	}
	for text, count := range oldSet {
		if newSet[text] != count {
			return false
		}
	}
	return true
}

func normalizedTextSet(texts []string) map[string]int {
	out := make(map[string]int, len(texts))
	for _, text := range texts {
		normalized := strings.Join(strings.Fields(text), " ")
		if normalized == "" {
			continue
		}
		out[normalized]++
	}
	return out
}
