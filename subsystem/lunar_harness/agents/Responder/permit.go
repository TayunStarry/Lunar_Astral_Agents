package Responder

// permit.go 特许运行标记。
//
// 响应者 Tick 只在主调度器空档期运行。对话者调用 LTPX 工具时会被阻塞在工具等待上、
// 模型实际空闲，此时置真本标记，让响应者 Tick 可以特许运行；
// 标记一直保持，响应者自身不清除，等对话者真正拿到工具返回时才由调用方置否。

import "sync/atomic"

var (
	// permit 特许运行标记（工具调用期间保持为真）
	permit atomic.Bool
	// running 响应者是否正在处理队列（主调度器据此避让，避免与模型调用并发）
	running atomic.Bool
)

// BeginPermit 开启特许运行：对话者开始等待 LTPX 工具返回。
func BeginPermit() {
	permit.Store(true)
}

// EndPermit 关闭特许运行：对话者已拿到工具返回。
func EndPermit() {
	permit.Store(false)
}

// Permitted 报告当前是否处于特许运行状态。
func Permitted() bool {
	return permit.Load()
}

// Busy 报告响应者是否正在处理队列。
func Busy() bool {
	return running.Load()
}
