package AgentSearch

import (
	"context"
	"net/http"
	"regexp"
	"sync"
	"time"
)

// =============================================================================
// 搜索智能体 — 全局变量与常量
// =============================================================================

// 搜索流程常量
const (
	MaxSearchRounds            = 8                // 深度搜索最大轮次
	MaxScreenshotsPerPage      = 6                // 单页最大截图数（每帧一次滚动，降载至 6 帧）
	TextHeavyThreshold         = 500              // 文本密集型判定阈值（字符数）
	MemorySimilarityMin        = 0.55             // 记忆库相似度最低阈值
	MemoryDirectAnswerMin      = 0.72             // 直接复用记忆答案的最低相似度阈值（低于则继续网络搜索）
	KeywordDedupThreshold      = 0.85             // 关键词去重余弦相似度阈值
	MaxContextTokensDefault    = 16384            // 默认最大上下文 tokens
	QueryTimeout               = 30 * time.Second // 单次浏览器操作超时
	BrowserMaxMemMB            = 2048             // 浏览器内存上限（MB）
	BrowserMaxCPUPercent       = 80.0             // 浏览器 CPU 占用上限（%）
	BrowserCPUHighDuration     = 5 * time.Second  // CPU 持续高占用阈值
	SearchResultsPerQuery      = 5                // 每个关键词取前 5 条结果
	QuickSearchResultsPerQuery = 5                // 快速搜索每个关键词取前 5 条结果
	SingleSearchResults        = 10               // 统一搜索模式：每轮进入搜索引擎并提取的前 N 条链接（TopN）
	EmbedRelevanceThreshold    = 0.5              // 摘要与初始查询嵌入余弦相似度阈值（≥此值视为相关）
	PageLoadTimeout            = 15 * time.Second // 页面加载超时（搜索页）
	PageFastSkipTimeout        = 10 * time.Second // 单页内容提取超时：打不开直接跳过，不再重启重试
	MaxBrowserRetryAttempts    = 3                // 浏览器加载/操作失败时最多重启重试次数
)

// 字典网站关键词黑名单 — 搜索智能体具备字典能力，无需浪费 token 在字典网站
var dictionaryKeywords = []string{
	"字典", "词典", "辞典", "dictionary", "lexicon", "thesaurus",
}

// 搜索智能体模块标识
const ModuleName = "SearchAgent"

// 浏览器全局状态
var (
	browserCtx          context.Context    // chromedp 浏览器上下文
	browserCancel       context.CancelFunc // 浏览器取消函数
	browserAllocCancel  context.CancelFunc // 分配器取消函数
	browserQueryCount   int                // 自上次重启以来的查询次数
	browserMutex        sync.Mutex         // 浏览器操作互斥锁
	browserLaunched     bool               // 浏览器是否已启动
	browserJustLaunched bool               // 浏览器刚启动/重启，跳过首次健康检查
)

// 搜索配置（InitSearch 时设置）
var (
	activeConfig *SearchConfig
	configMutex  sync.RWMutex
)

// 查询队列锁（确保串行）
var (
	queryMutex sync.Mutex
)

// 搜索引擎 URL 模板
var searchEngineURLs = map[string]string{
	"bing":  "https://www.bing.com/search?q=",
	"baidu": "https://www.baidu.com/s?wd=",
	"sogou": "https://www.sogou.com/web?query=",
}

// 搜索引擎降级顺序
var engineFallbackOrder = []string{"bing", "baidu", "sogou"}

// 浏览器可执行文件路径（空字符串表示自动检测）
var BrowserExecPath string

