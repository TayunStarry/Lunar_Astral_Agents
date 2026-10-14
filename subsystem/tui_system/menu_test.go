package TuiSystem

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestMenuMappingProbe 逐条校验菜单：Enter 进入的屏幕、快捷键直达的屏幕
// 都必须与条目表声明的 screen 一致（守卫「排版与功能对应不上」）
func TestMenuMappingProbe(t *testing.T) {
	for i, item := range menuItems {
		m := newAppModel()
		m.width, m.height = 100, 30
		m.menu = i
		cmd := m.menuAction()

		if item.screen == screenMenu { // 哨兵=退出程序
			if cmd == nil {
				t.Fatalf("第 %d 项「%s」应返回退出命令", i, item.title)
			}
			continue
		}
		if m.screen != item.screen {
			t.Fatalf("第 %d 项「%s」进入屏幕不符: 期望 screen=%d 实际 screen=%d",
				i, item.title, item.screen, m.screen)
		}
		t.Logf("[%s] %-14s → screen=%d", item.key, item.title, m.screen)
	}

	// 快捷键直达
	for _, item := range menuItems {
		if item.key == "" || item.screen == screenMenu {
			continue
		}
		m := newAppModel()
		m.width, m.height = 100, 30
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(item.key)})
		got := nm.(appModel)
		if got.screen != item.screen {
			t.Fatalf("快捷键 %q（%s）进入屏幕不符: 期望 %d 实际 %d",
				item.key, item.title, item.screen, got.screen)
		}
	}
}
