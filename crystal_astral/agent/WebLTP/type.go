package WebLTP

import (
	"database/sql"
	"time"
)

// reportInput 报告汇编输入
type reportInput struct {
	Instruction string      // 指令
	Query       string      // 检索词
	EngineName  string      // 胜出引擎中文名（必应/百度/搜狗，报告叙述用）
	Engine      PageExtract // 搜索引擎提取结果
	EngineShot  string      // 结果页首屏截图 dataURL（供视觉印证，可为空）
	Pages       []PageIntel // 每个结果页的情报
	Screenshots int         // 总截图张数（包含首屏截图）
	SaveDir     string      // 保存目录（相对 LocalDir）
	Interrupted bool        // 是否在中途被关闭
	Elapsed     string      // 总耗时（秒）
}

// pageCacheEntry 缓存条目
type pageCacheEntry struct {
	Title     string    // 结果页标题
	Domain    string    // 域名（如 example.com）
	Summary   string    // 已按配置硬切断（≤4096 字符）
	QueryHint string    // 用于提示用户（如 "请输入检索词"）
	FetchedAt time.Time // 采集时间（UTC）
}

// PageCache 页面摘要缓存（SQLite）。零值/nil 安全：get/put/close 均兼容 nil 接收者，
// 缓存不可用时调用方无需分支处理。
type PageCache struct {
	db  *sql.DB       // SQLite 数据库连接， nil 时缓存不可用
	ttl time.Duration // 缓存有效期（天）
}

// shotSaver 截图落盘器（save_screenshots=false 时仅计数）
type shotSaver struct {
	enabled bool     // 是否把滚动截图保存到本地目录（false 时截图仅用于计数，不落盘）
	dir     string   // 截图保存目录（相对 LocalDir）
	count   int      // 已保存截图张数
	files   []string // 已保存截图文件名
}

// scrollState 一次滚动后的页面状态
type scrollState struct {
	Before   float64 `json:"before"`   // 滚动前的滚动位置（像素）
	After    float64 `json:"after"`    // 滚动后的滚动位置（像素）
	AtBottom bool    `json:"atBottom"` // 是否到达页面底部
	Height   float64 `json:"height"`   // 页面高度（像素）
}

// Config 网络搜索配置（lunar_config.json 的 web_search 字段，缺失字段用默认值）
type Config struct {
	SaveScreenshots       bool   `json:"save_screenshots"`        // 是否把滚动截图保存到本地目录（false 时截图仅用于计数，不落盘）
	ScreenshotDir         string `json:"screenshot_dir"`          // 截图保存目录（相对 LocalDir）
	MaxPages              int    `json:"max_pages"`               // 依次进入的结果页数量上限（硬上限 10）
	MaxResults            int    `json:"max_results"`             // 结果页元素提取条数上限
	ResultsScrollCaptures int    `json:"results_scroll_captures"` // 搜索结果页的滚动截图张数上限
	PageScrollCaptures    int    `json:"page_scroll_captures"`    // 单个结果页的滚动截图张数上限
	SummaryMaxChars       int    `json:"summary_max_chars"`       // 单页情报摘要的硬切断字符数（≤4096）
	MaxRunSeconds         int    `json:"max_run_seconds"`         // 单次运行的总时长软上限（秒），超时后以已采集信息收尾
	CacheEnabled          *bool  `json:"cache_enabled"`           // 是否启用页面摘要缓存（SQLite，knowledge.db 的 web_ltp_page_cache 表）。
	CacheTTLDays          int    `json:"cache_ttl_days"`          // 页面摘要缓存有效期（天），超过后重新访问网页并用新摘要覆写
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
	Engine    searchEngine // 胜出引擎
	URL       string       // 实际使用的检索页 URL（报告收尾回访用）
	Extract   PageExtract  // 结果页提取（已过滤字典站并去重）
	FirstShot string       // 结果页首屏截图 dataURL（可为空）
}

// PageIntel 单个结果页采集到的情报
type PageIntel struct {
	Title      string // 结果页标题
	URL        string // 结果页 URL
	Domain     string // 域名（如 example.com）
	Summary    string // 已按配置硬切断（≤4096 字符）
	Captures   int    // 滚动截图张数（≤100）
	Failed     string // 打开/提取失败原因（空表示成功）
	SummaryErr string // 模型摘要失败原因（此时摘要为文本前段兜底）
	Cached     bool   // 摘要来自本地缓存（未重新访问网页，Captures 为 0）
}

// chatMessage 聊天消息
type chatMessage struct {
	Role    string `json:"role"`    // 角色（user/assistant/system）
	Content any    `json:"content"` // 字符串或多媒体片段数组（图文混排）
}

// contentPart 多媒体内容片段（文本或图片）
type contentPart struct {
	Type     string    `json:"type"`                // 类型（text/image）
	Text     string    `json:"text,omitempty"`      // 文本内容（可选）
	ImageURL *imageURL `json:"image_url,omitempty"` // 图片 URL（可选）
}

// imageURL 图片 URL
type imageURL struct {
	URL string `json:"url"` // 图片 URL
}
