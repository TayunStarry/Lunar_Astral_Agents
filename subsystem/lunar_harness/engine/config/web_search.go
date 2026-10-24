package config

// ==== 网络搜索智能体（lunar_config.json 的 web_search 节） ====
//
// 这些字段由本包的 init → applyConfig 统一解析，Web-LTP 只读取解析结果。
// 指针字段用于区分「未配置」与「显式关闭」；数值字段 0 表示未配置，沿用默认值。

var (
	// WebSearchModel 检索词提炼与逐页摘要使用的模型名
	WebSearchModel = "system-search"
	// WebSearchSwapModel 搜索前是否卸载主模型并加载搜索模型（nil=默认开启）
	WebSearchSwapModel *bool
	// WebSearchMaxPages 进入详情页的页数上限（智能体侧再施加硬上限）
	WebSearchMaxPages = 8
	// WebSearchMaxResults 结果页解析的检索结果条数
	WebSearchMaxResults = 15
	// WebSearchSummaryMaxChars 单页摘要硬切断字符数
	WebSearchSummaryMaxChars = 4096
	// WebSearchMaxRunSeconds 单次检索总时长软上限（秒）
	WebSearchMaxRunSeconds = 420
	// WebSearchCacheEnabled 近期访问标记开关（nil=默认开启）
	WebSearchCacheEnabled *bool
	// WebSearchCacheTTLDays 近期访问标记的有效期（天）
	WebSearchCacheTTLDays = 7
	// WebSearchTextChunkChars 文本摘要的单段字符数
	WebSearchTextChunkChars = 5000
	// WebSearchTextMaxChars 单页正文提取硬上限
	WebSearchTextMaxChars = 24000
	// WebSearchTextMaxChunks 单页最多参与摘要的文本段数
	WebSearchTextMaxChunks = 4
	// WebSearchScrollSettleMS 每次滚动后的等待毫秒数
	WebSearchScrollSettleMS = 350
	// WebSearchScrollMaxSteps 单页滚动步数上限
	WebSearchScrollMaxSteps = 40
)
