package learner

import (
	"context"
	"fmt"
	"strings"

	"logger"
	"storage/module"
)

// NewMemoryManager 创建记忆管理器
func NewMemoryManager() *MemoryManager {
	return &MemoryManager{}
}

// Init 初始化记忆库（创建知识记忆和经验记忆两个集合）
func (m *MemoryManager) Init(cfg LearnerConfig) error {
	if m.initialized {
		return nil
	}

	// 第一步：初始化记忆库实例
	if err := module.MemoryInitInstance(cfg.EmbeddingURL, cfg.EmbeddingKey); err != nil {
		logger.Error("Learner", "记忆库实例初始化失败: %v", err)
		return fmt.Errorf("记忆库实例初始化失败: %w", err)
	}

	ctx := context.Background()

	// 第二步：创建知识记忆集合
	if err := module.CollectionInit(ctx, TableKnowledge, cfg.EmbeddingModel); err != nil {
		logger.Error("Learner", "知识记忆集合 [%s] 创建失败: %v", TableKnowledge, err)
		return fmt.Errorf("知识记忆集合 [%s] 创建失败: %w", TableKnowledge, err)
	}

	// 第三步：创建经验记忆集合
	if err := module.CollectionInit(ctx, TableExperience, cfg.EmbeddingModel); err != nil {
		logger.Error("Learner", "经验记忆集合 [%s] 创建失败: %v", TableExperience, err)
		return fmt.Errorf("经验记忆集合 [%s] 创建失败: %w", TableExperience, err)
	}

	m.initialized = true
	logger.Info("Learner", "记忆库初始化完成，知识表: %s，经验表: %s，嵌入模型: %s",
		TableKnowledge, TableExperience, cfg.EmbeddingModel)
	return nil
}

// QueryKnowledge 查询知识记忆
func (m *MemoryManager) QueryKnowledge(queryText string, topK int) ([]MemoryMatch, error) {
	return m.query(TableKnowledge, queryText, topK)
}

// QueryExperience 查询经验记忆
func (m *MemoryManager) QueryExperience(queryText string, topK int) ([]MemoryMatch, error) {
	return m.query(TableExperience, queryText, topK)
}

// QueryBoth 同时查询知识记忆和经验记忆
// 返回知识记忆列表和经验记忆列表
func (m *MemoryManager) QueryBoth(queryText string) ([]MemoryMatch, []MemoryMatch, error) {
	if !m.initialized {
		return nil, nil, fmt.Errorf("记忆库未初始化")
	}

	knowledgeResults, err := m.QueryKnowledge(queryText, MemoryQueryTopK)
	if err != nil {
		logger.Warn("Learner", "知识记忆查询失败: %v", err)
		knowledgeResults = nil
	}

	experienceResults, err := m.QueryExperience(queryText, ExperienceQueryTopK)
	if err != nil {
		logger.Warn("Learner", "经验记忆查询失败: %v", err)
		experienceResults = nil
	}

	return knowledgeResults, experienceResults, nil
}

// query 内部查询方法
func (m *MemoryManager) query(tableName string, queryText string, topK int) ([]MemoryMatch, error) {
	if !m.initialized {
		return nil, fmt.Errorf("记忆库未初始化")
	}

	if topK <= 0 {
		topK = MemoryQueryTopK
	}

	ctx := context.Background()
	results, err := module.QueryMessagesWithContent(ctx, tableName, queryText, topK)
	if err != nil {
		logger.Error("Learner", "记忆查询失败 [%s]: %v", tableName, err)
		return nil, err
	}

	matches := make([]MemoryMatch, 0, len(results))
	for _, r := range results {
		matches = append(matches, MemoryMatch{
			ID:         r.ID,
			Content:    r.Content,
			Similarity: r.Similarity,
			Table:      tableName,
		})
	}

	logger.Info("Learner", "记忆查询 [%s]: %d 条结果", tableName, len(matches))
	return matches, nil
}

// AddKnowledge 添加知识记忆条目
func (m *MemoryManager) AddKnowledge(content string) (string, error) {
	return m.add(TableKnowledge, content)
}

