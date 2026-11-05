package resourcehub

// memory.go memory 模式：只检索长期记忆库，不混入资源目录或句柄查询。
//
// 检索方式为混合检索：向量（嵌入语义）与关键词（字面/正则）两路并行召回，
// 同一记忆去重后按「综合相关度」排序；嵌入服务不可用时自动降级为关键词结果。
// 命中结果标注角色 / 时间 / 相关度 / 命中来源，便于模型判断可信度。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
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
		if label := memoryMatchLabel(hit.MatchedBy); label != "" {
			meta += "｜命中=" + label
		}
		lines = append(lines, fmt.Sprintf("[%d] %s\n%s", index+1, meta, previewRunes(hit.Content, previewRunesLimit)))
	}
	text := fmt.Sprintf("检索到 %d 条相关记忆（向量 + 关键词融合，按综合相关度排序）：\n%s", len(hits), strings.Join(lines, "\n\n"))
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

// searchMemory 薄封装：召回、去重、融合、排序、topK 裁剪全部由 memory.HybridQuery 负责；
// 这里只做参数映射与正文去重（同文多条只保留融合分最高的一条）。
func searchMemory(ctx context.Context, keywords []string, topK int) []memory.Entry {
	results, err := memory.HybridQuery(ctx, keywords, "text", topK, memory.HybridQueryOptions{})
	if err != nil {
		logger.Warn("FileManager", "[记忆检索] 混合检索失败: %v", err)
		return nil
	}
	return dedupeMemoryContent(results, topK)
}

// dedupeMemoryContent 同一正文只保留融合分最高的一条（入参已按融合分排序）
func dedupeMemoryContent(results []memory.Entry, limit int) []memory.Entry {
	if len(results) == 0 {
		return results
	}
	seen := make(map[string]bool, len(results))
	hits := make([]memory.Entry, 0, len(results))
	for _, item := range results {
		content := strings.TrimSpace(item.Content)
		if content == "" || seen[content] {
			continue
		}
		seen[content] = true
		item.Content = content
		hits = append(hits, item)
		if len(hits) >= limit {
			break
		}
	}
	return hits
}

// memoryMatchLabel 命中来源中文名
func memoryMatchLabel(matchedBy string) string {
	switch matchedBy {
	case memory.MatchedByBoth:
		return "双路"
	case memory.MatchedByVector:
		return "向量"
	case memory.MatchedByKeyword:
		return "关键词"
	}
	return ""
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
