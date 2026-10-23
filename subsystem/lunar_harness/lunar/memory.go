package lunar

import (
	"context"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
)

// ensureMemoryReady 确保记忆库已初始化
func ensureMemoryReady() bool {
	stateMu.Lock()
	ready := memoryReady
	stateMu.Unlock()
	if ready {
		return true
	}
	persistence.InitMemoryDB(config.MemoryDBDir)
	if err := persistence.MemoryInitInstance(); err != nil {
		logger.Error(lunarCoreModule, "记忆库初始化失败: %v", err)
		return false
	}
	stateMu.Lock()
	memoryReady = true
	stateMu.Unlock()
	return true
}

// memoryResults 把持久层查询结果转换为检索命中
func memoryResults(results []persistence.MemoryQueryResult) []MemorySearchHit {
	hits := make([]MemorySearchHit, 0, len(results))
	for _, item := range results {
		hits = append(hits, MemorySearchHit{
			ID:         item.ID,
			Type:       item.Type,
			Role:       item.Role,
			Content:    item.Content,
			Similarity: float64(item.Similarity),
			Timestamp:  item.Timestamp,
			Collection: item.Collection,
			Source:     item.Source,
			URL:        item.URL,
			Base64:     item.Base64,
		})
	}
	return hits
}

// normalizeTerms 规整过滤词
func normalizeTerms(terms []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		value := strings.ToLower(strings.TrimSpace(term))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func containsAll(content string, terms []string) bool {
	lower := strings.ToLower(content)
	for _, term := range terms {
		if !strings.Contains(lower, term) {
			return false
		}
	}
	return true
}

func containsAny(content string, terms []string) bool {
	lower := strings.ToLower(content)
	for _, term := range terms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

// searchMemoryGlobal 跨集合检索文本记忆
func searchMemoryGlobal(query string, limit int, options MemorySearchOptions) []MemorySearchHit {
	text := strings.TrimSpace(query)
	if text == "" {
		return nil
	}
	if limit <= 0 {
		limit = 8
	}
	if limit > 30 {
		limit = 30
	}
	mode := options.Mode
	if mode == "" {
		mode = "vector"
	}
	terms := strings.Fields(text)
	if mode == "regex" {
		terms = []string{text}
	}
	if len(terms) == 0 {
		return nil
	}
	var raw []persistence.MemoryQueryResult
	var err error
	if mode == "vector" {
		raw, err = persistence.MemoryQueryWithOptions(context.Background(), text, "text", limit, persistence.MemoryQueryOptions{ForceGlobal: options.ForceGlobal})
	} else {
		raw, err = persistence.MemoryKeywordSearch("text", terms, mode == "regex", limit)
	}
	if err != nil {
		logger.Warn(lunarCoreModule, "[记忆检索] 查询失败: %v", err)
		return nil
	}
	filters := normalizeTerms(options.Filters)
	exclude := normalizeTerms(options.Exclude)
	hits := make([]MemorySearchHit, 0, len(raw))
	seen := map[string]bool{}
	for _, hit := range memoryResults(raw) {
		content := strings.TrimSpace(hit.Content)
		if content == "" || seen[content] {
			continue
		}
		if !containsAll(content, filters) || containsAny(content, exclude) {
			continue
		}
		seen[content] = true
		hit.Content = content
		hits = append(hits, hit)
		if len(hits) >= limit {
			break
		}
	}
	return hits
}

// queryEmotionSticker 从图像记忆检索表情包
func queryEmotionSticker(query string) string {
	query = strings.TrimSpace(query)
	if query == "" || !ensureMemoryReady() {
		return ""
	}
	results, err := persistence.MemoryQueryWithOptions(context.Background(), query, "image", 10, persistence.MemoryQueryOptions{ForceGlobal: true})
	if err != nil || len(results) == 0 {
		return ""
	}
	item := results[randomFloor(0, len(results)-1)]
	if item.URL != "" {
		return item.URL
	}
	if item.Base64 != "" {
		return saveBase64Media(item.Base64, "image/jpeg", "sticker")
	}
	return ""
}

// getMemoryEntry 按 ID 读取记忆
func getMemoryEntry(id string) *MemorySearchHit {
	item, err := persistence.MemoryGetEntry(strings.TrimSpace(id))
	if err != nil || item == nil {
		return nil
	}
	hits := memoryResults([]persistence.MemoryQueryResult{*item})
	if len(hits) == 0 {
		return nil
	}
	return &hits[0]
}

// listMemoryEntries 逐条列举记忆
func listMemoryEntries(mediaType string, limit, offset int) []MemorySearchHit {
	entries, _ := persistence.MemoryListEntries(mediaType, offset, limit)
	return memoryResults(entries)
}

// addMemory 写入一条文本记忆
func addMemory(role, content string) (string, error) {
	return persistence.MemoryAddMessage(context.Background(), role, content)
}
