package thinking

import (
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// 记忆时间线硬切断长度上限
const ragTimelineHardLimit = 2048

// 每条用户消息查询记忆库时返回的相关度最高的记录数
const ragPerQueryTopK = 10

// 去重后送入时间线的检索记录条数上限
const ragMaxRecords = 24

// 单轮最多执行的检索查询数
const ragMaxQueries = 4

// 进入自动记忆的最低相似度门槛
const ragMinSimilarity = 0.40

// persistUnreadRecords 把缓冲消息逐个写入记忆库
//
// 进入这里的消息已经越过近期记忆的 48 条对话窗口（maxCacheMessages / ContextLimit），
// 属于远期记忆：只保留文本与资源句柄，图片本体、播放地址与二进制一律不入库。
func (role *MemorizerRole) persistUnreadRecords() {
	unreadMu.Lock()
	records := append([]PostMessage(nil), unreadRecords...)
	unreadMu.Unlock()
	if len(records) == 0 {
		return
	}
	if !ensureMemoryReady() {
		logger.Warn(lunarCoreModule, "[记忆] 记忆库未就绪，保留缓冲消息待下次触发")
		return
	}
	written := 0
	for _, message := range records {
		if message.Role == RoleTool {
			continue
		}
		content := strings.TrimSpace(memoryRecordText(message))
		if content == "" || len([]rune(content)) <= 5 || strings.Contains(content, scheduleTriggerPrefix) {
			continue
		}
		if _, err := addMemory(message.Role, content); err != nil {
			logger.Warn(lunarCoreModule, "[记忆] 写入记忆库失败: %v", err)
			continue
		}
		written++
	}
	unreadMu.Lock()
	unreadRecords = nil
	unreadMu.Unlock()
	logger.Info(lunarCoreModule, "[记忆] 已写入 %d 条消息到记忆库", written)
}

// memoryRecordText 远期记忆的文本形态：只保留文本与资源句柄。
//
// 图片句柄优先取清洗阶段记下的句柄，其次从视觉读取地址的 tag 参数解析；
// 两者都没有时写入明确的缺失说明，避免图片被静默丢弃。
func memoryRecordText(message PostMessage) string {
	if text, ok := message.Content.(string); ok {
		return text
	}
	items := asItems(message.Content)
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case ContentText:
			if strings.TrimSpace(item.Text) != "" {
				parts = append(parts, item.Text)
			}
		case ContentImageURL:
			handle := strings.TrimSpace(item.ResourceHandle)
			if handle == "" && item.ImageURL != nil {
				handle = handleFromVisionURL(item.ImageURL.URL)
			}
			if handle == "" {
				handle = missingImageHandleText
			}
			parts = append(parts, handle)
		}
	}
	return strings.Join(parts, " ")
}

// queryRagDigest 检索长期记忆并生成时间线上下文
func (role *MemorizerRole) queryRagDigest(userMessages []string) string {
	if len(userMessages) == 0 || !ensureMemoryReady() {
		return ""
	}
	records := role.retrieveRagRecords(userMessages)
	if len(records) == 0 {
		return ""
	}
	return role.buildTimeline(records)
}

func (role *MemorizerRole) buildTimeline(records []MemorySearchHit) string {
	sorted := append([]MemorySearchHit(nil), records...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Timestamp < sorted[j-1].Timestamp; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	timeline := ""
	for _, record := range sorted {
		timeText := "未知"
		if record.Timestamp > 0 {
			timeText = time.Unix(record.Timestamp, 0).Format("2006-01-02 15:04:05")
		}
		content := record.Content
		if content == "" {
			content = "(空)"
		}
		line := fmt.Sprintf("[ 时间: %s ] [ 内容: %s ]", timeText, content)
		if len([]rune(timeline))+len([]rune(line))+1 > ragTimelineHardLimit {
			break
		}
		if timeline != "" {
			timeline += "\n"
		}
		timeline += line
	}
	return timeline
}

func (role *MemorizerRole) retrieveRagRecords(userMessages []string) []MemorySearchHit {
	queries := make([]string, 0)
	seen := map[string]bool{}
	for _, text := range userMessages {
		query := memoryQueryText(text)
		if query == "" || seen[query] {
			continue
		}
		seen[query] = true
		queries = append(queries, query)
	}
	if len(queries) > ragMaxQueries {
		queries = queries[len(queries)-ragMaxQueries:]
	}
	all := make([]MemorySearchHit, 0)
	for _, query := range queries {
		for _, hit := range searchMemoryGlobal(query, ragPerQueryTopK, MemorySearchOptions{}) {
			if hit.Similarity < ragMinSimilarity {
				continue
			}
			all = append(all, hit)
		}
	}
	best := map[string]MemorySearchHit{}
	for _, hit := range all {
		if existing, ok := best[hit.Content]; !ok || hit.Similarity > existing.Similarity {
			best[hit.Content] = hit
		}
	}
	unique := make([]MemorySearchHit, 0, len(best))
	for _, hit := range best {
		unique = append(unique, hit)
	}
	sortMemoryHits(unique)
	if len(unique) > ragMaxRecords {
		unique = unique[:ragMaxRecords]
	}
	return unique
}

func memoryQueryText(message string) string {
	text := strings.TrimSpace(message)
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return ""
	}
	// 去掉开头的 [群聊: …][用户: …]: 前缀
	trimmed := text
	for strings.HasPrefix(trimmed, "[") {
		end := strings.Index(trimmed, "]")
		if end < 0 {
			break
		}
		trimmed = strings.TrimSpace(trimmed[end+1:])
		if strings.HasPrefix(trimmed, ":") || strings.HasPrefix(trimmed, "：") {
			trimmed = strings.TrimSpace(trimmed[1:])
		}
	}
	if trimmed == "" {
		return text
	}
	return trimmed
}
