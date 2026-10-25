package WebSearch

// type.go 全部类型定义（结构体、接口、别名）集中于此。

import (
	"database/sql"
	"strings"
	"sync"
	"time"
)

// Config 网络搜索配置（engine/config 统一解析 web_search 节后的运行期快照）
type Config struct {
	SearchModel     string // 页面摘要使用的小模型名（默认 system-search，模型路径由 models.ini 定义）
	SwapModel       *bool  // 是否在搜索前卸载主多模态模型并加载搜索模型（默认开启）
	MaxPages        int    // 依次进入的结果页数量上限（硬上限 10）
	MaxResults      int    // 结果页元素提取条数上限
	SummaryMaxChars int    // 单页情报摘要的硬切断字符数（≤4096）
	MaxRunSeconds   int    // 单次运行的总时长软上限（秒），超时后以已采集信息收尾
	CacheEnabled    *bool  // 是否启用「近期已访问」缓存（SQLite，web.db 的 web_ltp_page_cache 表）
	CacheTTLDays    int    // 近期访问判定窗口（天），窗口内的页面跳过不重复访问

	TextChunkChars int // 单段摘要的字符数（默认 5000）
	TextMaxChars   int // 单页正文提取的硬上限字符数（默认 24000）
	TextMaxChunks  int // 单页最多参与摘要的文本段数（默认 4）
	ScrollSettleMS int // 每次滚动后的等待毫秒数（默认 350）
	ScrollMaxSteps int // 单页滚动步数上限（默认 40）
}

// reportInput 报告汇编输入
type reportInput struct {
	Instruction    string      // 指令
	Query          string      // 检索词
	EngineName     string      // 胜出引擎中文名（必应/百度/搜狗，报告叙述用）
	Engine         PageExtract // 搜索引擎提取结果
	SearchPageText string      // 回访搜索页后的正文（整体汇总依据）
	Pages          []PageIntel // 每个结果页的情报
	Interrupted    bool        // 是否在中途被关闭
	Elapsed        string      // 总耗时（秒）
}

// PageCache 页面「近期已访问」标记（SQLite）。
// 零值/nil 安全：fresh/put/close 均兼容 nil 接收者，缓存不可用时调用方无需分支处理。
type PageCache struct {
	db  *sql.DB       // SQLite 数据库连接，nil 时缓存不可用
	ttl time.Duration // 近期访问判定窗口
}

// stepState 一次「滚动 + 文本提取」后的页面状态
type stepState struct {
	Before    float64 `json:"before"`    // 滚动前的滚动位置（像素）
	After     float64 `json:"after"`     // 滚动后的滚动位置（像素）
	AtBottom  bool    `json:"atBottom"`  // 是否到达页面底部
	Height    float64 `json:"height"`    // 页面高度（像素）
	Viewport  float64 `json:"viewport"`  // 视口高度（像素）
	URL       string  `json:"url"`       // 当前页面地址
	Title     string  `json:"title"`     // 当前页面标题
	Text      string  `json:"text"`      // 正文（已按上限截断）
	TextChars int     `json:"textChars"` // 正文总字数（截断前）
}

// walkResult 一次自上而下滚动走查的结果
type walkResult struct {
	Steps     int     // 实际滚动步数
	AtBottom  bool    // 是否到达页面底部
	Height    float64 // 滚动结束时的页面高度
	Viewport  float64 // 视口高度
	URL       string  // 页面地址
	Title     string  // 页面标题
	Text      string  // 最终正文
	TextChars int     // 正文总字数（截断前）
}

// summaryJob 单段摘要任务
type summaryJob struct {
	index int    // 段序号（从 0 开始）
	chunk string // 该段正文
}

// pageSummarizer 页面摘要流水线：滚动过程中持续投递正文，后台逐段生成摘要，
// Finish 时等待全部摘要完成（页面保持在底部不动）。
type pageSummarizer struct {
	query      string          // 检索意图
	hit        SearchHit       // 结果条目（标题/地址）
	profile    summaryProfile  // 摘要档位
	chunkChars int             // 单段字符数
	maxChunks  int             // 最多段数
	jobs       chan summaryJob // 摘要任务队列（容量 = maxChunks，投递永不阻塞）
	done       chan struct{}   // 后台摘要协程结束信号
	mu         sync.Mutex      // 保护 pending/consumed/dispatched
	pending    strings.Builder // 尚未达到一段长度的正文
	consumed   int             // 已投递的正文游标（字符数）
	dispatched int             // 已投递的段数
	results    []string        // 各段摘要（按段序号落位）
	firstErr   error           // 首个摘要失败原因
}

// searchEngine 单个搜索引擎定义
type searchEngine struct {
	name      string // 引擎标识（日志与报告用）
	searchURL string // 检索页 URL 模板（查询词经 QueryEscape 拼接）
	container string // 结果容器选择器（querySelectorAll，逗号分隔多个候选）
	link      string // 容器内标题链接选择器
	snippet   string // 容器内摘要选择器（querySelector 逗号分隔候选，取首个命中）
}

// searchOutcome 一次降级链搜索的胜出结果
type searchOutcome struct {
	Engine     searchEngine // 胜出引擎
	URL        string       // 实际使用的检索页 URL（报告收尾回访用）
	Extract    PageExtract  // 结果页提取（已过滤字典站并去重）
	SearchText string       // 搜索页完整滚动后的正文
}

// PageIntel 单个结果页采集到的情报
type PageIntel struct {
	Title     string // 结果页标题
	URL       string // 结果页 URL
	Domain    string // 域名（如 example.com）
	Summary   string // 已按配置硬切断（≤4096 字符）
	TextChars int    // 正文总字数（截断前）
	Chunks    int    // 参与摘要的文本段数
}

// SearchHit 搜索结果条目
type SearchHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

// PageExtract 页面提取结果
type PageExtract struct {
	Kind      string      `json:"kind"`
	URL       string      `json:"url"`
	Title     string      `json:"title"`
	Results   []SearchHit `json:"results,omitempty"`
	Text      string      `json:"text"`
	TextChars int         `json:"textChars"` // 正文总字数（截断前）
}

// chatMessage 聊天消息
type chatMessage struct {
	Role    string `json:"role"`    // 角色（user/assistant/system）
	Content any    `json:"content"` // 字符串内容
}

// chatOptions 单次模型调用选项：模型覆盖、采样参数、输出上限与实况开关
type chatOptions struct {
	Model         string   // 模型名（空 = 主多模态模型）
	Temperature   *float64 // 温度（nil = 沿用服务端默认）
	TopP          *float64 // 核采样（nil = 沿用服务端默认）
	RepeatPenalty *float64 // 重复惩罚（nil = 沿用服务端默认）
	MaxTokens     int      // 最大输出 token（0 = 不限制）
	Silent        bool     // 是否跳过 TUI 实况（预热等内部调用）
}

// summaryProfile 摘要链路的模型与分块策略
type summaryProfile struct {
	Model      string // 模型名（空 = 主多模态模型）
	ChunkChars int    // 单块字符数
	MaxChunks  int    // 单页最多参与摘要的文本段数
	Merge      bool   // 多段时是否再用模型合并（小模型链路直接拼接，节省一次调用）
}

// modelUsage OpenAI 兼容响应中的 token 用量
type modelUsage struct {
	Prompt     int64 `json:"prompt_tokens"`
	Completion int64 `json:"completion_tokens"`
}
