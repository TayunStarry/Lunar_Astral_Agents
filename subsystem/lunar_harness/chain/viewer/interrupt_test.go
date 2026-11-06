package viewer

// interrupt_test.go 固定「用户消息优先于观影」的语义：
// 用户消息到达时观影在安全边界让出，回应结束后能原样续跑。

import (
	"testing"
)

// withPause 安装观影让出判定，测试结束自动撤销。
func withPause(t *testing.T, waiting func() bool) {
	t.Helper()
	SetPauseHook(waiting)
	t.Cleanup(func() { SetPauseHook(nil) })
}

// TestPauseHookDefaultOff 未注入判定时观影永不让出（不影响原有节奏）。
func TestPauseHookDefaultOff(t *testing.T) {
	SetPauseHook(nil)
	if pauseRequested() {
		t.Fatal("未注入判定时不应该让出")
	}
}

// TestPauseHookReportsWaiting 注入判定后按判定结果让出。
func TestPauseHookReportsWaiting(t *testing.T) {
	withPause(t, func() bool { return true })
	if !pauseRequested() {
		t.Fatal("有用户消息时应在安全边界让出")
	}
}
