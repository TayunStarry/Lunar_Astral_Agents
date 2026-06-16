package websearch

// New 创建网络检索子系统（使用默认配置，AI 功能不可用）
func New() *System {
	return NewWithConfig(defaultConfig)
}

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

// Search 执行搜索（根据模式自动选择搜索策略）
func (s *System) Search(query string, mode SearchMode) (string, error) {
	switch mode {
	case ModeWebpage:
		return s.WebpageSearch(query)
	case ModeDepth:
		return s.DepthSearch(query)
	default:
		return s.SimpleSearch(query)
	}
}

// SimpleSearch 执行轻量摘要搜索
func (s *System) SimpleSearch(query string) (string, error) {
	return s.simple.Search(query)
}

// WebpageSearch 执行网页搜索
func (s *System) WebpageSearch(query string) (string, error) {
	return s.webpage.Search(query)
}

// DepthSearch 执行深度研究
func (s *System) DepthSearch(query string) (string, error) {
	return s.depth.Search(query)
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

// ---- 便捷函数 ----

// QuickSearch 快速轻量摘要（使用默认配置）
func QuickSearch(query string) (string, error) {
	sys := New()
	return sys.SimpleSearch(query)
}

// QuickWebpageSearch 快速网页搜索（使用指定 LLM 配置）
func QuickWebpageSearch(query string, llmCfg LLMConfig) (string, error) {
	cfg := defaultConfig
	cfg.LLM = llmCfg
	sys := NewWithConfig(cfg)
	return sys.WebpageSearch(query)
}

// QuickDepthSearch 快速深度研究（使用指定 LLM 配置）
func QuickDepthSearch(query string, llmCfg LLMConfig) (string, error) {
	cfg := defaultConfig
	cfg.LLM = llmCfg
	sys := NewWithConfig(cfg)
	return sys.DepthSearch(query)
}
