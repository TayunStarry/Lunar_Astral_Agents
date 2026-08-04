package websearch

import (
	"net/http"
	"sync"
	"time"
)

// ── 搜索模式 ──

// SearchMode 搜索模式
type SearchMode string

const (
	ModeSimple  SearchMode = "simple"  // 轻量摘要模式
	ModeWebpage SearchMode = "webpage" // 网页搜索模式
	ModeDepth   SearchMode = "depth"   // 深度研究模式
)

// ── 时间范围 ──

// TimeRange 时间范围过滤
type TimeRange string

const (
	TimeRangeNone  TimeRange = ""   // 不限时间
	TimeRangeDay   TimeRange = "1d" // 1天内
	TimeRangeWeek  TimeRange = "1w" // 1周内
	TimeRangeMonth TimeRange = "1m" // 1月内
	TimeRangeYear  TimeRange = "1y" // 1年内
)

// ParseTimeRange 从字符串解析时间范围
func ParseTimeRange(s string) TimeRange {
	switch s {
	case "1d", "day", "today":
		return TimeRangeDay
	case "1w", "week":
		return TimeRangeWeek
	case "1m", "month":
		return TimeRangeMonth
	case "1y", "year":
		return TimeRangeYear
	default:
		return TimeRangeNone
	}
}

// ── 搜索结果 ──

// SearchResult 单条搜索结果
type SearchResult struct {
	Title          string
	URL            string
	Snippet        string
	IsOfficial     bool    // 是否为官方网站（由搜索引擎标记）
	AuthorityScore float64 // 站点权威性评分 (0.0~1.0)，由 ScoreDomainAuthority 计算
}

// Searcher 搜索引擎接口
type Searcher interface {
	Search(query string, limit int) ([]SearchResult, error)
	SearchRaw(query string, limit int) ([]SearchResult, error)
	SearchWithTimeRange(query string, limit int, timeRange TimeRange) ([]SearchResult, error)
	Name() string
}

// ── 配置类型 ──

// Config 网络检索子系统完整配置
type Config struct {
	Simple  SimpleConfig
	Webpage WebpageConfig
	Depth   DepthConfig
	HTTP    HTTPConfig
}

// SimpleConfig 轻量摘要配置
type SimpleConfig struct {
	MaxResults int
	TimeRange  TimeRange // 默认时间范围（空表示不限）
}

// WebpageConfig 网页搜索配置
type WebpageConfig struct {
	MaxResults            int
	FetchContent          bool
	FetchTimeout          int
	MaxContentLength      int
	EnableDomainDiscovery bool
}

// DepthConfig 深度研究配置
type DepthConfig struct {
	Enabled       bool
	MaxSubQueries int
	MaxGapRounds  int
}

// HTTPConfig HTTP 客户端配置
type HTTPConfig struct {
	Timeout      time.Duration
	UserAgent    string
	MaxRetries   int
	RetryBackoff time.Duration
}

// ── LLM 相关类型 ──

// ChatMessage 聊天消息
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest OpenAI v1 Chat Completion 请求
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

// chatResponse OpenAI v1 Chat Completion 响应
type chatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// errorResponse OpenAI v1 错误响应
type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Provider LLM 提供者接口
type Provider interface {
	Chat(messages []ChatMessage) (string, error)
}

// VisionProvider 图片识别提供者接口
type VisionProvider interface {
	AnalyzeImage(imageURL string, visionRules string) (string, error)
}

// MemoryProvider 记忆查询接口
type MemoryProvider interface {
	Query(query string) (string, error)
}

// ── 搜索引擎类型 ──

// BingSearcher 必应中文搜索
type BingSearcher struct {
	client  *http.Client
	httpCfg HTTPConfig
}

// BaiduSearcher 百度搜索
type BaiduSearcher struct {
	client            *http.Client
	httpCfg           HTTPConfig
	cookieWarmed      bool
	browserRenderer   BrowserRenderer
	captchaDetected   bool
	captchaDetectedAt time.Time
}

