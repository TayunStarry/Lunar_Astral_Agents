package websearch

import (
	"net/http"
	"time"
)

// ============================================================
// 搜索模式
// ============================================================

// SearchMode 搜索模式
type SearchMode string

const (
	// ModeSimple 轻量摘要模式
	ModeSimple SearchMode = "simple"
	// ModeWebpage 网页搜索模式
	ModeWebpage SearchMode = "webpage"
	// ModeDepth 深度研究模式
	ModeDepth SearchMode = "depth"
)

// ============================================================
// 搜索结果
// ============================================================

// SearchResult 单条搜索结果
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// ============================================================
// 搜索引擎接口
// ============================================================

// Searcher 搜索引擎接口
type Searcher interface {
	Search(query string, limit int) ([]SearchResult, error)
	SearchRaw(query string, limit int) ([]SearchResult, error) // 跳过预处理，原始搜索
	Name() string
}

// ============================================================
// 配置类型
// ============================================================

// Config 网络检索子系统完整配置
type Config struct {
	Simple  SimpleConfig
	Webpage WebpageConfig
	Depth   DepthConfig
	LLM     llmConfig
	HTTP    HTTPConfig
}

// SimpleConfig 轻量摘要配置
type SimpleConfig struct {
	MaxResults int
}

// WebpageConfig 网页搜索配置
type WebpageConfig struct {
	MaxResults       int
	FetchContent     bool
	FetchTimeout     int
	MaxContentLength int
}

// DepthConfig 深度研究配置（大会辩论模式）
type DepthConfig struct {
	Enabled       bool // 是否启用大会辩论深度搜索
	MaxRounds     int  // 最大辩论轮次 (默认3)
	MaxResults    int  // 每个子问题最大结果
	MaxSubQueries int  // 最大子问题数
}

// llmConfig OpenAI v1 协议兼容的 AI 模型配置
type llmConfig struct {
	BaseURL     string
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float64
}

// HTTPConfig HTTP 客户端配置
type HTTPConfig struct {
	Timeout   time.Duration
	UserAgent string
}

// ============================================================
// LLM 类型
// ============================================================

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

// VisionProvider 图片识别提供者接口（由调用方注入已有的模型池实例）
type VisionProvider interface {
	AnalyzeImage(imageURL string, visionRules string) (string, error)
}

// MemoryProvider 记忆查询接口（由项目层实现，供大会辩论使用）
type MemoryProvider interface {
	Query(query string) (string, error)
}

// ============================================================
// 搜索引擎类型
// ============================================================

// BingSearcher 使用必应中文搜索
type BingSearcher struct {
	client *http.Client
}

// BaiduSearcher 使用百度搜索
type BaiduSearcher struct {
	client *http.Client
}

// SogouSearcher 使用搜狗搜索
type SogouSearcher struct {
	client *http.Client
}

// DuckDuckGoSearcher 使用 DuckDuckGo Lite 搜索
type DuckDuckGoSearcher struct {
	client *http.Client
}

// ============================================================
// 轻量摘要
// ============================================================

// SimpleSearcher 轻量摘要搜索器：百度 → 搜狗 → Bing → DuckDuckGo 回退
type SimpleSearcher struct {
	baidu      Searcher
	sogou      Searcher
	bing       Searcher
	ddg        Searcher
	maxResults int
}

// ============================================================
// 网页搜索器
// ============================================================

// WebpageSearcher 网页搜索器：搜索 + 网页内容抓取 + LLM 总结
type WebpageSearcher struct {
	simple      *SimpleSearcher
	llmProvider Provider
	cfg         WebpageConfig
	httpClient  *http.Client
}

// ============================================================
// 深度研究
// ============================================================

// 深度研究预算
const (
	depthMaxResultsPerSub     = 15    // 每个子问题最多返回结果数
	depthMaxSnippetLen        = 300   // 每条Snippet最大字符数（中文需足够上下文）
	depthMaxFetchedContentLen = 1500  // 抓取的网页内容截断长度
	depthMaxPromptChars       = 12000 // generateReport prompt总预算
	depthMaxOutputChars       = 3000  // LLM报告输出最大字符数
	depthMaxSubResultsChars   = 8000  // 子问题结果注入generateReport的总预算
)

