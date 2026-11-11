package tickloop

// type.go 通用 Tick 运行时类型定义。
//
// 调度器只负责「何时触发哪个体系」与状态持久化；体系如何判定就绪、执行什么，
// 全部由宿主通过泛型快照与 Hooks 注入，运行时本身不依赖任何智能体包。

import (
	"context"
	"time"

	"LunarSubsystem/LunarHarness/engine/tickpermit"
)

// Kind 体系类别，用于特许运行与优先级策略。
type Kind int

const (
	// KindThinking 思维链：人格对话，优先级最高。
	KindThinking Kind = iota
	// KindMaintenance 后勤 / 故障修复。
	KindMaintenance
	// KindJob 用户发起的后台任务（演绎）。
	KindJob
	// KindResponse 外部响应链。
	KindResponse
	// KindLife 生活链。
	KindLife
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

// kinded 可选能力：声明体系类别；未声明时按 ID 推断。
type kinded interface {
	Kind() Kind
}

// KindOf 返回体系类别；未显式声明时按 ID 推断。
func KindOf[S any](system System[S]) Kind {
	if item, ok := system.(kinded); ok {
		return item.Kind()
	}
	switch system.ID() {
	case "life":
		return KindLife
	case "logistics":
		return KindMaintenance
	case "response":
		return KindResponse
	default:
		return KindThinking
	}
}

// idleKeeper 可选能力：把本次运行当作一格空闲推进（冷却 +1 而不是重置）。
type idleKeeper interface {
	KeepIdle() bool
}

// StepWaiter 可选能力：体系把长步骤放到后台执行，Stop 时需要等待其收尾。
//
// 异步步骤让调度器的忙碌标记只覆盖编排开销，模型调用与工具执行期间
// 其他链和响应链仍可运行；实现方必须保证同一时刻只有一个步骤在途。
type StepWaiter interface {
	// StepActive 报告后台步骤是否在途（不阻塞）。
	StepActive() bool
	// WaitStep 等待后台步骤结束；超时返回 false。
	WaitStep(timeout time.Duration) bool
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
	// Permit 特许运行控制器：思考链让出 Tick 期间只允许响应链并行。
	Permit *tickpermit.Controller
	// AllowResponseWhileBusy 主链步骤在途时是否允许响应链并行。
	//
	// 创作、分镜规划这类单步可能执行几十秒到几分钟；若响应链也被挡住，
	// 外部请求会表现为「后端卡死」。开启后响应链可在主链执行期间插入，
	// 主链之间仍保持串行。
	AllowResponseWhileBusy bool
	// Forced 报告是否存在需要抢占执行的强制事件（为 true 时只允许 life 体系）。
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
