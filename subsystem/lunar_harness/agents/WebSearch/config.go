package WebSearch

// config.go 网络搜索配置
//
// lunar_config.json 的 web_search 节由 engine/config 统一解析（启动时一次），
// 智能体只读取解析后的全局值，不再自行读取配置文件；这里仅负责快照并
// 施加搜索流程可接受的取值边界。

import "LunarSubsystem/LunarHarness/engine/config"

// LoadConfig 汇总统一解析后的 web_search 配置，并收敛到搜索流程允许的取值范围。
func LoadConfig() Config {
	cfg := Config{
		SearchModel:     config.WebSearchModel,
		SwapModel:       config.WebSearchSwapModel,
		MaxPages:        config.WebSearchMaxPages,
		MaxResults:      config.WebSearchMaxResults,
		SummaryMaxChars: config.WebSearchSummaryMaxChars,
		MaxRunSeconds:   config.WebSearchMaxRunSeconds,
		CacheEnabled:    config.WebSearchCacheEnabled,
		CacheTTLDays:    config.WebSearchCacheTTLDays,
		TextChunkChars:  config.WebSearchTextChunkChars,
		TextMaxChars:    config.WebSearchTextMaxChars,
		TextMaxChunks:   config.WebSearchTextMaxChunks,
		ScrollSettleMS:  config.WebSearchScrollSettleMS,
		ScrollMaxSteps:  config.WebSearchScrollMaxSteps,
	}
	// 结果页分析页数硬上限：避免单次检索进入过多详情页
	if cfg.MaxPages > webSearchPagesHardCap {
		cfg.MaxPages = webSearchPagesHardCap
	}
	// 摘要长度硬上限：报告提示词按 4096 字预算编写
	if cfg.SummaryMaxChars > 4096 {
		cfg.SummaryMaxChars = 4096
	}
	// 文本分段与提取的上下界：过小会放大模型调用次数，过大会超出小模型上下文
	if cfg.TextChunkChars < 800 {
		cfg.TextChunkChars = 800
	}
	if cfg.TextChunkChars > 8000 {
		cfg.TextChunkChars = 8000
	}
	if cfg.TextMaxChars < 2000 {
		cfg.TextMaxChars = 2000
	}
	if cfg.TextMaxChars > 60000 {
		cfg.TextMaxChars = 60000
	}
	if cfg.TextMaxChunks > 12 {
		cfg.TextMaxChunks = 12
	}
	// 滚动节奏与步数的上下界：等待过短会漏掉懒加载内容，步数过多会拖长单页耗时
	if cfg.ScrollSettleMS < 100 {
		cfg.ScrollSettleMS = 100
	}
	if cfg.ScrollSettleMS > 2000 {
		cfg.ScrollSettleMS = 2000
	}
	if cfg.ScrollMaxSteps < 5 {
		cfg.ScrollMaxSteps = 5
	}
	if cfg.ScrollMaxSteps > 120 {
		cfg.ScrollMaxSteps = 120
	}
	return cfg
}

// swapEnabled 是否启用「搜索前卸载主模型、加载搜索小模型」的模型切换（默认开启）
func (c Config) swapEnabled() bool {
	return c.SwapModel == nil || *c.SwapModel
}

// cacheEnabled 近期访问缓存开关（未配置时默认开启）
func (c Config) cacheEnabled() bool {
	return c.CacheEnabled == nil || *c.CacheEnabled
}