// SogouSearcher 搜狗搜索
type SogouSearcher struct {
	client  *http.Client
	httpCfg HTTPConfig
}

// DuckDuckGoSearcher DuckDuckGo Lite 搜索
type DuckDuckGoSearcher struct {
	client  *http.Client
	httpCfg HTTPConfig
}

// ── 搜索器类型 ──

// SimpleSearcher 轻量摘要搜索器：多引擎回退
type SimpleSearcher struct {
	baidu      Searcher
	sogou      Searcher
	bing       Searcher
	ddg        Searcher
	maxResults int
	timeRange  TimeRange // 默认时间范围
	health     *EngineHealth
	debugLog   func(format string, args ...interface{})
}

// WebpageSearcher 网页搜索器：搜索 + 网页内容抓取 + LLM 总结
type WebpageSearcher struct {
	simple          *SimpleSearcher
	llmProvider     Provider
	cfg             WebpageConfig
	httpClient      *http.Client
	browserRenderer BrowserRenderer
	debugLog        func(format string, args ...interface{})
	knowledge       *SearchKnowledge
}

// ── 深度研究类型 ──

// ResearchData 深度搜索采集的结构化研究数据
type ResearchData struct {
	OriginalQuery string
	SubQueries    []SubQueryResult
}

// SubQueryResult 单个子问题的搜索结果
type SubQueryResult struct {
	Query   string
	Results []SearchResult
	Error   error
}

// 深度研究预算常量
const (
	depthMaxResultsPerSub     = 15
	depthMaxSnippetLen        = 300
	depthMaxFetchedContentLen = 1500
	depthMaxPromptChars       = 12000
	depthMaxOutputChars       = 3000
	depthMaxSubResultsChars   = 8000
)

// 网页搜索预算常量
const (
	webpageMaxFetchResults    = 30
	webpageMaxTotalContentLen = 8000
	webpageMaxPerPageLen      = 1500
	webpageMaxLLMOutputLen    = 1500
	webpageMaxPromptChars     = 8000
	webpageMaxFallbackChars   = 4000
)

// 链接处理预算常量
const (
	linkMaxRawContent  = 2000
	linkMaxTotalBudget = 3000
	linkFetchTimeout   = 10
)

// subResult 子问题搜索结果
type subResult struct {
	Query   string
	Results []SearchResult
	Error   error
}

// DepthSearcher 深度研究：子问题拆解 + 并行搜索 + 内容抓取 + 综合报告
type DepthSearcher struct {
	simple          *SimpleSearcher
	llmProvider     Provider
	cfg             DepthConfig
	httpClient      *http.Client
	userAgent       string
	browserRenderer BrowserRenderer
	debugLog        func(format string, args ...interface{})
	knowledge       *SearchKnowledge
}

// ── 子系统入口 ──

// DownloadFunc 下载回调函数类型
type DownloadFunc func(url string, groupID string) (filePath string, err error)

// System 网络检索子系统入口
type System struct {
	cfg             Config
	simple          *SimpleSearcher
	webpage         *WebpageSearcher
	depth           *DepthSearcher
	knowledge       *SearchKnowledge
	llmProvider     Provider
	memProvider     MemoryProvider
	visionProvider  VisionProvider
	downloadFunc    DownloadFunc
	downloadGroupID string
	health          *EngineHealth
	browserRenderer BrowserRenderer
	browserMu       sync.Mutex
	reranker        *Reranker // LLM 重排序器
	DebugLog        func(format string, args ...interface{})
}

// ── 链接处理类型 ──

// LinkType 链接类型
type LinkType int

const (
	LinkWebpage LinkType = iota
	LinkImage
	LinkDownload
)

// LinkResult 单条链接处理结果
type LinkResult struct {
	URL     string
	Type    LinkType
	Summary string
}

// OpenAIProvider 基于 OpenAI v1 协议的 LLM 客户端
type OpenAIProvider struct {
	baseURL     string
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	client      *http.Client
}
