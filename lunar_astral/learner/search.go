package learner

import (
	"fmt"
	"strings"

	"logger"
	"websearch"
)

// SearchManager 搜索管理器
// 封装 websearch 包，提供搜索结果压缩和 token 预算控制
type SearchManager struct {
	searchSystem *websearch.System
	initialized  bool
}

// NewSearchManager 创建搜索管理器
func NewSearchManager() *SearchManager {
	return &SearchManager{}
}

// Init 初始化搜索子系统（使用学习者配置）
func (m *SearchManager) Init(cfg LearnerConfig) error {
	if m.initialized {
		return nil
	}

	wsCfg := websearch.DefaultConfig()
	wsCfg.LLM.BaseURL = cfg.BaseURL
	wsCfg.LLM.APIKey = cfg.APIKey
	wsCfg.LLM.Model = cfg.Model
	wsCfg.LLM.MaxTokens = cfg.MaxTokens
	wsCfg.LLM.Temperature = cfg.Temperature

	// 启用深度搜索，增加子问题数量
	wsCfg.Depth.Enabled = true
	wsCfg.Depth.MaxSubQueries = MaxSearchSubQuestions
	wsCfg.Depth.MaxResults = 15

	m.searchSystem = websearch.NewWithConfig(wsCfg)

	if m.searchSystem.HasLLM() {
		logger.Info("Learner", "搜索子系统初始化成功，LLM 已配置: %s", cfg.Model)
	} else {
		logger.Warn("Learner", "搜索子系统初始化成功，但 LLM 未配置")
	}

	m.initialized = true
	return nil
}

// DepthSearch 执行深度搜索
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

// SearchAndCompress 执行深度搜索并压缩结果到指定字符数内
// 如果搜索结果超过 maxChars，使用 LLM 进行摘要压缩
func (m *SearchManager) SearchAndCompress(query string, maxChars int, llm *LLMClient) (string, error) {
	if !m.initialized || m.searchSystem == nil {
		return "", fmt.Errorf("搜索子系统未初始化")
	}

	result, err := m.DepthSearch(query)
	if err != nil {
		return "", err
	}

	// 如果结果已经足够短，直接返回
	if len([]rune(result)) <= maxChars {
		return result, nil
	}

	// 使用 LLM 压缩搜索结果
	if llm != nil {
		compressed, compressErr := m.compressResult(result, maxChars, llm)
		if compressErr == nil && len([]rune(compressed)) <= maxChars {
			logger.Info("Learner", "搜索结果已压缩: %d → %d 字符", len([]rune(result)), len([]rune(compressed)))
			return compressed, nil
		}
		// 压缩失败，回退到截断
		if compressErr != nil {
			logger.Warn("Learner", "搜索结果 LLM 压缩失败: %v，回退到截断", compressErr)
		}
	}

	// 截断到最大字符数
	truncated := truncateText(result, maxChars)
	logger.Info("Learner", "搜索结果已截断: %d → %d 字符", len([]rune(result)), len([]rune(truncated)))
	return truncated, nil
}

// IsAvailable 检查搜索是否可用
func (m *SearchManager) IsAvailable() bool {
	return m.initialized && m.searchSystem != nil
}

// SimpleSearchRaw 执行轻量摘要搜索，返回原始搜索结果预览
func (m *SearchManager) SimpleSearchRaw(query string) ([]SearchItemPreview, error) {
	if !m.initialized || m.searchSystem == nil {
		return nil, fmt.Errorf("搜索子系统未初始化")
	}

	logger.Info("Learner", "执行轻量摘要搜索: %s", query)

	results, err := m.searchSystem.SimpleSearchRaw(query)
	if err != nil {
		return nil, err
	}

	// 转换为 learner 包的 SearchItemPreview
	preview := make([]SearchItemPreview, 0, len(results))
	for _, r := range results {
		preview = append(preview, SearchItemPreview{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: truncateText(r.Snippet, 200), // 截断摘要避免过长
		})
	}
	return preview, nil
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
	default: // AgentModeSimple
		return m.searchSystem.SimpleSearch(query)
	}
}

// compressResult 使用 LLM 压缩搜索结果
func (m *SearchManager) compressResult(result string, maxChars int, llm *LLMClient) (string, error) {
	prompt := fmt.Sprintf(`请将以下搜索结果压缩为不超过 %d 字的精炼摘要。保留所有关键事实、数据、来源信息，删除冗余内容。

搜索结果：
%s

要求：
1. 保留所有关键事实和数据
2. 保留信息来源
3. 删除重复和冗余内容
4. 不超过 %d 字`, maxChars, result, maxChars)

	messages := []LLMMessage{
		{Role: "system", Content: "你是一个信息压缩助手，擅长在保留关键信息的前提下精炼文本。"},
		{Role: "user", Content: prompt},
	}

	resp, err := llm.Chat(messages, BudgetSearchCompress)
	if err != nil {
		return "", err
	}

	compressed := strings.TrimSpace(resp.Content)
	if len([]rune(compressed)) > maxChars {
		compressed = truncateText(compressed, maxChars)
	}

	return compressed, nil
}