// edgePaths Windows 上 Edge 浏览器的可能安装路径
var edgePaths = []string{
	`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
}

// =============================================================================
// 搜索智能体全局实例
// =============================================================================

var (
	searchAgent     *SearchAgent
	searchAgentMu   sync.Mutex
	searchAgentInit bool
)

// =============================================================================
// AI 调用钩子（由 ai.go 在 init 时注册）
// =============================================================================

var (
	// aiCall 通用 AI 调用：发送 prompt，返回文本响应
	aiCall func(systemPrompt string, userPrompt string, images [][]byte) (string, error)

	// aiJudgeMemory 判定记忆库内容是否足以回答用户问题
	aiJudgeMemory func(memoryContext string, query string) (sufficient bool, timeSensitive bool, err error)

	// aiGenerateKeywords 将自然语言查询转化为结构化搜索关键词
	aiGenerateKeywords func(query string) ([]string, error)

	// aiGenerateDeepKeywords 基于已有上下文生成新的深度搜索关键词
	aiGenerateDeepKeywords func(query string, accumulatedSummaries string, usedKeywords []string) ([]string, error)

	// aiSummarizeContent 对网页内容进行摘要（文本或截图）
	aiSummarizeContent func(content string, screenshots [][]byte) (string, error)

	// aiEvaluateSufficiency 评估当前积累的信息是否足以回答用户问题
	aiEvaluateSufficiency func(query string, accumulatedSummaries string) (sufficient bool, reasoning string, err error)

	// aiGenerateReport 基于所有摘要生成最终搜索报告
	aiGenerateReport func(query string, summaries []string, sources []string) (string, error)

	// aiDecideSearchMode 判定是否使用快速视觉搜索模式
	// 返回: useQuickSearch（是否使用快速搜索）, reasoning（判定理由）, error
	aiDecideSearchMode func(query string) (useQuickSearch bool, reasoning string, err error)

	// aiSummarizeVisualContent 纯视觉摘要：仅基于截图生成页面内容摘要
	// 与 aiSummarizeContent 的区别：不接收文本内容，仅接收截图
	aiSummarizeVisualContent func(screenshots [][]byte) (summary string, err error)

	// aiEvaluateRelevance 判断单条网页摘要是否与用户查询直接相关
	// 返回: relevant（是否相关）, error
	aiEvaluateRelevance func(query string, itemText string) (relevant bool, err error)

	// aiJudgeSummary 判断单条页面摘要是否能够用于解答用户问题
	// 输入：原始问题、页面摘要、相关历史记忆（参考，未必相关）；返回: usable（能否解答）, error
	aiJudgeSummary func(query string, summary string, memoryReference string) (usable bool, err error)

	// aiEnhanceSearchText 基于原始问题推测真实意图，产出一条强化后的搜索文本
	// 输入：原始问题、第一轮失败覆盖的摘要、相关记忆提示；输出：强化搜索文本
	aiEnhanceSearchText func(query string, priorSummaries string, memoryHints string) (string, error)

	// aiExtractKeywords 从用户查询中提取核心完整实体名与搜索关键词数组
	// 输入：原始查询；输出：核心实体（用于标题初筛/摘要关键词比对）、关键词数组（用于拼接初始查询）
	aiExtractKeywords func(query string) (entities []string, keywords []string, err error)

	// aiJudgeComprehensive 综合判定多份网页摘要拼接后是否足以解答用户问题
	// 输入：原始问题、相关历史记忆（参考）、拼接后的有效摘要；输出：能否解答
	aiJudgeComprehensive func(query string, memoryReference string, summaries string) (bool, error)
)

// =============================================================================
// 记忆库钩子（由 memory.go 在 init 时注册）
// 模型配置从 config 模块（lunar_config.json）读取，不再通过参数传入
// =============================================================================

var (
	// memoryLookup 在 search_memory 集合中检索相似历史记录
	memoryLookup func(query string, topK int) ([]memoryEntry, error)

	// memoryStore 将搜索记录存入 search_memory
	memoryStore func(record MemorySearchRecord) error

	// memoryInitCollection 初始化 search_memory 集合
	memoryInitCollection func() error
)

// =============================================================================
// 浏览器健康检查钩子（由 monitor.go 在 init 时注册）
// =============================================================================

var (
	checkBrowserHealth func() BrowserHealth
)

// =============================================================================
// CPU 持续高占用追踪（monitor.go 使用）
// =============================================================================

var (
	cpuHighSince    time.Time
	cpuHighMu       sync.Mutex
	cpuWasHigh      bool
	cpuCheckHistory []cpuReading
)

const maxCPUHistory = 10 // 保留最近 10 次 CPU 检查记录

// =============================================================================
// 记忆库集合名称
// =============================================================================

const searchMemoryCollection = "search_memory"

// =============================================================================
// HTTP 客户端与类型
// =============================================================================

var (
	aiHTTPClient = &http.Client{Timeout: 120 * time.Second}
)

// =============================================================================
// 关键词去重缓存
// =============================================================================

var (
	keywordEmbedMu    sync.RWMutex
	keywordEmbedCache = make(map[string][]float32) // 关键词 → 嵌入向量
)

// =============================================================================
// DOM 文本清洗 — 噪声正则模式（browser.go 使用）
// =============================================================================

var noisePatterns = []*regexp.Regexp{
	// 导航栏
	regexp.MustCompile(`(?i)(首页|导航|菜单|Home|Menu|Navigation)`),
	// Cookie 横幅
	regexp.MustCompile(`(?i)(cookie|隐私政策|Privacy Policy|接受|cookie 政策)`),
	// 广告标识
	regexp.MustCompile(`(?i)(广告|AD|Sponsored|推广|Advertisement)`),
	// 社交分享文本
	regexp.MustCompile(`(?i)(分享到|Share|Tweet|转发|点赞)`),
	// 版权声明
	regexp.MustCompile(`(?i)(Copyright\s*©|版权所有|All Rights Reserved)`),
	// 登录/注册
	regexp.MustCompile(`(?i)(登录|注册|Sign\s*In|Sign\s*Up|Log\s*In|注册/登录)`),
}
