package websearch

import (
	"fmt"
	"strings"
)

// NewWithConfig 使用自定义配置创建网络检索子系统
func NewWithConfig(cfg Config) *System {
	s := &System{cfg: cfg}

	s.simple = NewSimpleSearcher(cfg)

	// 初始化 LLM（如果配置了 API Key）
	if cfg.LLM.APIKey != "" {
		s.llmProvider = NewOpenAIProvider(
			cfg.LLM.BaseURL,
			cfg.LLM.APIKey,
			cfg.LLM.Model,
			cfg.LLM.MaxTokens,
			cfg.LLM.Temperature,
		)
	}

	s.webpage = NewWebpageSearcher(s.simple, s.llmProvider, cfg.Webpage, cfg.HTTP)
	s.depth = NewDepthSearcher(s.simple, s.llmProvider, cfg.Depth, cfg.HTTP)

	return s
}

// NewWithLLM 使用自定义 LLM 提供者创建网络检索子系统
func NewWithLLM(cfg Config, provider Provider) *System {
	s := &System{
		cfg:         cfg,
		llmProvider: provider,
	}

	s.simple = NewSimpleSearcher(cfg)
	s.webpage = NewWebpageSearcher(s.simple, s.llmProvider, cfg.Webpage, cfg.HTTP)
	s.depth = NewDepthSearcher(s.simple, s.llmProvider, cfg.Depth, cfg.HTTP)

	return s
}

// SetMemoryProvider 设置记忆提供者（供大会辩论使用）
func (s *System) SetMemoryProvider(mp MemoryProvider) {
	s.memProvider = mp
	if s.cfg.Depth.Enabled && s.llmProvider != nil {
		s.assembly = NewAssembly(s.simple, s.llmProvider, mp, s.cfg.Depth)
	}
}

// SetVisionProvider 设置图片识别提供者（使用已有的模型池实例）
func (s *System) SetVisionProvider(vp VisionProvider) {
	s.visionProvider = vp
}

// SetDownloadFunc 设置下载回调函数（由调用方注入下载管理器）
func (s *System) SetDownloadFunc(fn DownloadFunc) {
	s.downloadFunc = fn
}

// SetDownloadGroupID 设置下载目标群组ID（每次处理前由调用方设置）
func (s *System) SetDownloadGroupID(groupID string) {
	s.downloadGroupID = groupID
}

// ProcessLinks 检测并替换消息中的链接为摘要，返回替换后的纯文本和链接描述列表。
// 不触发搜索，仅处理链接抓取和替换。
func (s *System) ProcessLinks(query string) (string, []string) {
	urls := extractURLs(query)
	if len(urls) == 0 {
		return query, nil
	}
	replacedQuery, linkMap := s.processAndReplaceLinks(query)

	// 构建链接描述列表
	descriptions := linkMapToDescriptions(linkMap)
	return replacedQuery, descriptions
}

// linkMapToDescriptions 将链接映射转换为描述列表
func linkMapToDescriptions(linkMap map[string]string) []string {
	if len(linkMap) == 0 {
		return nil
	}
	descs := make([]string, 0, len(linkMap))
	for i := 1; i <= len(linkMap); i++ {
		label := fmt.Sprintf("[链接%d]", i)
		if content, ok := linkMap[label]; ok {
			// content 格式为 "url：摘要"，提取摘要部分
			parts := strings.SplitN(content, "：", 2)
			summary := content
			if len(parts) == 2 {
				summary = parts[1]
			}
			descs = append(descs, fmt.Sprintf("链接%d: %s", i, summary))
		}
	}
	return descs
}

// Search 执行搜索（根据模式自动选择搜索策略）
// 如果查询中包含链接，会先剥离URL再对剩余文字执行搜索，让规划器通过 fetch_url 工具单独处理链接
func (s *System) Search(query string, mode SearchMode) (string, error) {
	// 检测并剥离链接（链接内容由规划器通过 fetch_url 工具单独处理）
	urls := extractURLs(query)
	if len(urls) > 0 {
		searchText := stripURLs(query, urls)
		searchText = strings.TrimSpace(searchText)

		if searchText == "" {
			// 纯链接消息，提示规划器使用 fetch_url
			return "查询中仅包含链接，链接内容请使用 fetch_url 工具单独获取。", nil
		}

		// 对剩余文字执行搜索
		return s.doSearch(searchText, mode)
	}

	return s.doSearch(query, mode)
}

// doSearch 按模式路由搜索（原 Search 逻辑）
func (s *System) doSearch(query string, mode SearchMode) (string, error) {
	switch mode {
	case ModeWebpage:
		return s.webpageSearch(query)
	case ModeDepth:
		if s.assembly != nil {
			return s.assembly.Search(query)
		}
		return s.depthSearch(query)
	default:
		return s.simpleSearch(query)
	}
}

// simpleSearch 执行轻量摘要搜索
func (s *System) simpleSearch(query string) (string, error) {
	return s.simple.Search(query)
}

// webpageSearch 执行网页搜索
func (s *System) webpageSearch(query string) (string, error) {
	return s.webpage.Search(query)
}

// depthSearch 执行深度研究
func (s *System) depthSearch(query string) (string, error) {
	return s.depth.Search(query)
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return defaultConfig
}

// SetSimpleMaxResults 设置轻量摘要搜索最大结果数
func (s *System) SetSimpleMaxResults(n int) {
	s.simple.SetMaxResults(n)
}

// GetConfig 获取当前配置
func (s *System) GetConfig() Config {
	return s.cfg
}

// HasLLM 检查是否配置了 LLM
func (s *System) HasLLM() bool {
	return s.llmProvider != nil
}

// SimpleSearch 执行轻量摘要搜索
func (s *System) SimpleSearch(query string) (string, error) {
	return s.simpleSearch(query)
}

// WebpageSearch 执行网页搜索
func (s *System) WebpageSearch(query string) (string, error) {
	return s.webpageSearch(query)
}

// DepthSearch 执行深度研究
func (s *System) DepthSearch(query string) (string, error) {
	return s.depthSearch(query)
}
