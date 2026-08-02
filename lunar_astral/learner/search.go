package learner

import (
	"fmt"
	"strings"

	"logger"
	"websearch"
)

// SearchManager 搜索管理器
// 封装 websearch 子系统，提供初步搜索和深度搜索能力
type SearchManager struct {
	searchSystem *websearch.System
	initialized  bool
}

// NewSearchManager 创建搜索管理器
func NewSearchManager() *SearchManager {
	return &SearchManager{}
}

// Init 初始化搜索子系统
func (m *SearchManager) Init(cfg LearnerConfig) error {
	if m.initialized {
		return nil
	}

	wsCfg := websearch.DefaultConfig()

	// 启用深度搜索
	wsCfg.Depth.Enabled = true
	wsCfg.Depth.MaxSubQueries = MaxSearchSubQuestions

	// 创建 LLM provider 并初始化搜索系统
	provider := websearch.NewOpenAIProvider(cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.MaxTokens, cfg.Temperature)
	m.searchSystem = websearch.NewWithLLM(wsCfg, provider)

	if m.searchSystem.HasLLM() {
		logger.Info("Learner", "搜索子系统初始化成功，LLM 已配置: %s", cfg.Model)
	} else {
		logger.Warn("Learner", "搜索子系统初始化成功，但 LLM 未配置")
	}

	m.initialized = true
	return nil
}

// SimpleSearch 执行初步网络搜索（轻量摘要模式）
// 返回搜索结果摘要列表，用于步骤 d 的初步信息获取
func (m *SearchManager) SimpleSearch(query string) ([]SearchItemPreview, error) {
	if !m.initialized || m.searchSystem == nil {
		return nil, fmt.Errorf("搜索子系统未初始化")
	}

	logger.Info("Learner", "执行初步网络搜索: %s", query)

	results, err := m.searchSystem.SimpleSearchRaw(query)
	if err != nil {
		logger.Error("Learner", "初步网络搜索失败: %v", err)
		return nil, err
	}

	preview := make([]SearchItemPreview, 0, len(results))
	for _, r := range results {
		preview = append(preview, SearchItemPreview{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: truncateRunes(r.Snippet, 200),
		})
	}

	logger.Info("Learner", "初步网络搜索完成: %d 条结果", len(preview))
	return preview, nil
}

// DepthSearch 执行深度搜索
// 返回完整的深度研究报告
func (m *SearchManager) DepthSearch(query string) (string, error) {
	if !m.initialized || m.searchSystem == nil {
		return "", fmt.Errorf("搜索子系统未初始化")
	}

	logger.Info("Learner", "执行深度搜索: %s", query)

	result, err := m.searchSystem.DepthSearch(query)
	if err != nil {
		logger.Error("Learner", "深度搜索失败: %v", err)
		return "", err
	}

	return result, nil
}

// WebpageSearch 执行网页搜索（搜索 + 内容抓取 + LLM 总结）
func (m *SearchManager) WebpageSearch(query string) (string, error) {
	if !m.initialized || m.searchSystem == nil {
		return "", fmt.Errorf("搜索子系统未初始化")
	}

	logger.Info("Learner", "执行网页搜索: %s", query)

	return m.searchSystem.WebpageSearch(query)
}

// Search 统一搜索入口，按模式路由
func (m *SearchManager) Search(query string, mode AgentSearchMode) (string, error) {
	if !m.initialized || m.searchSystem == nil {
		return "", fmt.Errorf("搜索子系统未初始化")
	}

	switch mode {
	case AgentModeWebpage:
		return m.WebpageSearch(query)
	case AgentModeDepth:
		return m.DepthSearch(query)
	default:
		// Simple 模式：返回格式化文本
		items, err := m.SimpleSearch(query)
		if err != nil {
			return "", err
		}
		return formatSimpleSearchResults(items), nil
	}
}

// IsAvailable 检查搜索是否可用
func (m *SearchManager) IsAvailable() bool {
	return m.initialized && m.searchSystem != nil
}

// formatSimpleSearchResults 格式化初步搜索结果为文本
func formatSimpleSearchResults(items []SearchItemPreview) string {
	if len(items) == 0 {
		return "未找到相关搜索结果"
	}

	var parts []string
	for i, item := range items {
		parts = append(parts, fmt.Sprintf("[%d] %s\n    %s\n    %s",
			i+1, item.Title, item.URL, item.Snippet))
	}
	return strings.Join(parts, "\n\n")
}