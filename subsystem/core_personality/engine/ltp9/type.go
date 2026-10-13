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

// ==== 蓝图执行器（调度循环自前端迁移，节点执行复用 execTestAction / Probe*） ====

// bpNodeSpec 蓝图节点定义（ltp9/bp_run 入站 JSON 直解）。
type bpNodeSpec struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Params  map[string]any `json:"params"`
	Enabled *bool          `json:"enabled"` // nil = 启用（对齐前端 enabled !== false）
}

// bpPortRef 连线端点。
type bpPortRef struct {
	Node string `json:"node"`
	Port string `json:"port"`
}

// bpLinkSpec 蓝图连线定义。
type bpLinkSpec struct {
	ID   string    `json:"id"`
	From bpPortRef `json:"from"`
	To   bpPortRef `json:"to"`
}

// bpGraph 入站图数据（复合节点经 params.subgraph 原样携带，由执行器扁平化）。
type bpGraph struct {
	Nodes []bpNodeSpec `json:"nodes"`
	Links []bpLinkSpec `json:"links"`
}

// bpRunRequest ltp9/bp_run 入站：mode=all 运行整图；mode=single 仅执行 node_id 指定节点（不向后续传播）。
type bpRunRequest struct {
	Mode   string   `json:"mode"`
	NodeID string   `json:"node_id"`
	Graph  *bpGraph `json:"graph"`
}

// bpNodeRun 节点运行时状态（扁平命名空间：主图 + 复合重映射节点同处一表）。
type bpNodeRun struct {
	spec bpNodeSpec
	mu   sync.Mutex // 同节点多次激活串行化（对齐前端 _queue Promise 链）

	comp *bpCompState // 非 nil 表示本节点由该复合实例派生（完成判定计数用）

	outMu   sync.Mutex     // 保护 primary/multi/pop/status（复合收尾读取内部节点输出）
	primary any            // outValue 主输出
	multi   map[string]any // outValues 多输出（extract）
	pop     map[string]any // 显示类节点气泡（display/image_display；复合汇总 multi）
	status  string         // 最近一次执行状态（ok/err/skip；复合收尾判定用）
	hadOut  bool           // 是否已产出 primary（区分「输出为 null」与「未执行」）
}

// bpCompState 复合节点运行时实例（对齐前端 compRuns inst）：派生激活全部出池后关闭 doneCh。
type bpCompState struct {
	ownerID string // 复合节点 ID
	active  int    // 派生激活计数（stMu 保护）
	closed  bool   // doneCh 是否已关闭（防重复 close）
	doneCh  chan struct{}
}

// bpEventWait event_start 等待项（topic 空 = 任意事件）。
type bpEventWait struct {
	topic    string
	resultCh chan any
}

// bpAckResult execTestAction 的异步回执（channel 投递用；TTS hook 可能晚于调用返回）。
type bpAckResult struct {
	value any
	err   error
}

// bpExecResult 浏览器协作节点执行回报（ltp9/bp_exec_result 入站 JSON 直解）。
type bpExecResult struct {
	ExecID      string         `json:"exec_id"`
	OK          bool           `json:"ok"`
	Value       any            `json:"value"`
	Error       string         `json:"error"`
	ParamsPatch map[string]any `json:"params_patch"`
	Pop         map[string]any `json:"pop"`
}

// bpOutcome 单次节点执行的产物（bp_node 推送与下游取值来源）。
type bpOutcome struct {
	status  string // ok | err | skip
	out     any    // 主输出（nil 且 hadOut=false 表示无输出）
	hadOut  bool
	multi   map[string]any // 多输出
	patch   map[string]any // params 回写（仅变化字段）
	pop     map[string]any // 气泡
	errText string
}

// bpRunner 单次蓝图运行实例（扁平执行池调度器）。
type bpRunner struct {
	runID string
	mode  string // all | single
	graph *bpGraph

	nodes   map[string]*bpNodeRun // 扁平命名空间（含复合重映射 ~cN 节点）
	order   []string              // 主图节点顺序（种子扫描遍历用）
	links   []bpLinkSpec          // 已清洗的有效连线（端点均存在；复合执行时追加派生连线，stMu 保护）
	compSeq int                   // 复合节点 ID 重映射序号（stMu 保护）

	stMu            sync.RWMutex            // 保护 active/callCount/maxCalls/refs/comps/compSeq/links
	active          int                     // 执行池内挂起/执行中的激活数
	callCount       int                     // 本次运行已执行的节点数量
	maxCalls        int                     // 单次运行最大节点执行数量（limit 节点动态调整）
	limitedNotified bool                    // 达到上限只提示一次
	refs            map[string]int          // 节点 → 在池激活数
	comps           map[string]*bpCompState // 复合节点 ID → 运行时实例

	clockMu sync.Mutex
	clocks  []*time.Ticker // clock_start 定时器（finish 时统一停止）

	evWaitMu  sync.Mutex
	evWaiters []*bpEventWait // event_start 等待者
	evWaitSeq int

	execMu   sync.Mutex
	execSeq  int
	execWait map[string]chan *bpExecResult // exec_id → 浏览器回报通道

	haltCh     chan struct{} // 关闭即终止（手动停止或运行收尾）
	stopOnce   sync.Once
	doneMu     sync.Mutex
	doneReason string // 首个收尾原因（finished/maxcalls/stopped/error）；非空表示已收尾
	startAt    time.Time
}
