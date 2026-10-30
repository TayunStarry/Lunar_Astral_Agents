package lunar

// tick_systems.go 内建 Tick 体系适配器（对话 / 后勤 / 观影 / 生活）。
// 外部推理请求由响应者独立 Tick 处理（见 responder_loop.go），不在本调度器内。

import (
	"context"
	"fmt"

	"LunarSubsystem/LunarHarness/agents/BilibiliViewer"
	"LunarSubsystem/LunarHarness/agents/Savior"
	"LunarSubsystem/LunarHarness/engine/life"
)

type dialogueSystem struct{}

func (dialogueSystem) ID() string     { return "dialogue" }
func (dialogueSystem) Priority() int  { return 300 }
func (dialogueSystem) IdleReset() int { return dialogueIdleReset }

func (dialogueSystem) Ready(snapshot TickSnapshot) (bool, string) {
	if snapshot.Unread > 0 {
		return true, fmt.Sprintf("未读消息 %d 条", snapshot.Unread)
	}
	return false, ""
}

func (dialogueSystem) Run(ctx context.Context) error {
	stateMu.Lock()
	reasoningInProgress = true
	stateMu.Unlock()
	defer func() {
		stateMu.Lock()
		reasoningInProgress = false
		stateMu.Unlock()
	}()
	incoming := pullContext()
	if len(incoming) == 0 {
		return nil
	}
	runDialogueTurn(incoming)
	return nil
}

type logisticsSystem struct{}

func (logisticsSystem) ID() string     { return "logistics" }
func (logisticsSystem) Priority() int  { return 250 }
func (logisticsSystem) IdleReset() int { return 0 }
func (logisticsSystem) KeepIdle() bool { return true }

func (logisticsSystem) Ready(snapshot TickSnapshot) (bool, string) {
	if snapshot.SalvagePending > 0 {
		return true, fmt.Sprintf("待修复故障 %d 个", snapshot.SalvagePending)
	}
	return false, ""
}

// Run 后勤体系：尝试修复一个故障（外部推理已由响应者独立 Tick 承担）。
// 它接管空闲 Tick（KeepIdle）：本次运行只推进一格生活冷却，不重置进度。
func (logisticsSystem) Run(ctx context.Context) error {
	Savior.Process(ctx)
	return nil
}

type viewingSystem struct{}

func (viewingSystem) ID() string     { return "viewing" }
func (viewingSystem) Priority() int  { return 200 }
func (viewingSystem) IdleReset() int { return viewingIdleReset }

func (viewingSystem) Ready(snapshot TickSnapshot) (bool, string) {
	if snapshot.ViewingPending {
		return true, "待处理视频切片"
	}
	return false, ""
}

func (viewingSystem) Run(ctx context.Context) error {
	return BilibiliViewer.RunOneStep(ctx)
}

type lifeSystem struct{}

func (lifeSystem) ID() string     { return "life" }
func (lifeSystem) Priority() int  { return 100 }
func (lifeSystem) IdleReset() int { return lifeIdleReset }

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
