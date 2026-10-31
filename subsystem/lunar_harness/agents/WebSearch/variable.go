package WebSearch

// variable.go 全局变量与常量集中声明（类型定义见 type.go）。

import (
	"net/http"
	"regexp"
	"sync"
	"time"
)

const (
	// webSearchPagesHardCap 页面条目总数的硬上限（含首页 1 条）
	webSearchPagesHardCap = 10
	// webSearchLoopRetries 模型输出陷入重复时，单次调用的额外重试次数。
	// 网络搜索的每次调用都有兜底路径，重试一次仍失败就交给调用点降级。
	webSearchLoopRetries = 1
	// webSearchModelRole 网络搜索的实况角色标签（TUI 显示）
	webSearchModelRole = "网络搜索"
	// promptUserMarker 任务型提示词的「系统段 / 用户模板」分隔符
	promptUserMarker = "\n---user---\n"
	// readURLInstruction 指定页面提炼的默认指令（没有任何具体诉求时使用）
	readURLInstruction = "用中文概括这个页面的主要内容、关键结论与值得注意的信息；只依据页面正文，不要编造页面里没有的内容"
	// PipelineSearch 联网检索管线（兜底管线）
	PipelineSearch = "search"
	// PipelineWeather 天气查询管线（纯 API）
	PipelineWeather = "weather"
	// routeConfidenceMin 低于该把握时一律回落检索管线
	routeConfidenceMin = 0.6
	// routeQueryMaxRunes 检索词/城市参数的最大字符数
	routeQueryMaxRunes = 120
)

// routePipelines 结构化路由注册表：新增直连能力只需在此加一条，
// 提示词候选列表与合法性校验都从这里生成，不需要额外的正则或分支判断。
var routePipelines = []routePipeline{
	{
		ID:          PipelineSearch,
		Description: "联网检索：查资料、新闻、百科、网页内容或任何实时信息；query 填简洁检索词",
	},
	{
		ID:          PipelineWeather,
		Description: "天气查询：当前天气、气温、降水、预报、空气质量；args.city 填城市名（中文优先），确实不知道城市时留空按 IP 定位",
	},
}

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

	// 模型实况观察者：由 app 注入控制台的 StreamBegin/Delta/End，
	// 让 TUI「推理实况」能看到网络搜索的模型思考与输出。
	observerMu    sync.Mutex
	observerBegin func(role string)
	observerDelta func(role, kind, text string)
	observerEnd   func(role string)
	// observerMeter 模型调用计量回调：网络搜索的调用同样计入「运行状态」的模型计量
	observerMeter func(prompt, completion, cached int64, generation time.Duration, estimated bool)
)
