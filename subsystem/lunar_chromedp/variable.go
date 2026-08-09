package lunar_chromedp

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"logger"

	"github.com/chromedp/chromedp"
)

// =============================================================================
// 搜索智能体 — 全局变量与常量
// =============================================================================

// 搜索流程常量
const (
	MaxSearchRounds         = 8                // 深度搜索最大轮次
	MaxScreenshotsPerPage   = 10               // 单页最大截图数
	TextHeavyThreshold      = 500              // 文本密集型判定阈值（字符数）
	MemorySimilarityMin     = 0.55             // 记忆库相似度最低阈值
	KeywordDedupThreshold   = 0.85             // 关键词去重余弦相似度阈值
	MaxContextTokensDefault = 16384            // 默认最大上下文 tokens
	QueryTimeout            = 30 * time.Second // 单次浏览器操作超时
	BrowserMaxMemMB         = 2048             // 浏览器内存上限（MB）
	BrowserMaxCPUPercent    = 80.0             // 浏览器 CPU 占用上限（%）
	BrowserCPUHighDuration  = 5 * time.Second  // CPU 持续高占用阈值
	SearchResultsPerQuery   = 5                // 每个关键词取前 5 条结果
	PageLoadTimeout         = 15 * time.Second // 页面加载超时
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

// chromedp 浏览器启动选项
func buildBrowserOpts() []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-translate", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("disable-features", "TranslateUI"),
		chromedp.WindowSize(1920, 1080),
	}

	if BrowserExecPath != "" {
		opts = append(opts, chromedp.ExecPath(BrowserExecPath))
	}

	return opts
}

// edgePaths Windows 上 Edge 浏览器的可能安装路径
var edgePaths = []string{
	`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
}

// detectBrowserPath 自动检测可用的浏览器
// 优先级：用户指定 > Edge > Chrome（默认）
func detectBrowserPath() string {
	if BrowserExecPath != "" {
		fmt.Printf("[%s] 使用用户指定的浏览器: %s\n", ModuleName, BrowserExecPath)
		return BrowserExecPath
	}

	// 尝试检测 Edge
	for _, p := range edgePaths {
		if _, err := os.Stat(p); err == nil {
			fmt.Printf("[%s] 自动检测到 Edge 浏览器: %s\n", ModuleName, p)
			return p
		}
	}

	// 未检测到，让 chromedp 使用默认 Chrome 查找逻辑
	fmt.Printf("[%s] 未检测到 Edge，使用默认浏览器查找逻辑\n", ModuleName)
	return ""
}

// logProgress 输出搜索进度到终端日志
// 根据阶段类型使用不同的 logger 层级：
//   - 正常阶段 → logger.SubInfo
//   - 警告/错误 → logger.SubWarn / logger.SubError
func logProgress(event ProgressEvent) {
	switch event.Phase {
	case "memory_lookup":
		logger.Info(ModuleName, "[记忆检索] %s", event.Message)
	case "searching":
		logger.SubInfo(ModuleName, "搜索", "[轮次 %d/%d] %s", event.Round, event.Total, event.Message)
	case "extracting":
		logger.SubInfo(ModuleName, "提取", "%s", event.Message)
	case "summarizing":
		logger.SubInfo(ModuleName, "摘要", "%s", event.Message)
	case "evaluating":
		logger.Info(ModuleName, "[充分性评估] %s", event.Message)
	case "deep_search":
		logger.SubInfo(ModuleName, "深度搜索", "[轮次 %d/%d] %s", event.Round, event.Total, event.Message)
	case "generating_report":
		logger.Info(ModuleName, "[报告生成] %s", event.Message)
	case "warning":
		logger.Warn(ModuleName, "%s", event.Message)
	case "error":
		logger.Error(ModuleName, "%s", event.Message)
	default:
		logger.Info(ModuleName, "%s", event.Message)
	}
}
