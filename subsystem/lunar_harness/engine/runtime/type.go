package runtime

// ==== 类型定义集中区 ====

import (
	"regexp"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// jsFunc goja 回调引用（在对应运行时事件循环内调用）。
type jsFunc = goja.Value

// ==== 私有运行时（每次运行一个独立 goja 沙箱） ====

// plugin 一个私有 js 运行时实例。持有独立事件循环（自带定时器）与顶层全局绑定。
type plugin struct {
	ID    string // 运行时标识
	Title string

	DataDir string // data/（file.* 写入隔离区）

	loop *eventloop.EventLoop // 独立事件循环（goja.New + 自带定时器）
	mu   sync.Mutex           // 串行化同一运行时全部 JS 执行

	loaded  bool
	loadErr string

	// 订阅 id 单调计数器（事件订阅器 + 广播订阅共用）
	subSeq int
	// 事件订阅器：topic → 订阅器组（可重复订阅）
	events map[string][]*eventSub
	// 导出函数：fnName → 处理器（供宿主经 Runtime.Call 调用）
	exports map[string]jsFunc
	// 广播订阅器（signal.subscribe）
	signalSubs []*eventSub

	// 指令注册：指令名（含别名）→ 指令项（command.register）
	commands map[string]*commandEntry
}

// commandEntry 单个指令项（command.register 注册，宿主侧经 Command/CommandAll 触发）。
type commandEntry struct {
	name    string
	re      *regexp.Regexp
	handler jsFunc
	aliases []string
}

// eventSub 单个事件订阅项。
type eventSub struct {
	topic    string
	orderID  int // 订阅 id（运行时内单调递增，同时作为时间顺序依据）
	handler  jsFunc
	priority int // 优先级（0 最高；priorityUnset 表示未设置，按时间顺序排列在设置者之后）
}

// rwResult 沙箱 API 的统一返回（Go → JS）。
type rwResult struct {
	Success bool   `json:"success"`
	Text    string `json:"text,omitempty"`
	Error   string `json:"error,omitempty"`
	// ToolCalls agent.chat 响应中的工具调用（OpenAI 兼容 tool_calls 原样透传）
	ToolCalls []any `json:"tool_calls,omitempty"`
}
