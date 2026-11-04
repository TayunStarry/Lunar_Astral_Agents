//go:build windows

package desktopchain

// control_test.go 钉住中断收尾行为：状态必须能恢复，不能永远停在「运行中」。

import (
	"testing"
	"time"
)

func TestForceFinishResetsRunningState(t *testing.T) {
	runMu.Lock()
	oldState, oldSeq := runState, runSeq
	runState = RunState{Running: true, Stopping: true, Instruction: "打开记事本", StartedAt: time.Now()}
	runSeq = 10
	forceFinishLocked("已中断（强制结束）")
	state, seq := runState, runSeq
	runMu.Unlock()

	t.Cleanup(func() {
		runMu.Lock()
		runState, runSeq = oldState, oldSeq
		runMu.Unlock()
	})

	if state.Running || state.Stopping {
		t.Fatalf("强制结束后应退出运行/停止中状态: %#v", state)
	}
	if state.Error != "已中断（强制结束）" {
		t.Fatalf("强制结束原因错误: %q", state.Error)
	}
	if seq != 11 {
		t.Fatalf("强制结束应递增运行代次，实际 %d", seq)
	}
}
