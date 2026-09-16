package StarLTP

// ==== 类型定义集中区 ====

import (
	"regexp"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// jsFunc goja 回调引用（在对应插件事件循环内调用）。
type jsFunc = goja.Value

// ==== 插件（每插件独立 goja 沙箱） ====

// plugin 一个 LTP9 包对应的运行实例。持有独立事件循环（自带定时器）与顶层全局绑定。
type plugin struct {
	ID         string // 插件 ID
	DirName    string // 包目录名
	Title      string
	Root       string
	MainPath   string // execute.js
	ConfigPath string // config.yaml
	DataDir    string // data/
	KeyPath    string // permissions.key

	config  map[string]any  // config.yaml 注入的配置（config.read/write 读写）
	granted map[string]bool // 经 permissions.key 代码哈希解密校验通过的 allow-* 清单

	loop *eventloop.EventLoop // 每插件独立事件循环（goja.New + 自带定时器）
	mu   sync.Mutex           // 串行化同一插件全部 JS 执行

	loaded  bool
	loadErr string

	// 订阅 id 单调计数器（事件订阅器 + 广播订阅共用）
	subSeq int
	// 事件订阅器：topic → 订阅器组（可重复订阅）
	events map[string][]*eventSub
	// 导出函数：fnName → 处理器（供 callFunction 跨包调用）
	exports map[string]jsFunc
	// 广播订阅器（signal.subscribe）
	signalSubs []*eventSub

	// 指令注册：指令名（含别名）→ 指令项（command.register）
	commands map[string]*commandEntry
	// 异步子任务：taskId → 任务状态（async.run）
	asyncMu    sync.Mutex
	asyncSeq   int
	asyncTasks map[int]*asyncTask
}

// commandEntry 单个指令项（command.register 注册，引擎侧经 Command/CommandAll 触发）。
type commandEntry struct {
	name    string
	re      *regexp.Regexp
	handler jsFunc
	aliases []string
}

// asyncTask 单个异步子任务状态（async.run 创建）。
type asyncTask struct {
	id        int
	status    string // running / done / timeout / error
	progress  any
	data      any
	fn        jsFunc
	createdAt time.Time // 创建时刻（清理已终结的过期任务记录用）
}

// eventSub 单个事件订阅项。
type eventSub struct {
	topic    string
	orderID  int // 订阅 id（插件内单调递增，同时作为时间顺序依据）
	handler  jsFunc
	priority int // 优先级（0 最高；priorityUnset 表示未设置，按时间顺序排列在设置者之后）
}

// ==== 引擎管理器 ====

// engine LTP9 引擎管理器。
type engine struct {
	mu      sync.RWMutex
	plugins map[string]*plugin // 插件 ID → 插件
	root    string             // LTP9 包根目录
	running bool

	// reconcileStop 关闭对账循环（shutdown 时关闭；nil 表示循环未启动/已停止）
	reconcileStop chan struct{}

	// outbound 由外部 Go 程序注入：插件产生的单向通报/事件发布（signal / event.publish → Go 侧）。
	outbound func(topic string, payload any)
}

// ==== 公开接口结果类型 ====

// Outcome 单个插件对一次事件/调用的处理结果。
type Outcome struct {
	PluginID string `json:"plugin_id"`
	Error    string `json:"error,omitempty"`
	Handled  bool   `json:"handled"`
	Result   any    `json:"result,omitempty"` // 订阅回调返回对象（intercept/modifiedData/cancel/return…）
}

// EmitSummary 一次事件分发的汇总。
type EmitSummary struct {
	Subscribed  int  `json:"subscribed"`
	Errored     int  `json:"errored"`
	Intercepted bool `json:"intercepted"` // 任一订阅器拦截，短路后续调用
	Canceled    bool `json:"canceled"`    // 任一订阅器撤回事件，不再派发
}

// EmitResult 事件分发的返回（客户端直接调用引擎功能的结果）。
type EmitResult struct {
	Topic    string      `json:"topic"`
	Outcomes []Outcome   `json:"outcomes"`
	Summary  EmitSummary `json:"summary"`
	// Data 沿订阅器 modifiedData 链更新后的最终负载；无插件改写时等于原始 payload。
	Data any `json:"data,omitempty"`
	// Modified 是否有任一订阅器通过 modifiedData 改写了负载（供调用方决定是否采用处理结果）。
	Modified bool `json:"modified"`
	// Returned 是否有任一订阅器通过 return 回传了业务结果（供调用方判断是否采用 Return）。
	Returned bool `json:"returned"`
	// Return 订阅器经 return 回传的业务结果；同主题多订阅器时以最后回传者为准，无回传时为 nil。
	// 与 Modified 的区别：modifiedData 改写的是供下游订阅器读取的事件负载，
	// return 是供事件发起方（客户端）消费的本次事件业务结果。
	Return any `json:"return,omitempty"`
}

// PluginState 插件状态（供外部 Go 程序枚举 / 前端引擎管理器填充动态选项）。
type PluginState struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Loaded  bool     `json:"loaded"`
	Error   string   `json:"error,omitempty"`
	Granted []string `json:"granted,omitempty"`
	// Events 已订阅的事件主题（引擎侧注册的真实信息，供前端下拉填充）
	Events []string `json:"events,omitempty"`
	// Exports 导出的函数名（exportFunction 注册的真实信息，供前端下拉填充）
	Exports []string `json:"exports,omitempty"`
}

// rwResult 沙箱 API 的统一返回（Go → JS）。
type rwResult struct {
	Success bool   `json:"success"`
	Text    string `json:"text,omitempty"`
	Error   string `json:"error,omitempty"`
	// ToolCalls agent.chat 响应中的工具调用（OpenAI 兼容 tool_calls 原样透传，供 AtoA 接头）
	ToolCalls []any `json:"tool_calls,omitempty"`
}
