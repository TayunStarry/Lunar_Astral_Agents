package LunarPersonality

// life_trigger_test.go 工具触发生活链：强制事件在下一 Tick 抢占其他体系。

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/life"
)

type recordingDialogueSystem struct{ ran atomic.Int32 }

func (s *recordingDialogueSystem) ID() string     { return "dialogue" }
func (s *recordingDialogueSystem) Priority() int  { return 300 }
func (s *recordingDialogueSystem) IdleReset() int { return dialogueIdleReset }
func (s *recordingDialogueSystem) Ready(TickSnapshot) (bool, string) {
	return true, "模拟未读消息"
}
func (s *recordingDialogueSystem) Run(context.Context) error {
	s.ran.Add(1)
	return nil
}

func TestForcedLifeEventPreemptsDialogue(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	oldHook := viewingPendingHook
	viewingPendingHook = func() bool { return false }
	t.Cleanup(func() {
		// 即使用例中途失败也要清空待执行事件，避免污染其他 Tick 用例。
		config.LocalDir = t.TempDir()
		life.Init()
		config.LocalDir = oldDir
		viewingPendingHook = oldHook
	})

	life.Init()
	if !life.RequestEvent("daze", "强制发呆") {
		t.Fatal("强制生活事件登记失败")
	}

	scheduler := newTickScheduler()
	dialogue := &recordingDialogueSystem{}
	scheduler.Register(dialogue)
	scheduler.Register(lifeSystem{})
	scheduler.Tick()

	deadline := time.Now().Add(3 * time.Second)
	for {
		scheduler.mu.Lock()
		busy := scheduler.busy
		lastSystem := scheduler.state.LastSystem
		scheduler.mu.Unlock()
		if !busy {
			if lastSystem != "life" {
				t.Fatalf("下一 Tick 应进入生活链，实际触发 %s", lastSystem)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("生活流程未在限时内完成")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if dialogue.ran.Load() != 0 {
		t.Fatal("强制生活事件应抢占对话体系，优先进入生活链")
	}
	scheduler.Stop()
}
