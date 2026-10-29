package lunar

// tick_systems.go 三个内建 Tick 体系适配器。

import (
	"context"
	"fmt"

	"LunarSubsystem/LunarHarness/agents/BilibiliViewer"
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
