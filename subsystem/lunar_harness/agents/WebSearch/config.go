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
		TextTotalChars:  config.WebSearchTextTotalChars,
		ScrollSettleMS:  config.WebSearchScrollSettleMS,
		ScrollMaxSteps:  config.WebSearchScrollMaxSteps,
	}
	// 页面条目总数硬上限（含首页 1 条）：避免单次检索同时打开过多窗口
	if cfg.MaxPages > webSearchPagesHardCap {
		cfg.MaxPages = webSearchPagesHardCap
	}
	if cfg.MaxPages < 1 {
		cfg.MaxPages = 1
	}
	// 统一摘要长度硬上限：摘要即最终输出，超长没有意义
	if cfg.SummaryMaxChars > 4096 {
		cfg.SummaryMaxChars = 4096
	}
	// 全部页面正文的总提取硬上限：过小会丢信息，过大超出搜索模型长文承受范围
	if cfg.TextTotalChars < 6000 {
		cfg.TextTotalChars = 6000
	}
	if cfg.TextTotalChars > 48000 {
		cfg.TextTotalChars = 48000
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
