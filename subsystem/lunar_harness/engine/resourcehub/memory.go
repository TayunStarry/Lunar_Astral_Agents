package resourcehub

// memory.go memory 模式：只检索长期记忆库，不混入资源目录或句柄查询。
//
// 语义与对话者原有实现保持一致：向量检索优先，数量不足时用关键词检索补足；
// 命中结果按相关度排序，并标注角色 / 时间 / 相关度，便于模型判断可信度。

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/memory"
)

// runMemory 检索长期记忆。
func runMemory(ctx context.Context, q Query) Result {
	if len(q.Query) == 0 {
		return failure("query 需要 1-4 个搜索关键词")
	}
	topK := q.Limit
	if topK <= 0 {
		topK = defaultLimit
	}
	if topK > memoryMaxResults {
		topK = memoryMaxResults
	}
	if !ensureMemoryReady() {
		return failure("记忆库未就绪，无法检索。请稍后再试一次。")
	}
	hits := searchMemory(ctx, q.Query, topK)
	if len(hits) == 0 {
		return Result{
			Text:   "记忆库中没有相关记录，可以换一种表述再试一次；",
			Handle: []string{},
		}
	}
	lines := make([]string, 0, len(hits))
	for index, hit := range hits {
		meta := fmt.Sprintf("角色=%s｜时间=%s｜相关度=%.3f", roleLabel(hit.Role), formatMemoryTime(hit.Timestamp), hit.Similarity)
		lines = append(lines, fmt.Sprintf("[%d] %s\n%s", index+1, meta, previewRunes(hit.Content, previewRunesLimit)))
	}
	text := fmt.Sprintf("检索到 %d 条相关记忆：\n%s", len(hits), strings.Join(lines, "\n\n"))
	text += "\n\n只依据以上原文回答；原文不足以回答用户问题时，直接说明不记得/没有找到相关信息，不要编造，也不要改用 handle 模式现场读取。"
	return Result{Text: text, Handle: []string{}}
}

// ensureMemoryReady 确保记忆库已初始化
func ensureMemoryReady() bool {
	memoryMu.Lock()
	ready := memoryReady
	memoryMu.Unlock()
	if ready {
		return true
	}
	memory.Init(config.MemoryDBDir)
	if err := memory.EnsureReady(); err != nil {
		return false
	}
	memoryMu.Lock()
	memoryReady = true
	memoryMu.Unlock()
	return true
}

// searchMemory 向量检索 + 关键词补足，按相关度去重排序
func searchMemory(ctx context.Context, keywords []string, topK int) []memory.Entry {
	merged := map[string]memory.Entry{}
	order := make([]string, 0)
	for _, keyword := range keywords {
		results, err := memory.Query(ctx, keyword, "text", topK, memory.QueryOptions{})
		if err != nil {
			continue
		}
		for _, item := range results {
			content := strings.TrimSpace(item.Content)
			if content == "" || item.ID == "" {
				continue
			}
			item.Content = content
			if existing, ok := merged[item.ID]; !ok {
				merged[item.ID] = item
				order = append(order, item.ID)
			} else if item.Similarity > existing.Similarity {
				merged[item.ID] = item
			}
		}
	}
	hits := make([]memory.Entry, 0, len(order))
	for _, id := range order {
		hits = append(hits, merged[id])
	}
	sort.SliceStable(hits, func(i, j int) bool {
		return hits[i].Similarity > hits[j].Similarity
	})
	if len(hits) > topK {
		hits = hits[:topK]
	}
	if len(hits) >= topK {
		return hits
	}
	// 关键词补足：向量检索不足时，用原文关键词再找一轮
	known := make(map[string]bool, len(hits))
	for _, hit := range hits {
		known[hit.ID] = true
	}
	fallback := keywordFallback(keywords, topK, known)
	hits = append(hits, fallback...)
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits
}

// keywordFallback 关键词检索补足（去重向量命中，按命中次数与时间排序）
func keywordFallback(keywords []string, topK int, known map[string]bool) []memory.Entry {
	terms := keywordTerms(keywords)
	if len(terms) == 0 {
		return nil
	}
	type entry struct {
		hit      memory.Entry
		hitCount int
	}
	merged := map[string]entry{}
	order := make([]string, 0)
	for _, term := range terms {
		results, err := memory.KeywordSearch("text", []string{term}, false, topK*2)
		if err != nil {
			continue
		}
		for _, item := range results {
			content := strings.TrimSpace(item.Content)
			if content == "" || item.ID == "" || known[item.ID] {
				continue
			}
			item.Content = content
			if existing, ok := merged[item.ID]; ok {
				existing.hitCount++
				merged[item.ID] = existing
			} else {
				merged[item.ID] = entry{hit: item, hitCount: 1}
				order = append(order, item.ID)
			}
		}
	}
	hits := make([]memory.Entry, 0, len(order))
	for _, id := range order {
		hits = append(hits, merged[id].hit)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		left, right := merged[hits[i].ID], merged[hits[j].ID]
		if right.hitCount != left.hitCount {
			return right.hitCount > left.hitCount
		}
		return right.hit.Timestamp > left.hit.Timestamp
	})
	return hits
}

// keywordTerms 把查询拆成适合原文检索的短词
func keywordTerms(keywords []string) []string {
	terms := make([]string, 0)
	seen := map[string]bool{}
	for _, keyword := range keywords {
		text := strings.TrimSpace(keyword)
		if text == "" {
			continue
		}
		if len([]rune(text)) <= 20 {
			if !seen[text] {
				seen[text] = true
				terms = append(terms, text)
			}
			continue
		}
		for _, part := range strings.Fields(text) {
			word := strings.TrimSpace(part)
			if len([]rune(word)) >= 2 && !seen[word] {
				seen[word] = true
				terms = append(terms, word)
			}
		}
	}
	if len(terms) > 8 {
		terms = terms[:8]
	}
	return terms
}

// roleLabel 记忆角色中文名
func roleLabel(role string) string {
	switch role {
	case "user":
		return "用户"
	case "assistant":
		return "月华"
	case "system":
		return "系统"
	case "tool":
		return "工具"
	case "image":
		return "图片"
	}
	if role == "" {
		return "未知"
	}
	return role
}

// formatMemoryTime 记忆时间格式化
func formatMemoryTime(timestamp int64) string {
	if timestamp <= 0 {
		return "时间未知"
	}
	return time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")
}
