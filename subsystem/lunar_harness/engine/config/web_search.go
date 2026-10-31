package config

// ==== 网络搜索智能体（lunar_config.json 的 web_search 节） ====
//
// 这些字段由本包的 init → applyConfig 统一解析，Web-LTP 只读取解析结果。
// 指针字段用于区分「未配置」与「显式关闭」；数值字段 0 表示未配置，沿用默认值。

const (
	// WebSearchModel 检索词提炼与统一摘要固定使用的轻量模型名
	WebSearchModel = "light_reasoning"
)

var (
	// WebSearchMaxPages 单次搜索的页面条目总数：首页（浏览器引擎页）恒占 1 条，其余为详情页
	WebSearchMaxPages = 6
	// WebSearchMaxResults 结果页解析的检索结果条数
	WebSearchMaxResults = 15
	// WebSearchSummaryMaxChars 统一摘要硬切断字符数
	WebSearchSummaryMaxChars = 4096
	// WebSearchMaxRunSeconds 单次检索总时长软上限（秒）
	WebSearchMaxRunSeconds = 420
	// WebSearchCacheEnabled 近期访问标记开关（nil=默认开启）
	WebSearchCacheEnabled *bool
	// WebSearchCacheTTLDays 近期访问标记的有效期（天）
	WebSearchCacheTTLDays = 7
	// WebSearchTextTotalChars 全部页面正文的总提取硬上限（由所有页面均分）
	WebSearchTextTotalChars = 30000
	// WebSearchScrollSettleMS 滚动走查中每步之间的等待毫秒数（中途只滚动不提取，短等待即可）
	WebSearchScrollSettleMS = 120
	// WebSearchScrollMaxSteps 单页滚动步数上限
	WebSearchScrollMaxSteps = 40
)
