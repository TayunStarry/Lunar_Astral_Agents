//go:build windows

package desktop

// control_test.go 钉住中断收尾与桌面调用超时的行为：状态必须能恢复，不能永远停在「运行中」。

import (
	"context"
	"errors"
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

func TestCallDesktopHonorsCancelAndTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := callDesktop(ctx, time.Second, "测试", func() (int, error) { return 1, nil }); !errors.Is(err, ErrCanceled) {
		t.Fatalf("已取消的上下文应返回 ErrCanceled，实际 %v", err)
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	started := time.Now()
	_, err := callDesktop(ctx2, 30*time.Millisecond, "阻塞测试", func() (int, error) {
		time.Sleep(200 * time.Millisecond)
		return 1, nil
	})
	if err == nil || errors.Is(err, ErrCanceled) {
		t.Fatalf("超时应返回普通错误而不是 ErrCanceled，实际 %v", err)
	}
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("超时保护没有及时返回，耗时 %s", elapsed)
	}
}
