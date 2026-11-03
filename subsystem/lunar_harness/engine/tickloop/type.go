package tickloop

// type.go 通用 Tick 运行时类型定义。
//
// 调度器只负责「何时触发哪个体系」与状态持久化；体系如何判定就绪、执行什么，
// 全部由宿主通过泛型快照与 Hooks 注入，运行时本身不依赖任何智能体包。

import (
	"context"
	"time"
)

// System 可注册的 Tick 体系。
//   - Priority 越大越先判定；
//   - Ready 只读判定，不得消费消息或改状态；
//   - IdleReset 本次运行结束后把空闲进度重置到该值；
//   - Run 执行一个原子工作单元，返回即表示释放 Tick。
type System[S any] interface {
	ID() string
	Priority() int
	Ready(S) (bool, string)
	IdleReset() int
	Run(ctx context.Context) error
}

// idleKeeper 可选能力：把本次运行当作一格空闲推进（冷却 +1 而不是重置）。
type idleKeeper interface {
	KeepIdle() bool
}

// Hooks 宿主注入的运行时钩子。除 Snapshot 外均可为空，空钩子按零值语义处理。
type Hooks[S any] struct {
	// Module 日志模块名（如 TickLoop）。
	Module string
	// StateFile 调度状态文件（相对 LocalDir）。
	StateFile string
	// PersistEveryN 空闲计数每 N 个 Tick 落盘一次；<=0 时只在触发/释放时落盘。
	PersistEveryN int
	// Snapshot 按当前空闲进度构造体系判定所需的快照。
	Snapshot func(idleTicks int) S
	// Busy 报告宿主是否有独占模型的任务在跑（为 true 时本 Tick 让出）。
	Busy func() bool
	// Forced 报告是否存在需要抢占执行的强制事件（为 true 时只触发 ID 为 life 的体系）。
	Forced func() bool
	// NotifyIdle 空闲 Tick 通知（不放行任何体系时调用）。
	NotifyIdle func()
	// IdleThreshold 生活冷却阈值，仅用于周期性日志。
	IdleThreshold func() int
	// OnError 体系执行失败回调（宿主据此交给修复流程）。
	OnError func(systemID string, err error)
}

// State 调度器持久化状态。
type State struct {
	Version       int    `json:"version"`
	IdleTicks     int    `json:"idle_ticks"`
	LastSystem    string `json:"last_system"`
	LastReason    string `json:"last_reason"`
	LastTriggerAt int64  `json:"last_trigger_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// nowUnix 当前 Unix 秒（便于测试替换）。
var nowUnix = func() int64 { return time.Now().Unix() }
