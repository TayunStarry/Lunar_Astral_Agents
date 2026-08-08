package lunar_chromedp

import "time"

// =============================================================================
// 搜索智能体 — 类型定义
// =============================================================================

// SearchConfig 搜索智能体初始化配置
// 调用方在 InitSearch 时一次性传入，后续 Search 调用复用此配置
type SearchConfig struct {
	// 多模态模型配置（用于视觉理解、摘要、报告生成）
	MultimodalURL   string // 模型 API 地址（如 http://127.0.0.1:8080/v1）
	MultimodalName  string // 模型名称（如 system-multimodal）
	MultimodalKey   string // API 密钥（可为空）
	// 嵌入模型配置（用于记忆库检索、关键词去重）
	EmbeddingURL  string // 嵌入服务 API 地址
	EmbeddingName string // 嵌入模型名称（如 system-embedding）
	EmbeddingKey  string // 嵌入服务 API 密钥（可为空）
	// 上下文控制
	MaxContextTokens int // 单次 AI 调用最大上下文长度，默认 16384
}

// SearchReport 搜索最终报告
type SearchReport struct {
	Query       string `json:"query"`        // 用户原始查询
	Answer      string `json:"answer"`       // AI 生成的答案
	FromMemory  bool   `json:"from_memory"`  // 是否直接来自记忆库（跳过了网络搜索）
	UsedSources []string `json:"used_sources,omitempty"` // 引用的来源 URL
	SearchRounds int   `json:"search_rounds"` // 实际执行的搜索轮次
	GeneratedAt time.Time `json:"generated_at"` // 报告生成时间
}

// SearchResult 搜索引擎单条搜索结果
type SearchResult struct {
	Title   string // 结果标题
	URL     string // 目标 URL
	Snippet string // 摘要文本
	Engine  string // 来源搜索引擎（bing/baidu/sogou）
}

// PageContent 网页提取内容
type PageContent struct {
	URL         string            // 页面 URL
	TextContent string            // DOM 提取并清洗后的文本
	TextLength  int               // 清洗后文本长度（字符数）
	ContentType string            // "text"（文本密集型）或 "visual"（视觉主导型）
	Screenshots []PageScreenshot  // 截图列表（仅 visual 类型，text 类型为空）
}

// PageScreenshot 单页截图
type PageScreenshot struct {
	ImageData  []byte // PNG 格式截图数据
	PageNumber int    // 页码（从 1 开始）
	Width      int    // 截图宽度
	Height     int    // 截图高度
}

// BrowserHealth 浏览器健康状态
type BrowserHealth struct {
	IsRunning  bool   // 浏览器进程是否存活
	MemMB      uint64 // 内存占用（MB）
	CPUPercent float64 // CPU 占用百分比
	QueryCount int    // 自上次重启以来的查询次数
	Healthy    bool   // 综合判定是否健康
}

// MemorySearchRecord 存入 search_memory 的结构化记录
// 格式：问题 → 搜索关键词 → 关键发现 → 最终答案
type MemorySearchRecord struct {
	Question     string   `json:"question"`      // 用户原始问题
	Keywords     []string `json:"keywords"`      // 使用的搜索关键词
	KeyFindings  string   `json:"key_findings"`  // 关键发现摘要
	Answer       string   `json:"answer"`        // 最终答案
	Timestamp    int64    `json:"timestamp"`     // Unix 时间戳
}

// ProgressEvent 搜索进度事件（用于终端日志）
type ProgressEvent struct {
	Phase   string // 阶段标识：memory_lookup / searching / extracting / summarizing / evaluating / deep_search / generating_report
	Message string // 进度描述
	Round   int    // 当前搜索轮次（从 1 开始）
	Total   int    // 总轮次（深度搜索阶段有效）
}
