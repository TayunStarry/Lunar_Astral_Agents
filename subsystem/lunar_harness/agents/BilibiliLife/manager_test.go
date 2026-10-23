package BilibiliLife

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// TestConfigHotReload 钉住 bilibili_life.json 的改动会被自动调度重新加载：
// 否则新增的 @ 目标要等重启才生效，表现为「永远只 @ 同一个人」。
func TestConfigHotReload(t *testing.T) {
	tmp := t.TempDir()
	orig := *config.LocalDir
	*config.LocalDir = tmp
	t.Cleanup(func() { *config.LocalDir = orig })

	path := filepath.Join(tmp, "bilibili_life.json")
	write := func(body string, mod time.Time) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("写入配置失败: %v", err)
		}
		// 显式设定修改时间，避免同一时刻写入导致 mtime 无法区分
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatalf("设置修改时间失败: %v", err)
		}
	}

	m := &lifeManager{}
	write(`{"mention_targets":[{"name":"甲"}]}`, time.Now().Add(-2*time.Hour))
	m.reloadConfigLocked()
	if len(m.cfg.MentionTargets) != 1 || m.cfg.MentionTargets[0].Name != "甲" {
		t.Fatalf("首次加载不正确: %+v", m.cfg.MentionTargets)
	}

	// 文件更新后刷新配置，新的 @ 目标应立刻进入候选池
	write(`{"mention_targets":[{"name":"甲"},{"name":"乙"}]}`, time.Now().Add(-time.Hour))
	m.refreshConfigLocked()
	names := m.cfg.mentionNames()
	if len(names) != 2 || names[0] != "甲" || names[1] != "乙" {
		t.Fatalf("配置未热重载，实际 %v", names)
	}
}

// TestAutoLifeStartupGrace 钉住启动静默期：进程启动 10 分钟内自动生活不进入，
// 只有 TUI 强制触发（Start）可以开始；超过 10 分钟后自动调度才恢复。
func TestAutoLifeStartupGrace(t *testing.T) {
	orig := processStartedAt
	t.Cleanup(func() { processStartedAt = orig })

	processStartedAt = time.Now()
	if !autoLifeGraceActive() {
		t.Fatal("刚启动应处于生活静默期")
	}
	if startupLifeGrace != 10*time.Minute {
		t.Fatalf("静默期应为 10 分钟，实际 %v", startupLifeGrace)
	}

	processStartedAt = time.Now().Add(-startupLifeGrace - time.Second)
	if autoLifeGraceActive() {
		t.Fatal("超过静默期后不应再拦截自动生活")
	}
}

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