// 网页搜索预算
const (
	webpageMaxFetchResults    = 30   // 网页搜索最多抓取条数
	webpageMaxTotalContentLen = 8000 // 总内容预算：最多8000字符
	webpageMaxPerPageLen      = 1500 // 单页截断
	webpageMaxLLMOutputLen    = 1500 // LLM 总结输出上限
	webpageMaxPromptChars     = 8000 // prompt 总大小预算
	webpageMaxFallbackChars   = 4000 // 回退格式化截断上限
)

// 链接处理预算
const (
	linkMaxSummaryNoLLM = 200  // 无 LLM 时单链接摘要上限
	linkMaxSummaryLLM   = 300  // LLM 总结时单链接摘要上限
	linkMaxTotalBudget  = 3000 // 所有链接处理总字符预算
	linkFetchTimeout    = 10   // 单链接抓取超时（秒）
)

// subResult 子问题搜索结果（保留原始结果用于URL去重）
type subResult struct {
	Query   string
	Results []SearchResult // 原始结果，用于URL去重
	Error   error
}

// DepthSearcher 深度研究：子问题拆解 + 并行搜索 + 内容抓取 + 综合报告
type DepthSearcher struct {
	simple      *SimpleSearcher
	llmProvider Provider
	cfg         DepthConfig
	httpClient  *http.Client
	userAgent   string
}

// ============================================================
// 子系统入口
// ============================================================

// DownloadFunc 下载回调函数类型：接收URL和群组ID，返回下载后的文件路径
type DownloadFunc func(url string, groupID string) (filePath string, err error)

// System 网络检索子系统
type System struct {
	cfg             Config
	simple          *SimpleSearcher
	webpage         *WebpageSearcher
	depth           *DepthSearcher
	assembly        *Assembly
	llmProvider     Provider
	memProvider     MemoryProvider
	visionProvider  VisionProvider
	downloadFunc    DownloadFunc
	downloadGroupID string // 下载目标群组ID（由调用方在处理前设置）
}

// ============================================================
// 链接处理类型
// ============================================================

// LinkType 链接类型
type LinkType int

const (
	LinkWebpage  LinkType = iota // 网页链接
	LinkImage                    // 图片链接
	LinkDownload                 // 下载链接
)

// LinkResult 单条链接处理结果
type LinkResult struct {
	URL     string
	Type    LinkType
	Summary string // 替换文本，失败时为空
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

// ============================================================
// 大会辩论类型定义
// ============================================================

// DelegateRole 辩论角色
type DelegateRole string

const (
	// RoleReformist 维新派（搜索派）：主张相信网络搜索结果
	RoleReformist DelegateRole = "reformist"
	// RoleConservative 守旧派（记忆派）：主张相信已有记忆/知识
	RoleConservative DelegateRole = "conservative"
	// RoleSupporter 赞同者：寻找双方论点的共同点
	RoleSupporter DelegateRole = "supporter"
	// RoleOpponent 反对者：挑双方论点的漏洞
	RoleOpponent DelegateRole = "opponent"
	// RoleSynthesizer 整合者：主持辩论，综合各方论点
	RoleSynthesizer DelegateRole = "synthesizer"
)

// DebateRound 一轮辩论记录
type DebateRound struct {
	Round         int
	ReformistA    string // 维新派A发言
	ConservativeA string // 守旧派A发言
	Supporter     string // 赞同者评论
	Opponent      string // 反对者挑刺
	ReformistB    string // 维新派B反驳/补充
	ConservativeB string // 守旧派B反驳/补充
	Verdict       string // 整合者本轮判断
	Converged     bool   // 本轮是否收敛
}

// AssemblyState 大会状态
type AssemblyState struct {
	OriginalQuery string
	Topics        []string // 拆解出的议题
	Rounds        []DebateRound
	CurrentRound  int
	Converged     bool
	SearchResults map[string][]SearchResult // 维新派搜索结果, key=议题
	MemoryResults map[string]string         // 守旧派记忆检索结果, key=议题
}

// Assembly 大会辩论系统
type Assembly struct {
	simple      *SimpleSearcher
	llmProvider Provider
	memProvider MemoryProvider
	cfg         DepthConfig
}
