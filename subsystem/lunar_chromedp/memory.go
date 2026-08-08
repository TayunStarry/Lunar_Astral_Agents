package lunar_chromedp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"storage/module"
)

// =============================================================================
// 记忆系统集成 — 基于 storage/module 的记忆库 CRUD
// =============================================================================

const searchMemoryCollection = "search_memory"

func init() {
	// 注册记忆库钩子到 agent.go
	memoryInitCollection = initMemoryCollection
	memoryLookup = lookupMemory
	memoryStore = storeMemoryRecord
}

// =============================================================================
// Hook 实现：集合初始化
// =============================================================================

// initMemoryCollection 初始化记忆库连接并创建 search_memory 集合
// 若集合维度因模型变更而不匹配，则销毁旧集合重新创建
func initMemoryCollection(embeddingURL, embeddingModel, embeddingKey string) error {
	// 初始化记忆库实例（内部调用 InitMemoryDB + MemoryInitInstance）
	if !module.IsInitialized() {
		if err := module.MemoryInitInstance(embeddingURL, embeddingKey); err != nil {
			return fmt.Errorf("记忆库实例初始化失败: %w", err)
		}
		fmt.Printf("[%s] 记忆库实例已初始化\n", ModuleName)
	}

	// 检查 search_memory 集合状态
	model, dim, count, err := module.MemoryGetCollectionInfo(searchMemoryCollection)

	if err != nil {
		// 集合不存在，创建新集合
		fmt.Printf("[%s] 记忆集合 '%s' 不存在，正在创建...\n", ModuleName, searchMemoryCollection)
		return createCollection(embeddingModel)
	}

	fmt.Printf("[%s] 记忆集合 '%s' 已存在 (模型=%s 维度=%d 文档=%d)\n",
		ModuleName, searchMemoryCollection, model, dim, count)

	// 检查维度是否匹配
	if module.HasSyncMismatch(searchMemoryCollection) {
		fmt.Printf("[%s] 记忆集合维度不匹配，销毁并重建...\n", ModuleName)
		if err := module.DeleteCollection(searchMemoryCollection); err != nil {
			return fmt.Errorf("删除旧记忆集合失败: %w", err)
		}
		return createCollection(embeddingModel)
	}

	return nil
}

// createCollection 创建 search_memory 集合
func createCollection(modelName string) error {
	ctx := context.Background()
	if err := module.CollectionInit(ctx, searchMemoryCollection, modelName); err != nil {
		return fmt.Errorf("创建记忆集合 '%s' 失败: %w", searchMemoryCollection, err)
	}

	model, dim, count, err := module.MemoryGetCollectionInfo(searchMemoryCollection)
	if err != nil {
		return fmt.Errorf("获取新集合信息失败: %w", err)
	}

	fmt.Printf("[%s] 记忆集合 '%s' 创建完成 (模型=%s 维度=%d 文档=%d)\n",
		ModuleName, searchMemoryCollection, model, dim, count)
	return nil
}

// =============================================================================
// Hook 实现：记忆检索
// =============================================================================

// lookupMemory 在 search_memory 集合中检索相似历史记录
// 返回按相似度降序排列的记录列表
func lookupMemory(query string, topK int) ([]memoryEntry, error) {
	if !module.IsInitialized() {
		return nil, fmt.Errorf("记忆库未初始化")
	}

	ctx := context.Background()

	// 查询记忆库（storage 模块内部完成嵌入和相似度计算）
	results, err := module.QueryMessagesWithContent(ctx, searchMemoryCollection, query, topK)
	if err != nil {
		return nil, fmt.Errorf("记忆检索失败: %w", err)
	}

	// 解析结果：Content 字段是 JSON 序列化的 MemorySearchRecord
	entries := make([]memoryEntry, 0, len(results))
	for _, r := range results {
		entry := memoryEntry{
			Similarity: r.Similarity,
		}

		// 尝试解析为 MemorySearchRecord
		var record MemorySearchRecord
		if err := json.Unmarshal([]byte(r.Content), &record); err == nil {
			// 成功解析，构建可读文本
			entry.Content = formatMemoryRecord(&record)
		} else {
			// 旧格式或非 JSON 内容，直接使用原始文本
			entry.Content = r.Content
		}

		entries = append(entries, entry)
	}

	fmt.Printf("[%s] 记忆检索完成，查询='%s'，找到 %d 条结果\n",
		ModuleName, truncateText(query, 50), len(entries))

	return entries, nil
}

// =============================================================================
// Hook 实现：记忆存储
// =============================================================================

// storeMemoryRecord 将搜索记录存入 search_memory 集合
func storeMemoryRecord(record MemorySearchRecord) error {
	if !module.IsInitialized() {
		return fmt.Errorf("记忆库未初始化")
	}

	// 序列化为 JSON
	jsonData, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("序列化记忆记录失败: %w", err)
	}

	ctx := context.Background()

	// 存入记忆库（role 使用 "search" 标识搜索记录）
	id, err := module.AddMessageWithID(ctx, searchMemoryCollection, "search", string(jsonData))
	if err != nil {
		return fmt.Errorf("存储记忆记录失败: %w", err)
	}

	fmt.Printf("[%s] 记忆记录已存储 ID=%s 问题='%s'\n",
		ModuleName, id, truncateText(record.Question, 50))
	return nil
}

// =============================================================================
// 辅助函数
// =============================================================================

// formatMemoryRecord 将 MemorySearchRecord 格式化为可读文本
func formatMemoryRecord(r *MemorySearchRecord) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("问题：%s\n", r.Question))

	if len(r.Keywords) > 0 {
		sb.WriteString(fmt.Sprintf("搜索关键词：%s\n", strings.Join(r.Keywords, "、")))
	}

	if r.KeyFindings != "" {
		// 截断过长发现
		findings := truncateText(r.KeyFindings, 300)
		sb.WriteString(fmt.Sprintf("关键发现：\n%s\n", findings))
	}

	sb.WriteString(fmt.Sprintf("答案：%s", truncateText(r.Answer, 500)))

	return sb.String()
}
