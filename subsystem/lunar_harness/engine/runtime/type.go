package runtime

// ==== 类型定义集中区 ====

import (
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

	// 导出函数：fnName → 处理器（供宿主经 Runtime.Call 调用）
	exports map[string]jsFunc
}

// rwResult 沙箱 API 的统一返回（Go → JS）。
type rwResult struct {
	Success bool   `json:"success"`
	Text    string `json:"text,omitempty"`
	Error   string `json:"error,omitempty"`
	// ToolCalls agent.chat 响应中的工具调用（OpenAI 兼容 tool_calls 原样透传）
	ToolCalls []any `json:"tool_calls,omitempty"`
}
