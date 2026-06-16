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
	LLM     LLMConfig
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

// DepthConfig 深度研究配置
type DepthConfig struct {
	MaxResults    int
	MaxSubQueries int
}

// LLMConfig OpenAI v1 协议兼容的 AI 模型配置
type LLMConfig struct {
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

// ChatRequest OpenAI v1 Chat Completion 请求
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

// ChatResponse OpenAI v1 Chat Completion 响应
type ChatResponse struct {
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

// ErrorResponse OpenAI v1 错误响应
type ErrorResponse struct {
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

// ============================================================
// 搜索引擎类型
// ============================================================

// BingSearcher 使用必应中文搜索
type BingSearcher struct {
	client *http.Client
}

// DuckDuckGoSearcher 使用 DuckDuckGo Lite 搜索
type DuckDuckGoSearcher struct {
	client *http.Client
}

// ============================================================
// 轻量摘要
// ============================================================

// SimpleSearcher 轻量摘要搜索器：Bing + DuckDuckGo 回退
type SimpleSearcher struct {
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

// System 网络检索子系统
type System struct {
	cfg         Config
	simple      *SimpleSearcher
	webpage     *WebpageSearcher
	depth       *DepthSearcher
	llmProvider Provider
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
