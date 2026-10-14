package TuiSystem

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestTokenInputAcceptsRunes 驱动更新包屏的令牌输入态：
// 按 1 进入 → 逐字符输入 → 断言输入框累积到完整令牌。
func TestTokenInputAcceptsRunes(t *testing.T) {
	m := newAppModel()
	m.width, m.height = 100, 30
	m.screen = screenUpdate
	m.menu = 9
	_ = m.updateEnter()

	// 菜单态按 1 → 进入令牌输入态
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = nm.(appModel)
	if m.update.mode != updateInputToken {
		t.Fatalf("按 1 应进入令牌输入态，实际 mode=%d", m.update.mode)
	}
	t.Logf("进入令牌输入态，placeholder=%q", m.update.input.Placeholder)

	// 逐字符输入
	for _, r := range "f9bdbc7d" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(appModel)
	}
	got := m.update.input.Value()
	t.Logf("输入后 value=%q（密码模式回显）view=%q", got, m.update.input.View())

	// 隔离诊断：单独驱动 textinput 是否接收字符
	ti := m.update.input
	ti2, _ := ti.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	t.Logf("隔离 textinput: value=%q", ti2.Value())

	if got != "f9bdbc7d" {
		t.Fatalf("输入框未累积字符: 期望 %q 实际 %q", "f9bdbc7d", got)
	}
	if !m.update.input.Focused() {
		t.Log("提示：输入框未 Focus（不影响字符接收，但光标不闪烁）")
	}
}

// TestArchiveCustomPathInput 验证打包归档屏 [P] 自定义配置路径输入框可接收字符
func TestArchiveCustomPathInput(t *testing.T) {
	m := newAppModel()
	m.width, m.height = 100, 30
	m.screen = screenArchive
	_ = m.archiveEnter()

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = nm.(appModel)
	if !m.archive.inputMode {
		t.Fatalf("按 p 应进入自定义路径输入态")
	}
	for _, r := range "local_data/archive_config.json" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(appModel)
	}
	if got := m.archive.input.Value(); got != "local_data/archive_config.json" {
		t.Fatalf("归档屏输入框未累积字符: 期望 %q 实际 %q", "local_data/archive_config.json", got)
	}
	t.Logf("归档屏自定义路径输入正常: %q", m.archive.input.Value())
}
