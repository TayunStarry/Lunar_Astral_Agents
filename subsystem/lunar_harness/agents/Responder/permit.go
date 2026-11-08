package Responder

// permit.go 特许运行标记。
//
// 响应者只在主调度器空档期运行。对话者调用 LTPX 工具时会被阻塞在工具等待上、
// 模型实际空闲，此时置位特许运行，让响应者可以在同一 Tick 周期内特许运行；
// 特许状态与有界等待由 engine/tickpermit 统一维护，本包只保留对外语义。

import (
	"sync/atomic"

	"LunarSubsystem/LunarHarness/engine/tickpermit"
)

var (
	// running 响应者是否正在处理队列（维护态据此避让）
	running atomic.Bool
	// paused 维护态：暂停响应者运行
	paused atomic.Bool
)

// BeginPermit 开启特许运行：对话者开始等待 LTPX 工具返回。
func BeginPermit() {
	tickpermit.Default().Begin(tickpermit.DefaultOwner, tickpermit.DefaultTTL)
}

// EndPermit 关闭特许运行：对话者已拿到工具返回。
// 内部会等待在途响应步骤结束，避免对话恢复模型调用时与响应链并发。
func EndPermit() {
	tickpermit.Default().End(tickpermit.EndWaitTimeout)
}

// Permitted 报告当前是否处于特许运行状态。
func Permitted() bool {
	return tickpermit.Default().Active()
}

// Busy 报告响应者是否正在处理队列。
func Busy() bool {
	return running.Load()
}

// Pause 进入维护态：响应者跳过运行，Drain 不再开始新的推理。
func Pause() {
	paused.Store(true)
}

// Resume 退出维护态，恢复响应者运行。
func Resume() {
	paused.Store(false)
}

// Paused 报告是否处于维护暂停态。
func Paused() bool {
	return paused.Load()
}
