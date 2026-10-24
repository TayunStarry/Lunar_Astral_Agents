package WebLTP

// variable.go 全局变量与常量集中声明（类型定义见 type.go）。

import (
	"net/http"
	"regexp"
	"sync"
	"time"
)

const (
	// webSearchPagesHardCap 单次搜索进入结果页的硬上限
	webSearchPagesHardCap = 10
	// webSearchLoopRetries 模型输出陷入重复时，单次调用的额外重试次数。
	// 网络搜索的每次调用都有兜底路径，重试一次仍失败就交给调用点降级。
	webSearchLoopRetries = 1
	// webSearchModelRole 网络搜索的实况角色标签（TUI 显示）
	webSearchModelRole = "网络搜索"
	// promptUserMarker 任务型提示词的「系统段 / 用户模板」分隔符
	promptUserMarker = "\n---user---\n"
)

// engineFallbackOrder 搜索引擎降级顺序：bing → baidu → sogou
var engineFallbackOrder = []searchEngine{
	{
		name:      "bing",
		searchURL: "https://cn.bing.com/search?q=",
		container: "#b_results > li.b_algo",
		link:      "h2 a",
		snippet:   ".b_caption p, .b_caption, p",
	},
	{
		name:      "baidu",
		searchURL: "https://www.baidu.com/s?wd=",
		container: "#content_left > div",
		link:      "h3 a",
		snippet:   `.c-abstract, span[class*="content-right"], [class*="abstract"], p`,
	},
	{
		name:      "sogou",
		searchURL: "https://www.sogou.com/web?query=",
		container: "div.rb, div.vrwrap",
		link:      "h3 a",
		snippet:   ".str-text-info, .str_info, p",
	},
}

var (
	// runMu 串行化网络搜索运行（共享浏览器窗口资源）
	runMu sync.Mutex
	// pageCountRe 指令中的「前N个/页/条」数量要求
	pageCountRe = regexp.MustCompile(`前\s*([0-9]+|[０-９]+)\s*[个页条]`)
	// placeholderPattern 提示词占位符形态（{key}）
	placeholderPattern = regexp.MustCompile(`\{[A-Za-z0-9_\-]+\}`)
	// httpClient 模型调用 HTTP 客户端
	httpClient = &http.Client{Timeout: 120 * time.Second}

	// 主模型端点只解析一次
	modelOnce sync.Once
	modelName string
	modelURL  string
	modelKey  string

	// 模型实况观察者：由 app 注入控制台的 StreamBegin/Delta/End，
	// 让 TUI「核心智能 实况」能看到网络搜索的模型思考与输出。
	observerMu    sync.Mutex
	observerBegin func(role string)
	observerDelta func(role, kind, text string)
	observerEnd   func(role string)
	// observerMeter 模型调用计量回调：网络搜索的调用同样计入「运行状态」的模型计量
	observerMeter func(prompt, completion int64, generation time.Duration, estimated bool)
)
