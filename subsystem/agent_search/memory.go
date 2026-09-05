package AgentSearch

import (
	"LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"context"
	"fmt"
	"strings"
)

// =============================================================================
// v2 记忆系统集成 — 基于 storage/module 的标签向量中介检索
// =============================================================================

func init() {
	// 注册记忆库钩子到 agent.go
	memoryInitCollection = initMemoryCollection
	memoryLookup = lookupMemory
	memoryStore = storeMemoryRecord
	memoryStorePageSummary = storePageSummaryWithTags
}

// =============================================================================
// Hook 实现：集合初始化
// =============================================================================

// initMemoryCollection 初始化记忆库连接并创建 search_memory 集合
// 模型配置（URL、模型名、API Key）从 config 模块（lunar_config.json）读取
func initMemoryCollection() error {
	// 初始化记忆库实例（嵌入服务 + LLM 标签生成服务）
	// 模型配置从 config 模块读取，不再通过参数传入
	if !module.IsMemoryInitialized() {
		if err := module.MemoryInitInstance(); err != nil {
			return fmt.Errorf("记忆库实例初始化失败: %w", err)
		}
		LoggerGeneral.Info(ModuleName, "记忆库实例已初始化 (嵌入=%s, LLM=%s)\n", *GeneralConfig.SearchEmbeddingModel, *GeneralConfig.SearchMultimodalModel)
	}

	// 检查 search_memory 集合状态
	info := module.MemoryGetCollectionInfo(searchMemoryCollection)

	if info == nil {
		// 集合不存在，创建新集合
		LoggerGeneral.Info(ModuleName, "记忆集合 '%s' 不存在，正在创建...\n", searchMemoryCollection)
		return createCollection()
	}

	dim := getIntField(info, "embedding_dimension")
	count := getIntField(info, "document_count")
	LoggerGeneral.Info(ModuleName, "记忆集合 '%s' 已存在 (模型=%s 维度=%d 文档=%d)\n", searchMemoryCollection, *GeneralConfig.SearchEmbeddingModel, dim, count)

	// 检查维度是否匹配
	if module.MemoryHasSyncMismatch(searchMemoryCollection) {
		LoggerGeneral.Info(ModuleName, "记忆集合维度不匹配，销毁并重建...\n")
		if err := module.MemoryDeleteCollection(searchMemoryCollection); err != nil {
			return fmt.Errorf("删除旧记忆集合失败: %w", err)
		}
		return createCollection()
	}

	return nil
}

// createCollection 创建 search_memory 集合
// 模型名从 config 模块读取
func createCollection() error {
	modelName := *GeneralConfig.SearchEmbeddingModel
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
	LoggerGeneral.Info(ModuleName, "记忆集合 '%s' 创建完成 (模型=%s 维度=%d 文档=%d)\n", searchMemoryCollection, modelName, dim, count)
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

	LoggerGeneral.Info(ModuleName, "记忆检索完成，查询='%s'，找到 %d 条结果\n", truncateText(query, 50), len(entries))

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

	LoggerGeneral.Info(ModuleName, "记忆记录已存储 ID=%s 问题='%s'\n", id, truncateText(record.Question, 50))
	return nil
}

// =============================================================================
// Hook 实现：页面摘要携带标签入库（跳过 LLM 标签生成）
// =============================================================================

// storePageSummaryWithTags 将单页页面摘要以显式标签方式注入 search_memory 集合
// 跳过 LLM 标签生成，直接使用调用方提供的关键词/实体作为标签，节省 LLM 调用开销。
// 即使当前查询判定该摘要不相关，也可能对未来其他相关查询有价值。
func storePageSummaryWithTags(summary string, tags []string) error {
	if !module.IsMemoryInitialized() {
		return fmt.Errorf("记忆库未初始化")
	}
	if len(tags) == 0 {
		return nil // 无标签则跳过，避免空标签入库
	}
	// 过短的摘要不入库（无效内容，浪费存储空间与检索时间）
	if len([]rune(strings.TrimSpace(summary))) < 20 {
		return nil
	}

	// 构建自然语言文本（格式与完整答案不同，便于区分来源）
	content := fmt.Sprintf("页面摘要：%s\n来源关键词：%s",
		truncateText(summary, 400), strings.Join(tags, "、"))

	ctx := context.Background()

	// 调用携带标签入库 API（跳过 LLM 标签生成）
	id, err := module.MemoryAddMessageWithTags(ctx, searchMemoryCollection, "page_summary", content, tags)
	if err != nil {
		return fmt.Errorf("页面摘要记忆存储失败: %w", err)
	}

	LoggerGeneral.Info(ModuleName, "页面摘要已入记忆库 ID=%s 标签=%v\n", id, tags)
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
