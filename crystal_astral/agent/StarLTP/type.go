package StarLTP

// ==== 类型定义集中区 ====

import (
	"encoding/json"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// jsFunc goja 回调引用（在对应插件事件循环内调用）。
type jsFunc = goja.Value

// ==== 插件（每插件独立 goja 沙箱） ====

// plugin 一个 LTP9 包对应的运行实例。持有独立事件循环（自带定时器）与 engine.* 绑定。
type plugin struct {
	ID      string // 插件 ID
	DirName string // 包目录名
	Title   string
	Root    string
	MainPath   string // execute.js
	ConfigPath string // config.yaml
	DataDir    string // data/
	KeyPath    string // permissions.key

	config  map[string]any
	granted map[string]bool // 经 permissions.key 代码哈希解密校验通过的 allow-* 清单

	loop *eventloop.EventLoop // 每插件独立事件循环（goja.New + 自带定时器）
	mu   sync.Mutex           // 串行化同一插件全部 JS 执行

	loaded  bool
	loadErr string
	onLoad  jsFunc
	onUnload jsFunc

	// 订阅 id 单调计数器（事件订阅器 + 前端信号订阅共用）
	subSeq int
	// 事件订阅器：topic → 订阅器组（可重复订阅）
	events map[string][]*eventSub
	// 导出函数：fnName → 处理器（供 engine.call）
	exports map[string]jsFunc
	// 前端事件订阅器（engine.frontEvent.signal）
	frontSignal []*eventSub
}

// eventSub 单个事件订阅项。
type eventSub struct {
	topic    string
	orderID  int    // 订阅 id（插件内单调递增，同时作为时间顺序依据）
	handler  jsFunc
	priority int    // 优先级（0 最高；priorityUnset 表示未设置，按时间顺序排列在设置者之后）
}

// ==== 引擎管理器 ====

// engine LTP9 引擎管理器。
type engine struct {
	mu      sync.RWMutex
	plugins map[string]*plugin // 插件 ID → 插件
	root    string             // LTP9 包根目录
	running bool

	// outboundSender 由外部 Go 程序注入：插件产生的单向通报/消息（engine.signal → Go 侧）。
	outbound func(topic string, payload any)
}

// ==== 公开接口结果类型 ====

// Outcome 单个插件对一次事件/调用的处理结果。
type Outcome struct {
	PluginID string `json:"plugin_id"`
	Error    string `json:"error,omitempty"`
	Handled  bool   `json:"handled"`
	Result   any    `json:"result,omitempty"` // 订阅回调返回对象（intercept/modifiedData…）
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
	Topic   string        `json:"topic"`
	Outcomes []Outcome     `json:"outcomes"`
	Summary EmitSummary   `json:"summary"`
}

// PluginState 插件状态（供外部 Go 程序枚举）。
type PluginState struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Loaded  bool   `json:"loaded"`
	Error   string `json:"error,omitempty"`
	Granted []string `json:"granted,omitempty"`
}

// ==== 扩展类型 ====

// WsBridge 宿主注入的 WebSocket 服务端传输：Serve 挂载一个路径并返回访问地址，Publish 向该路径广播。
// 由宿主（crystal_astral）实现真实传输并调用 SetWsServer 注入；未注入时 engine.ws 返回错误提示。
type WsBridge struct {
	Serve   func(pluginID, path string, onMessage func(msg string) string) (string, error)
	Publish func(pluginID, path, data string) error
}

func ok(text string) rwResult { return rwResult{Success: true, Text: text} }
func fail(err string) rwResult { return rwResult{Success: false, Error: err} }

// rwResult engine.* API 的统一返回（Go → JS）。
type rwResult struct {
	Success bool   `json:"success"`
	Text    string `json:"text,omitempty"`
	Error   string `json:"error,omitempty"`
}

func parseJSONArgs(s string) map[string]any {
	m := map[string]any{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return map[string]any{}
	}
	return m
}