package websearch

import "time"

// ============================================================
// 搜索模式常量
// ============================================================

const (
	// ModeSimple 轻量摘要模式
	ModeSimple SearchMode = "simple"
	// ModeWebpage 网页搜索模式
	ModeWebpage SearchMode = "webpage"
	// ModeDepth 深度研究模式
	ModeDepth SearchMode = "depth"
)

// ============================================================
// 深度研究预算
// ============================================================

const (
	depthMaxResultsPerSub     = 15    // 每个子问题最多返回结果数
	depthMaxSnippetLen        = 300   // 每条Snippet最大字符数（中文需足够上下文）
	depthMaxFetchedContentLen = 1500  // 抓取的网页内容截断长度
	depthMaxPromptChars       = 12000 // generateReport prompt总预算
	depthMaxOutputChars       = 3000  // LLM报告输出最大字符数
	depthMaxSubResultsChars   = 8000  // 子问题结果注入generateReport的总预算
)

// ============================================================
// 网页搜索预算
// ============================================================

const (
	webpageMaxFetchResults    = 30   // 网页搜索最多抓取条数
	webpageMaxTotalContentLen = 8000 // 总内容预算：最多8000字符
	webpageMaxPerPageLen      = 1500 // 单页截断
	webpageMaxLLMOutputLen    = 1500 // LLM 总结输出上限
	webpageMaxPromptChars     = 8000 // prompt 总大小预算
	webpageMaxFallbackChars   = 4000 // 回退格式化截断上限
)

// defaultConfig 默认配置
var defaultConfig = Config{
	Simple: SimpleConfig{
		MaxResults: 10,
	},
	Webpage: WebpageConfig{
		MaxResults:       30,
		FetchContent:     true,
		FetchTimeout:     10,
		MaxContentLength: 2000,
	},
	Depth: DepthConfig{
		MaxResults:    10,
		MaxSubQueries: 6,
	},
	LLM: LLMConfig{
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		MaxTokens:   4096,
		Temperature: 0.7,
	},
	HTTP: HTTPConfig{
		Timeout:   10 * time.Second,
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	},
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return defaultConfig
}
