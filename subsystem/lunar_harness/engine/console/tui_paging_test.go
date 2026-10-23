package console

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestPgKeysSwitchScreens 覆盖：PgUp/PgDn 是「快速换页」——按侧栏顺序切屏，
// 不需要先按 Esc 回主菜单。主菜单上按 PgDn 进入首屏，功能屏上按 PgUp/PgDn 前后切。
func TestPgKeysSwitchScreens(t *testing.T) {
	m := newAppModel()
	m.width, m.height = 120, 40

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	next := model.(appModel)
	if next.screen != navigableScreens()[0] {
		t.Fatalf("主菜单按 PgDn 应进入首屏 %d，实得 %d", navigableScreens()[0], next.screen)
	}

	model, _ = next.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	after := model.(appModel)
	if after.screen != navigableScreens()[1] {
		t.Fatalf("PgDn 应切到下一屏 %d，实得 %d", navigableScreens()[1], after.screen)
	}

	model, _ = after.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	back := model.(appModel)
	if back.screen != navigableScreens()[0] {
		t.Fatalf("PgUp 应切回上一屏 %d，实得 %d", navigableScreens()[0], back.screen)
	}
}

// TestCtrlKeysPageContent 覆盖：PgUp/PgDn 改为切屏后，屏内翻页由 Ctrl+U/Ctrl+D 承担。
func TestCtrlKeysPageContent(t *testing.T) {
	if next, follow, ok := scrollKey(20, "ctrl+u", 100, 10, false); !ok || next != 11 || follow {
		t.Fatalf("Ctrl+U 应上翻 9 行到 11，实得 next=%d follow=%v ok=%v", next, follow, ok)
	}
	if next, _, ok := scrollKey(20, "ctrl+d", 100, 10, false); !ok || next != 29 {
		t.Fatalf("Ctrl+D 应下翻 9 行到 29，实得 next=%d ok=%v", next, ok)
	}

	if idx, ok := listNavKey(0, "ctrl+d", 50, 10, false); !ok || idx != 10 {
		t.Fatalf("列表 Ctrl+D 应翻到 10，实得 idx=%d ok=%v", idx, ok)
	}
	if idx, ok := listNavKey(20, "ctrl+u", 50, 10, false); !ok || idx != 10 {
		t.Fatalf("列表 Ctrl+U 应翻回 10，实得 idx=%d ok=%v", idx, ok)
	}

	pane := textPane{}
	if !pane.key("ctrl+d", 10) || pane.offset != 9 {
		t.Fatalf("详情窗格 Ctrl+D 应下移 9 行，实得 offset=%d", pane.offset)
	}
	if !pane.key("ctrl+u", 10) || pane.offset != 0 {
		t.Fatalf("详情窗格 Ctrl+U 应上移回 0，实得 offset=%d", pane.offset)
	}
}
