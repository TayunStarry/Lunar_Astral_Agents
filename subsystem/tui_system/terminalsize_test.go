package TuiSystem

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestWatchTerminalSizePushesChanges 终端尺寸轮询的核心契约：**尺寸变化必须上报给程序**。
//
// 为什么必须钉住：bubbletea 在 Windows 上没有 SIGWINCH（signals_windows.go 的
// listenForResize 直接关闭），尺寸只在启动时取一次。用户把窗口改小后，框架仍按旧尺寸渲染
// 整屏——多出来的行会被终端顶掉，表现为「控制台不停向上滚、把字滚没了」，并且永远不会自愈。
//
// 实测证据（真实 conhost 窗口，120x30 → 缩小到 120x20，见 .scratch/scrollprobe）：
//
//	按旧尺寸渲染 30 行 → 屏幕首行变成 frame10，顶部 10 行被顶掉（症状复现）
//	重新测量后渲染 20 行 → 屏幕首行仍是 frame00（不滚动）
//
// 模型侧「缩放后仍不溢出」由 TestAllScreensFitTerminal 与按键漫游测试覆盖；
// 本用例守住它们的前提——缩放事件真的会被送达。
func TestWatchTerminalSizePushesChanges(t *testing.T) {
	old := terminalSizePollInterval
	terminalSizePollInterval = 5 * time.Millisecond
	defer func() { terminalSizePollInterval = old }()

	sizes := []struct {
		w, h int
		ok   bool
	}{
		{120, 30, true}, // 初始
		{120, 30, true}, // 无变化 → 不应上报
		{0, 0, false},   // 读不到 → 不应上报
		{120, 20, true}, // 窗口改小 → 必须上报
		{120, 20, true}, // 无变化
		{160, 50, true}, // 放大 → 必须上报
	}
	i := 0
	next := func() (int, int, bool) {
		if i >= len(sizes) {
			return 160, 50, true // 序列用尽后保持最后一次尺寸（不应再上报）
		}
		s := sizes[i]
		i++
		return s.w, s.h, s.ok
	}

	got := make(chan tea.WindowSizeMsg, 8)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		watchTerminalSize(func(m tea.Msg) {
			if ws, ok := m.(tea.WindowSizeMsg); ok {
				got <- ws
			}
		}, next, stop)
		close(done)
	}()

	want := []tea.WindowSizeMsg{{Width: 120, Height: 20}, {Width: 160, Height: 50}}
	for idx, w := range want {
		select {
		case msg := <-got:
			if msg != w {
				t.Fatalf("第 %d 次上报应为 %+v，实际 %+v", idx+1, w, msg)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("第 %d 次尺寸变化未上报（期望 %+v）——Windows 上尺寸失效后界面会一直向上滚",
				idx+1, w)
		}
	}

	select {
	case <-got:
		t.Fatalf("尺寸未变化时不应额外上报")
	case <-time.After(50 * time.Millisecond):
	}

	close(stop)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("停止信号未使轮询退出（goroutine 泄漏）")
	}
}

// TestConsoleSizeOnRealConsole 轮询依赖 consoleSize 真能读到尺寸：
// 若它在真实控制台上返回 ok=false，轮询会静默退出，滚动问题会原样保留。
func TestConsoleSizeOnRealConsole(t *testing.T) {
	f := consoleOut()
	if f == nil {
		t.Skip("当前环境没有控制台，跳过")
	}
	defer f.Close()
	w, h, ok := consoleSize(f)
	if !ok || w <= 0 || h <= 0 {
		t.Fatalf("真实控制台句柄上未读到尺寸（ok=%v %dx%d）——尺寸轮询会失效", ok, w, h)
	}
	t.Logf("控制台可视窗口尺寸: %d 列 x %d 行", w, h)
}

// TestWatchTerminalSizeNoConsole 取不到尺寸时轮询应立即退出（不产生空转 goroutine）
func TestWatchTerminalSizeNoConsole(t *testing.T) {
	done := make(chan struct{})
	go func() {
		watchTerminalSize(func(tea.Msg) { t.Errorf("无控制台时不应上报尺寸") },
			func() (int, int, bool) { return 0, 0, false }, make(chan struct{}))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("取不到尺寸时应立即返回")
	}
}
