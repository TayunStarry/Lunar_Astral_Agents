package websearch

// New 创建网络检索子系统（使用默认配置，AI 功能不可用）
func New() *System {
	return NewWithConfig(defaultConfig)
}

// NewWithConfig 使用自定义配置创建网络检索子系统
func NewWithConfig(cfg Config) *System {
	s := &System{cfg: cfg}

	s.shallow = NewShallowSearcher(cfg)

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

	s.deep = NewDeepSearcher(s.shallow, s.llmProvider, cfg.Deep, cfg.HTTP)
	s.research = NewResearchSearcher(s.shallow, s.llmProvider, cfg.Research)

	return s
}

// NewWithLLM 使用自定义 LLM 提供者创建网络检索子系统
func NewWithLLM(cfg Config, provider Provider) *System {
	s := &System{
		cfg:         cfg,
		llmProvider: provider,
	}

	s.shallow = NewShallowSearcher(cfg)
	s.deep = NewDeepSearcher(s.shallow, s.llmProvider, cfg.Deep, cfg.HTTP)
	s.research = NewResearchSearcher(s.shallow, s.llmProvider, cfg.Research)

	return s
}

// Search 执行搜索（根据模式自动选择搜索策略）
func (s *System) Search(query string, mode SearchMode) (string, error) {
	switch mode {
	case ModeDeep:
		return s.DeepSearch(query)
	case ModeResearch:
		return s.ResearchSearch(query)
	default:
		return s.ShallowSearch(query)
	}
}

// ShallowSearch 执行浅层搜索
func (s *System) ShallowSearch(query string) (string, error) {
	return s.shallow.Search(query)
}

// DeepSearch 执行深层搜索
func (s *System) DeepSearch(query string) (string, error) {
	return s.deep.Search(query)
}

// ResearchSearch 执行研究搜索
func (s *System) ResearchSearch(query string) (string, error) {
	return s.research.Search(query)
}

// SetShallowMaxResults 设置浅层搜索最大结果数
func (s *System) SetShallowMaxResults(n int) {
	s.shallow.SetMaxResults(n)
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

// QuickSearch 快速浅层搜索（使用默认配置）
func QuickSearch(query string) (string, error) {
	sys := New()
	return sys.ShallowSearch(query)
}

// QuickDeepSearch 快速深层搜索（使用指定 LLM 配置）
func QuickDeepSearch(query string, llmCfg LLMConfig) (string, error) {
	cfg := defaultConfig
	cfg.LLM = llmCfg
	sys := NewWithConfig(cfg)
	return sys.DeepSearch(query)
}

// QuickResearchSearch 快速研究搜索（使用指定 LLM 配置）
func QuickResearchSearch(query string, llmCfg LLMConfig) (string, error) {
	cfg := defaultConfig
	cfg.LLM = llmCfg
	sys := NewWithConfig(cfg)
	return sys.ResearchSearch(query)
}
