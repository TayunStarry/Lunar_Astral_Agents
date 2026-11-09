package thinking

// tick_systems.go 内建 Tick 体系适配器（对话 / 后勤 / 响应 / 创作 / 演绎 / 生活）。
//
// 所有链都向同一个调度器注册，由调度器统一保证「同一时刻只有一条链执行一个原子步骤」；
// 新增链只需要实现 System 接口并注册优先级，不需要在调度器里增加分支。
// 观影由生活链的五步回合编排驱动，不再单独占用 Tick 体系。

import (
	"context"
	"fmt"

	"LunarSubsystem/LunarHarness/agents/Responder"
	"LunarSubsystem/LunarHarness/agents/Savior"
	"LunarSubsystem/LunarHarness/chain/creation"
	"LunarSubsystem/LunarHarness/chain/life"
	"LunarSubsystem/LunarHarness/chain/storyboard"
	"LunarSubsystem/LunarHarness/chain/thinking/tickloop"
)

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

type creationSystem struct{}

func (creationSystem) ID() string          { return "creation" }
func (creationSystem) Priority() int       { return 220 }
func (creationSystem) Kind() tickloop.Kind { return tickloop.KindJob }
func (creationSystem) IdleReset() int      { return 0 }

func (creationSystem) Ready(TickSnapshot) (bool, string) {
	if creation.HasWork() {
		return true, "待执行的资源库创作任务"
	}
	return false, ""
}

func (creationSystem) Run(ctx context.Context) error {
	return creation.Step(ctx)
}

type storyboardSystem struct{}

func (storyboardSystem) ID() string          { return "storyboard" }
func (storyboardSystem) Priority() int       { return 200 }
func (storyboardSystem) Kind() tickloop.Kind { return tickloop.KindJob }
func (storyboardSystem) IdleReset() int      { return 0 }

func (storyboardSystem) Ready(TickSnapshot) (bool, string) {
	if storyboard.HasWork() {
		return true, "待推进的演绎任务"
	}
	return false, ""
}

func (storyboardSystem) Run(ctx context.Context) error {
	return storyboard.Advance(ctx)
}

type lifeSystem struct{}

func (lifeSystem) ID() string          { return "life" }
func (lifeSystem) Priority() int       { return 100 }
func (lifeSystem) Kind() tickloop.Kind { return tickloop.KindLife }
func (lifeSystem) IdleReset() int      { return lifeIdleReset }

func (lifeSystem) Ready(snapshot TickSnapshot) (bool, string) {
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

func (lifeSystem) Run(ctx context.Context) error {
	return life.Step(ctx)
}
