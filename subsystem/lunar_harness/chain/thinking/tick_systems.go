package thinking

// tick_systems.go 内建 Tick 体系适配器（对话 / 后勤 / 响应 / 演绎 / 生活）。
//
// 所有链都向同一个调度器注册，由调度器统一保证「同一时刻只有一条链执行一个原子步骤」；
// 新增链只需要实现 System 接口并注册优先级，不需要在调度器里增加分支。
// 观影由生活链的五步回合编排驱动，不再单独占用 Tick 体系。

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"LunarSubsystem/LunarHarness/agents/Responder"
	"LunarSubsystem/LunarHarness/agents/Savior"
	"LunarSubsystem/LunarHarness/chain/life"
	"LunarSubsystem/LunarHarness/chain/storyboard"
	"LunarSubsystem/LunarHarness/chain/thinking/tickloop"
)

// asyncStep 后台步骤执行器：同一时刻最多一个步骤在途。
//
// 演绎与生活的单步可能包含几十秒到几分钟的模型调用。同步执行会一直占着
// 调度器的忙碌标记，把对话和其他链一起挡住；放到后台后，Run 立即返回，
// Ready 在步骤结束前保持 false，其他链可以正常推进。
type asyncStep struct {
	running atomic.Bool
	wg      sync.WaitGroup
}

func newAsyncStep() *asyncStep { return &asyncStep{} }

// active 报告是否已有步骤在途。
func (a *asyncStep) active() bool { return a.running.Load() }

// start 启动一个后台步骤；已有步骤在途时直接忽略本次触发。
func (a *asyncStep) start(ctx context.Context, step func(context.Context) error, onError func(error)) {
	if !a.running.CompareAndSwap(false, true) {
		return
	}
	a.wg.Add(1)
	go func() {
		defer func() {
			a.running.Store(false)
			a.wg.Done()
		}()
		if err := step(ctx); err != nil && onError != nil {
			onError(err)
		}
	}()
}

// wait 等待在途步骤结束；超时返回 false。
func (a *asyncStep) wait(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

type dialogueSystem struct{}

func (dialogueSystem) ID() string          { return "dialogue" }
func (dialogueSystem) Priority() int       { return 300 }
func (dialogueSystem) Kind() tickloop.Kind { return tickloop.KindThinking }
func (dialogueSystem) IdleReset() int      { return dialogueIdleReset }

func (dialogueSystem) Ready(snapshot TickSnapshot) (bool, string) {
	if snapshot.Unread > 0 {
		return true, fmt.Sprintf("未读消息 %d 条", snapshot.Unread)
	}
	return false, ""
}

func (dialogueSystem) Run(ctx context.Context) error {
	incoming := pullContext()
	if len(incoming) == 0 {
		return nil
	}
	runDialogueTurn(incoming)
	return nil
}

type logisticsSystem struct{}

func (logisticsSystem) ID() string          { return "logistics" }
func (logisticsSystem) Priority() int       { return 280 }
func (logisticsSystem) Kind() tickloop.Kind { return tickloop.KindMaintenance }
func (logisticsSystem) IdleReset() int      { return 0 }
func (logisticsSystem) KeepIdle() bool      { return true }

func (logisticsSystem) Ready(snapshot TickSnapshot) (bool, string) {
	if snapshot.SalvagePending > 0 {
		return true, fmt.Sprintf("待修复故障 %d 个", snapshot.SalvagePending)
	}
	return false, ""
}

// Run 后勤体系：尝试修复一个故障。
// 它接管空闲 Tick（KeepIdle）：本次运行只推进一格生活冷却，不重置进度。
func (logisticsSystem) Run(ctx context.Context) error {
	Savior.Process(ctx)
	return nil
}

type responseSystem struct{}

func (responseSystem) ID() string          { return "response" }
func (responseSystem) Priority() int       { return 260 }
func (responseSystem) Kind() tickloop.Kind { return tickloop.KindResponse }
func (responseSystem) IdleReset() int      { return 0 }

func (responseSystem) Ready(TickSnapshot) (bool, string) {
	if Responder.Paused() {
		return false, ""
	}
	if pending := Responder.Pending(); pending > 0 {
		return true, fmt.Sprintf("外部推理请求 %d 条", pending)
	}
	return false, ""
}

func (responseSystem) Run(ctx context.Context) error {
	Responder.Drain(ctx)
	return nil
}

type storyboardSystem struct{ step *asyncStep }

// newStoryboardSystem 创建异步推进的演绎体系：长步骤不再独占 Tick。
func newStoryboardSystem() storyboardSystem {
	return storyboardSystem{step: newAsyncStep()}
}

func (storyboardSystem) ID() string          { return "storyboard" }
func (storyboardSystem) Priority() int       { return 200 }
func (storyboardSystem) Kind() tickloop.Kind { return tickloop.KindJob }
func (storyboardSystem) IdleReset() int      { return 0 }

func (s storyboardSystem) Ready(TickSnapshot) (bool, string) {
	if s.step != nil && s.step.active() {
		return false, ""
	}
	if storyboard.HasWork() {
		return true, "待推进的演绎任务"
	}
	return false, ""
}

func (s storyboardSystem) Run(ctx context.Context) error {
	if s.step == nil {
		return storyboard.Advance(ctx)
	}
	s.step.start(ctx, storyboard.Advance, func(err error) { reportTickError("storyboard", err) })
	return nil
}

// StepActive 报告演绎步骤是否在途。
func (s storyboardSystem) StepActive() bool {
	return s.step != nil && s.step.active()
}

// WaitStep 退出时等待在途的演绎步骤收尾。
func (s storyboardSystem) WaitStep(timeout time.Duration) bool {
	if s.step == nil {
		return true
	}
	return s.step.wait(timeout)
}

type lifeSystem struct{ step *asyncStep }

// newLifeSystem 创建异步推进的生活体系：创作等长事件不再独占 Tick。
func newLifeSystem() lifeSystem {
	return lifeSystem{step: newAsyncStep()}
}

func (lifeSystem) ID() string          { return "life" }
func (lifeSystem) Priority() int       { return 100 }
func (lifeSystem) Kind() tickloop.Kind { return tickloop.KindLife }
func (lifeSystem) IdleReset() int      { return lifeIdleReset }

func (s lifeSystem) Ready(snapshot TickSnapshot) (bool, string) {
	if s.step != nil && s.step.active() {
		return false, ""
	}
	if life.HasPendingEvent() {
		return true, "工具触发的强制生活事件"
	}
	if life.HasActiveRun() {
		return true, "续跑未完成生活事件"
	}
	if snapshot.IdleTicks >= life.IdleThreshold() {
		return true, "空闲冷却到期"
	}
	return false, ""
}

func (s lifeSystem) Run(ctx context.Context) error {
	if s.step == nil {
		return life.Step(ctx)
	}
	s.step.start(ctx, life.Step, func(err error) { reportTickError("life", err) })
	return nil
}

// StepActive 报告生活步骤是否在途。
func (s lifeSystem) StepActive() bool {
	return s.step != nil && s.step.active()
}

// WaitStep 退出时等待在途的生活步骤收尾。
func (s lifeSystem) WaitStep(timeout time.Duration) bool {
	if s.step == nil {
		return true
	}
	return s.step.wait(timeout)
}
