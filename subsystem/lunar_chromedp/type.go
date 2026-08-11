package lunar_chromedp

import "time"

// =============================================================================
// 搜索智能体 — 类型定义
// =============================================================================

// SearchConfig 搜索智能体初始化配置
// 模型配置（URL、模型名、API Key）已迁移至 config 模块（lunar_config.json）
// 调用方在 InitSearch 时仅需传入记忆库目录和上下文控制参数
type SearchConfig struct {
	// 记忆库存储目录
	MemoryDBDir string // 记忆库数据存储根目录，默认 "local_data/database/memory"
	// 上下文控制
	MaxContextTokens int // 单次 AI 调用最大上下文长度，默认 16384
}

// SearchReport 搜索最终报告
type SearchReport struct {
	Query        string    `json:"query"`                  // 用户原始查询
	Answer       string    `json:"answer"`                 // AI 生成的答案
	FromMemory   bool      `json:"from_memory"`            // 是否直接来自记忆库（跳过了网络搜索）
	UsedSources  []string  `json:"used_sources,omitempty"` // 引用的来源 URL
	SearchRounds int       `json:"search_rounds"`          // 实际执行的搜索轮次
	GeneratedAt  time.Time `json:"generated_at"`           // 报告生成时间
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
	URL         string           // 页面 URL
	TextContent string           // DOM 提取并清洗后的文本
	TextLength  int              // 清洗后文本长度（字符数）
	ContentType string           // "text"（文本密集型）或 "visual"（视觉主导型）
	Screenshots []PageScreenshot // 截图列表（仅 visual 类型，text 类型为空）
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
	IsRunning  bool    // 浏览器进程是否存活
	MemMB      uint64  // 内存占用（MB）
	CPUPercent float64 // CPU 占用百分比
	QueryCount int     // 自上次重启以来的查询次数
	Healthy    bool    // 综合判定是否健康
}

// MemorySearchRecord 存入 search_memory 的结构化记录
// 格式：问题 → 搜索关键词 → 关键发现 → 最终答案
type MemorySearchRecord struct {
	Question    string   `json:"question"`     // 用户原始问题
	Keywords    []string `json:"keywords"`     // 使用的搜索关键词
	KeyFindings string   `json:"key_findings"` // 关键发现摘要
	Answer      string   `json:"answer"`       // 最终答案
	Timestamp   int64    `json:"timestamp"`    // Unix 时间戳
}

// ProgressEvent 搜索进度事件（用于终端日志）
type ProgressEvent struct {
	Phase   string // 阶段标识：memory_lookup / searching / extracting / summarizing / evaluating / deep_search / generating_report
	Message string // 进度描述
	Round   int    // 当前搜索轮次（从 1 开始）
	Total   int    // 总轮次（深度搜索阶段有效）
}

// chatMessage OpenAI 兼容消息格式
type chatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string 或 []contentPart
}

// contentPart 多模态消息的内容部分
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

// imageURL 图片 URL（base64 data URI）
type imageURL struct {
	URL string `json:"url"`
}

// chatRequest OpenAI 兼容聊天请求
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream"`
}

// chatResponse OpenAI 兼容聊天响应
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// embeddingRequest 嵌入请求
type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// embeddingResponse 嵌入响应
type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// SearchAgent 搜索智能体主控制器
// 负责流程编排：记忆检索 → 网络搜索 → 深度搜索 → 报告生成 → 记忆存储
type SearchAgent struct {
	config SearchConfig

	// 已使用的搜索关键词（用于深度搜索去重）
	usedKeywords []string
	// 搜索过程中积累的摘要
	accumulatedSummaries []string
	// 搜索过程中积累的来源 URL
	accumulatedSources []string
}

// memoryEntry 记忆检索中间结果
type memoryEntry struct {
	Content    string
	Similarity float32
}

// cpuReading CPU 读数记录
type cpuReading struct {
	percent   float64
	timestamp time.Time
}
