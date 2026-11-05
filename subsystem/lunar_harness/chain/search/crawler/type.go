package crawler

// type.go 全部类型定义（结构体、接口、别名）集中于此。

import (
	"time"
)

// Config 网络搜索配置（engine/config 统一解析 web_search 节后的运行期快照）
type Config struct {
	SearchModel     string // 检索词提炼与统一摘要使用的模型名（固定 light_reasoning，由 engine/config 常量给出）
	MaxPages        int    // 页面条目总数：首页（浏览器引擎页）恒占 1 条，其余为同时打开的详情页
	MaxResults      int    // 结果页元素提取条数上限
	SummaryMaxChars int    // 统一摘要的硬切断字符数（≤4096）
	MaxRunSeconds   int    // 单次运行的总时长软上限（秒），超时后以已采集信息收尾
	CacheEnabled    *bool  // 是否启用「近期已访问」缓存（统一句柄库 url_master）
	CacheTTLDays    int    // 近期访问判定窗口（天），窗口内的页面跳过不重复访问

	TextTotalChars int // 全部页面正文的总提取硬上限（由所有页面均分，默认 30000）
	ScrollSettleMS int // 滚动走查中每步之间的等待毫秒数（默认 120，中途只滚动不提取）
	ScrollMaxSteps int // 单页滚动步数上限（默认 40）
}

// DetailCount 需要打开的详情页数量：首页恒占 1 条目，其余条目全部给详情页
func (c Config) DetailCount() int {
	n := max(c.MaxPages-1, 0)
	return n
}

// PerPageChars 单页正文提取配额：总提取上限由所有参与页面（首页 + 详情页）均分
func (c Config) PerPageChars(pageCount int) int {
	if pageCount < 1 {
		pageCount = 1
	}
	return c.TextTotalChars / pageCount
}

// PageCache 页面「近期已访问」标记（SQLite，读写走 engine/knowledge 接口）。
// 零值/nil 安全：fresh/put/close 均兼容 nil 接收者，缓存不可用时调用方无需分支处理。
type PageCache struct {
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

// CrawlResult 一次抓取流程的产出（不含任何模型调用）
type CrawlResult struct {
	Engine      string        // 胜出引擎标识（bing/baidu/sogou）
	EngineName  string        // 胜出引擎中文名（必应/百度/搜狗）
	SearchURL   string        // 实际使用的检索页 URL
	Extract     PageExtract   // 结果页提取（已过滤字典站并去重）
	SearchText  string        // 搜索页完整滚动后的正文
	Pages       []PageCapture // 各详情页的原始提取
	Interrupted bool          // 是否在中途被关闭
}

// PageCapture 单个详情页并发提取到的原始内容
type PageCapture struct {
	Title     string // 页面标题
	URL       string // 页面最终地址（跳转后）
	Domain    string // 域名（如 example.com）
	Text      string // 正文（已按单页均分配额截断）
	TextChars int    // 正文总字数（截断前）
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
