package BilibiliLife

// 手动冒烟：真实生活跑到一半时模拟“收到未读消息”，验证会停在步骤边界并落断点。
//   $env:BILI_LIFE_PAUSE_PROBE="1"; go test -run TestManualPauseProbe -v ./agents/BilibiliLife

import (
	"os"
	"testing"
	"time"

	"LunarSubsystem/GeneralConfig"
)

func TestManualPauseProbe(t *testing.T) {
	if os.Getenv("BILI_LIFE_PAUSE_PROBE") != "1" {
		t.Skip("设置 BILI_LIFE_PAUSE_PROBE=1 才真实跑生活并中断")
	}
	*GeneralConfig.LocalDir = `D:\Lunar_Astral_Agents\local_data`
	done := make(chan error, 1)
	go func() { done <- Run() }()
	time.Sleep(15 * time.Second)
	NotifyUnread()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("生活返回错误: %v", err)
		}
	case <-time.After(6 * time.Minute):
		t.Fatal("生活没有在 6 分钟内停到断点")
	}
	manager.mu.Lock()
	paused := manager.paused
	progress := manager.progress
	r := manager.runner
	manager.running = false
	manager.paused = false
	manager.runner = nil
	manager.mu.Unlock()
	if !paused {
		t.Fatalf("应处于 paused，实际 progress=%q", progress)
	}
	t.Logf("已暂停，进度标题：%s", progress)
	if r != nil {
		r.cleanup()
	}
}
