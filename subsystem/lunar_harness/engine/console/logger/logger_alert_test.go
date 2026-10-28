package logger

// logger_alert_test.go 钉住告警钩子的触发契约：仅开发者模式 + 警告及以上。

import (
	"io"
	"os"
	"testing"
)

func TestAlertHookOnlyFiresForWarnAndAboveInDevMode(t *testing.T) {
	previousDev := devModeEnabled()
	defer SetDevMode(previousDev)
	SetOutput(io.Discard)
	defer SetOutput(os.Stdout)

	var got []string
	SetAlertHook(func(level, module, sub, msg string) {
		got = append(got, level+"|"+module+"|"+sub+"|"+msg)
	})
	defer SetAlertHook(nil)

	// 非开发者模式：警告也不回调
	SetDevMode(false)
	Warn("AlertTest", "warn %d", 1)

	if len(got) != 0 {
		t.Fatalf("非开发者模式不应触发告警钩子，实际: %v", got)
	}

	// 开发者模式：INFO 不回调，WARN/ERROR 各回调一次
	SetDevMode(true)
	Info("AlertTest", "info")
	Warn("AlertTest", "warn %d", 2)
	SubError("AlertTest", "Sub", "error %d", 3)

	if len(got) != 2 {
		t.Fatalf("应只收到 2 次告警回调，实际 %d 次: %v", len(got), got)
	}
	if got[0] != "WARN|AlertTest|not|warn 2" {
		t.Fatalf("WARN 回调内容不符: %q", got[0])
	}
	if got[1] != "ERROR|AlertTest|Sub|error 3" {
		t.Fatalf("ERROR 回调内容不符: %q", got[1])
	}
}

func TestAlertHookNilIsSafe(t *testing.T) {
	previousDev := devModeEnabled()
	defer SetDevMode(previousDev)
	SetOutput(io.Discard)
	defer SetOutput(os.Stdout)

	// 钩子为 nil 时日志路径不得报错或崩溃
	SetAlertHook(nil)
	Warn("AlertTest", "warn without hook")
}