// AddExperience 添加经验记忆条目
func (m *MemoryManager) AddExperience(content string) (string, error) {
	return m.add(TableExperience, content)
}

// add 内部添加方法
func (m *MemoryManager) add(tableName string, content string) (string, error) {
	if !m.initialized {
		return "", fmt.Errorf("记忆库未初始化")
	}

	ctx := context.Background()
	id, err := module.AddMessageWithID(ctx, tableName, "user", content)
	if err != nil {
		logger.Error("Learner", "记忆添加失败 [%s]: %v", tableName, err)
		return "", err
	}

	logger.Info("Learner", "记忆添加成功 [%s]: id=%s, 内容长度=%d", tableName, id, len([]rune(content)))
	return id, nil
}

// BatchAddKnowledge 批量添加知识记忆条目
func (m *MemoryManager) BatchAddKnowledge(contents []string) (int, error) {
	return m.batchAdd(TableKnowledge, contents)
}

// batchAdd 批量添加
func (m *MemoryManager) batchAdd(tableName string, contents []string) (int, error) {
	if !m.initialized {
		return 0, fmt.Errorf("记忆库未初始化")
	}

	successCount := 0
	ctx := context.Background()

	for _, content := range contents {
		if len([]rune(strings.TrimSpace(content))) < 10 {
			continue // 跳过太短的内容
		}
		_, err := module.AddMessageWithID(ctx, tableName, "user", content)
		if err != nil {
			logger.Warn("Learner", "批量添加记忆失败 [%s]: %v", tableName, err)
			continue
		}
		successCount++
	}

	logger.Info("Learner", "批量添加记忆 [%s]: %d/%d 成功", tableName, successCount, len(contents))
	return successCount, nil
}

// FindSuperseded 查找需要被替代的旧记忆条目
func (m *MemoryManager) FindSuperseded(tableName string, newContent string, threshold float32) ([]MemoryMatch, error) {
	if !m.initialized {
		return nil, fmt.Errorf("记忆库未初始化")
	}

	results, err := m.query(tableName, newContent, MemoryQueryTopK)
	if err != nil {
		return nil, err
	}

	var superseded []MemoryMatch
	for _, match := range results {
		if match.Similarity >= threshold {
			superseded = append(superseded, match)
		}
	}

	return superseded, nil
}

// DeleteEntry 删除记忆条目
func (m *MemoryManager) DeleteEntry(tableName string, id string) error {
	if !m.initialized {
		return fmt.Errorf("记忆库未初始化")
	}

	ctx := context.Background()
	err := module.DeleteMessage(ctx, tableName, id)
	if err != nil {
		logger.Warn("Learner", "记忆删除失败 [%s] id=%s: %v", tableName, id, err)
		return err
	}

	logger.Info("Learner", "记忆删除成功 [%s] id=%s", tableName, id)
	return nil
}

// IsAvailable 检查记忆库是否可用
func (m *MemoryManager) IsAvailable() bool {
	return m.initialized && module.IsInitialized()
}

// HasKnowledgeMatches 检查知识记忆是否有足够的匹配
// 用于降级决策：知识库可用但匹配不足时返回"月华不知道"
func (m *MemoryManager) HasKnowledgeMatches(matches []MemoryMatch) bool {
	if len(matches) < KnowledgeMinMatchCount {
		return false
	}

	// 检查是否有足够的高相似度匹配
	qualifiedCount := 0
	for _, match := range matches {
		if match.Similarity >= KnowledgeMinSimilarity {
			qualifiedCount++
		}
	}

	return qualifiedCount >= KnowledgeMinMatchCount
}

// FormatMemoryResults 格式化记忆检索结果为可读文本
func FormatMemoryResults(matches []MemoryMatch) string {
	if len(matches) == 0 {
		return "没有回忆起相关的内容"
	}

	var parts []string
	for i, match := range matches {
		parts = append(parts, fmt.Sprintf("[记忆%d] 相关度:%.1f%% | 内容:%s",
			i+1, match.Similarity*100, match.Content))
	}

	return strings.Join(parts, "\n")
}