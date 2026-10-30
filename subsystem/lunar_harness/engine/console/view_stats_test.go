package console

// view_stats_test.go「运行状态」屏：前端窗口已并入本屏，且保留重启入口。

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestStatsScreenMergesFrontendSection(t *testing.T) {
	SetFrontendHooks(func() bool { return true }, func() bool { return false }, nil, nil)
	t.Cleanup(func() { SetFrontendHooks(nil, nil, nil, nil) })

	m := newAppModel()
	m.width, m.height = 140, 40
	m.screen = screenStats
	out := m.View()
	for _, want := range []string{
		"运行状态", "前端窗口", "[1] 月华前端", "[2] 琉璃前端",
		"● 在线", "○ 离线", "桌面操纵智能体", "总上下文", "输入", "输出",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("运行状态屏缺少 %q:\n%s", want, out)
		}
	}

	// 原独立「前端页面」菜单项不应再出现
	for _, item := range menuItems {
		if item.title == "前端页面" {
			t.Fatal("前端页面应已并入运行状态，不再作为独立菜单项")
		}
	}
}

func TestStatsHotkeyRelaunchFrontend(t *testing.T) {
	SetFrontendHooks(nil, nil, func() error { return nil }, nil)
	t.Cleanup(func() { SetFrontendHooks(nil, nil, nil, nil) })

	m := newAppModel()
	m.screen = screenStats
	updated, _ := m.updateStats(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	if got := updated.(appModel); !got.frontend.busyLunar {
		t.Fatal("按 1 应发起月华前端重新唤起")
	}
}
