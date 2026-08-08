package lunar_chromedp

import (
	"context"
	"fmt"
	"strings"

	"storage/module"
)

// =============================================================================
// v2 记忆系统集成 — 基于 storage/module 的标签向量中介检索
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
// embeddingURL: 嵌入服务 base_url（也是 LLM 服务的 base_url）
// embeddingModel: 嵌入模型名（也是 LLM 多模态模型名）
func initMemoryCollection(embeddingURL, embeddingModel, embeddingKey string) error {
	// 初始化记忆库实例（嵌入服务 + LLM 标签生成服务）
	if !module.IsMemoryInitialized() {
		if err := module.MemoryInitInstance(embeddingURL, embeddingKey, embeddingURL, embeddingKey, embeddingModel); err != nil {
			return fmt.Errorf("记忆库实例初始化失败: %w", err)
		}
		fmt.Printf("[%s] 记忆库实例已初始化\n", ModuleName)
	}

	// 检查 search_memory 集合状态
	info := module.MemoryGetCollectionInfo(searchMemoryCollection)

	if info == nil {
		// 集合不存在，创建新集合
		fmt.Printf("[%s] 记忆集合 '%s' 不存在，正在创建...\n", ModuleName, searchMemoryCollection)
		return createCollection(embeddingModel)
	}

	dim := getIntField(info, "embedding_dimension")
	count := getIntField(info, "document_count")
	fmt.Printf("[%s] 记忆集合 '%s' 已存在 (模型=%s 维度=%d 文档=%d)\n",
		ModuleName, searchMemoryCollection, embeddingModel, dim, count)

	// 检查维度是否匹配
	if module.MemoryHasSyncMismatch(searchMemoryCollection) {
		fmt.Printf("[%s] 记忆集合维度不匹配，销毁并重建...\n", ModuleName)
		if err := module.MemoryDeleteCollection(searchMemoryCollection); err != nil {
			return fmt.Errorf("删除旧记忆集合失败: %w", err)
		}
		return createCollection(embeddingModel)
	}

	return nil
}

// createCollection 创建 search_memory 集合
func createCollection(modelName string) error {
	ctx := context.Background()
	if err := module.CollectionInit(ctx, searchMemoryCollection, modelName, module.CollectionTypeText); err != nil {
		return fmt.Errorf("创建记忆集合 '%s' 失败: %w", searchMemoryCollection, err)
	}

	info := module.MemoryGetCollectionInfo(searchMemoryCollection)
	if info == nil {
		return fmt.Errorf("获取新集合信息失败")
	}

	dim := getIntField(info, "embedding_dimension")
	count := getIntField(info, "document_count")
	fmt.Printf("[%s] 记忆集合 '%s' 创建完成 (模型=%s 维度=%d 文档=%d)\n",
		ModuleName, searchMemoryCollection, modelName, dim, count)
	return nil
}

// =============================================================================
// Hook 实现：记忆检索
// =============================================================================

// lookupMemory 在 search_memory 集合中检索相似历史记录
// 返回按标签匹配频次得分降序排列的记录列表
func lookupMemory(query string, topK int) ([]memoryEntry, error) {
	if !module.IsMemoryInitialized() {
		return nil, fmt.Errorf("记忆库未初始化")
	}

	ctx := context.Background()

	// 查询记忆库（标签向量中介检索）
	results, err := module.MemoryQueryMessagesWithContent(ctx, searchMemoryCollection, query, topK)
	if err != nil {
		return nil, fmt.Errorf("记忆检索失败: %w", err)
	}

	// v2: Content 字段是自然语言文本，直接使用
	entries := make([]memoryEntry, 0, len(results))
	for _, r := range results {
		entries = append(entries, memoryEntry{
			Content:    r.Content,
			Similarity: r.Similarity,
		})
	}

	fmt.Printf("[%s] 记忆检索完成，查询='%s'，找到 %d 条结果\n",
		ModuleName, truncateText(query, 50), len(entries))

	return entries, nil
}

// =============================================================================
// Hook 实现：记忆存储
// =============================================================================

// storeMemoryRecord 将搜索记录以自然语言文本存入 search_memory 集合
// v2: 存储自然语言文本而非 JSON，LLM 自动生成标签用于检索
func storeMemoryRecord(record MemorySearchRecord) error {
	if !module.IsMemoryInitialized() {
		return fmt.Errorf("记忆库未初始化")
	}

	// 构建自然语言文本
	content := formatMemoryRecord(&record)

	ctx := context.Background()

	// 存入记忆库（role 使用 "search" 标识搜索记录，LLM 自动生成标签）
	id, err := module.MemoryAddMessage(ctx, searchMemoryCollection, "search", content)
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

// formatMemoryRecord 将 MemorySearchRecord 格式化为自然语言文本
func formatMemoryRecord(r *MemorySearchRecord) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("问题：%s\n", r.Question))

	if len(r.Keywords) > 0 {
		sb.WriteString(fmt.Sprintf("搜索关键词：%s\n", strings.Join(r.Keywords, "、")))
	}

	if r.KeyFindings != "" {
		findings := truncateText(r.KeyFindings, 300)
		sb.WriteString(fmt.Sprintf("关键发现：\n%s\n", findings))
	}

	sb.WriteString(fmt.Sprintf("答案：%s", truncateText(r.Answer, 500)))

	return sb.String()
}

// getIntField 从 map[string]interface{} 安全获取整数字段
func getIntField(m map[string]interface{}, key string) int {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64:
			return int(val)
		}
	}
	return 0
}