package agent

import (
	"context"
	"fmt"
	"strings"

	"logger"
	"storage/module"
)

// MemoryManager 记忆管理器
// 封装 storage/module 包，提供记忆更新逻辑
type MemoryManager struct {
	initialized bool
}

// NewMemoryManager 创建记忆管理器
func NewMemoryManager() *MemoryManager {
	return &MemoryManager{}
}

// Init 初始化记忆库（创建 online_learning 集合）
func (m *MemoryManager) Init(cfg LearnerConfig) error {
	if m.initialized {
		return nil
	}

	// 第一步：初始化记忆库实例
	if err := module.MemoryInitInstance(cfg.EmbeddingURL, cfg.EmbeddingKey); err != nil {
		logger.Error("Learner", "记忆库实例初始化失败: %v", err)
		return fmt.Errorf("记忆库实例初始化失败: %w", err)
	}

	// 第二步：创建/打开 online_learning 集合
	ctx := context.Background()
	if err := module.CollectionInit(ctx, LearnerMemoryTable, cfg.EmbeddingModel); err != nil {
		logger.Error("Learner", "集合 [%s] 创建失败: %v", LearnerMemoryTable, err)
		return fmt.Errorf("集合 [%s] 创建失败: %w", LearnerMemoryTable, err)
	}

	m.initialized = true
	logger.Info("Learner", "记忆库初始化完成，集合: %s，嵌入模型: %s", LearnerMemoryTable, cfg.EmbeddingModel)
	return nil
}

// Query 查询记忆
func (m *MemoryManager) Query(queryText string, topK int) ([]MemoryMatch, error) {
	if !m.initialized {
		return nil, fmt.Errorf("记忆库未初始化")
	}

	if topK <= 0 {
		topK = MemoryQueryTopK
	}

	ctx := context.Background()
	results, err := module.QueryMessagesWithContent(ctx, LearnerMemoryTable, queryText, topK)
	if err != nil {
		logger.Error("Learner", "记忆查询失败: %v", err)
		return nil, err
	}

	matches := make([]MemoryMatch, 0, len(results))
	for _, r := range results {
		matches = append(matches, MemoryMatch{
			ID:         r.ID,
			Content:    r.Content,
			Similarity: r.Similarity,
		})
	}

	logger.Info("Learner", "记忆查询到 %d 条结果", len(matches))
	return matches, nil
}

// Add 添加记忆条目，返回新条目 ID
func (m *MemoryManager) Add(content string) (string, error) {
	if !m.initialized {
		return "", fmt.Errorf("记忆库未初始化")
	}

	ctx := context.Background()
	id, err := module.AddMessageWithID(ctx, LearnerMemoryTable, "user", content)
	if err != nil {
		logger.Error("Learner", "记忆添加失败: %v", err)
		return "", err
	}

	logger.Info("Learner", "记忆添加成功: id=%s, 内容长度=%d", id, len([]rune(content)))
	return id, nil
}

// BatchUpdate 批量更新记忆：先添加新条目，再删除被替代的旧条目
// 实现策略：先新增完善后条目（role=user），再删除被标记为替代的旧条目
func (m *MemoryManager) BatchUpdate(updates []MemoryUpdate) error {
	if !m.initialized {
		return fmt.Errorf("记忆库未初始化")
	}

	if len(updates) == 0 {
		return nil
	}

	ctx := context.Background()
	successCount := 0

	for _, update := range updates {
		// 第一步：添加完善后的新条目
		_, err := module.AddMessageWithID(ctx, LearnerMemoryTable, "user", update.NewContent)
		if err != nil {
			logger.Warn("Learner", "记忆更新-新增失败 (oldID=%s): %v", update.OldID, err)
			continue
		}

		// 第二步：删除被替代的旧条目
		err = module.DeleteMessage(ctx, LearnerMemoryTable, update.OldID)
		if err != nil {
			logger.Warn("Learner", "记忆更新-删除旧条目失败 (id=%s): %v", update.OldID, err)
			// 新条目已添加成功，旧条目删除失败不影响流程
		}

		logger.Info("Learner", "记忆更新: 替代旧条目 %s (原因: %s)", update.OldID, update.Reason)
		successCount++
	}

	logger.Info("Learner", "批量记忆更新完成: %d/%d 成功", successCount, len(updates))
	return nil
}

// FindSuperseded 查找需要被替代的记忆条目
// 对每个新内容，查找相似度高于阈值的现有条目
func (m *MemoryManager) FindSuperseded(newContent string, threshold float32) ([]MemoryMatch, error) {
	if !m.initialized {
		return nil, fmt.Errorf("记忆库未初始化")
	}

	results, err := m.Query(newContent, MemoryQueryTopK)
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

// IsAvailable 检查记忆库是否可用
func (m *MemoryManager) IsAvailable() bool {
	return m.initialized && module.IsInitialized()
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
