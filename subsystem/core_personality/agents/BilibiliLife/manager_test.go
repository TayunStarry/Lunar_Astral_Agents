package BilibiliLife

import "testing"

func TestManagerHoldAndProgress(t *testing.T) {
	manager.mu.Lock()
	oldRunning, oldPaused, oldInterrupted := manager.running, manager.paused, manager.interrupted
	oldProgress, oldRunner := manager.progress, manager.runner
	manager.running = true
	manager.paused = false
	manager.interrupted = true
	manager.progress = "月华正在看《测试》，看到了 2/3"
	manager.runner = &runner{state: &lifeState{Phase: phaseUnderstand}}
	manager.mu.Unlock()

	if !HoldMessages() {
		t.Fatal("生活未停到断点时应 hold 消息")
	}
	if got := Progress(); got != "月华正在看《测试》，看到了 2/3" {
		t.Fatalf("进度透传错误: %q", got)
	}

	manager.mu.Lock()
	manager.running = false
	manager.paused = true
	manager.interrupted = false
	manager.mu.Unlock()
	if HoldMessages() {
		t.Fatal("生活已暂停后不应继续 hold 消息")
	}

	manager.mu.Lock()
	manager.runner.state.Phase = phaseDone
	manager.mu.Unlock()
	if got := Progress(); got != "" {
		t.Fatalf("整轮结束后进度应为空，实际 %q", got)
	}

	manager.mu.Lock()
	manager.running, manager.paused, manager.interrupted = oldRunning, oldPaused, oldInterrupted
	manager.progress, manager.runner = oldProgress, oldRunner
	manager.mu.Unlock()
}
