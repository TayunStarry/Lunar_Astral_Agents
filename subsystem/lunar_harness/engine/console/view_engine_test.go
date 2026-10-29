package console

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// keyMsg 构造一个字符按键（bubbletea v1 的 KeyMsg 是 Key 的别名）
func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// 覆盖：桌面操纵的唯一人工入口——输入要求后启动，并可随时中断。
func TestEngineScreenDesktopOperatorManualStart(t *testing.T) {
	var started []string
	SetDesktopOperatorHooks(func(instruction string) error {
		started = append(started, instruction)
		return nil
	}, func() bool { return false }, func() DesktopOperatorStatus { return DesktopOperatorStatus{} })
	defer SetDesktopOperatorHooks(nil, nil, nil)

	m := newAppModel()
	m.width, m.height = 120, 40
	m.screen = screenStats

	model, _ := m.updateEngine(keyMsg("i"))
	m = model.(appModel)
	if !m.engine.inputMode {
		t.Fatal("按 i 应进入桌面操纵任务要求输入态")
	}
	for _, r := range "打开记事本" {
		model, _ = m.updateEngine(keyMsg(string(r)))
		m = model.(appModel)
	}
	model, _ = m.updateEngine(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(appModel)
	if len(started) != 1 || started[0] != "打开记事本" {
		t.Fatalf("Enter 应把输入的要求交给启动钩子，实得 %#v", started)
	}
	if m.engine.inputMode || m.engine.err {
		t.Fatalf("启动后应退出输入态且无错误: input=%v err=%v", m.engine.inputMode, m.engine.err)
	}
}

// 覆盖：运行中按 x 请求中断；未运行时不应误触中断。
func TestEngineScreenDesktopOperatorInterrupt(t *testing.T) {
	stopped := 0
	running := true
	SetDesktopOperatorHooks(func(string) error { return nil }, func() bool { stopped++; return true }, func() DesktopOperatorStatus {
		return DesktopOperatorStatus{Running: running}
	})
	defer SetDesktopOperatorHooks(nil, nil, nil)

	m := newAppModel()
	m.width, m.height = 120, 40
	m.screen = screenStats

	model, _ := m.updateEngine(keyMsg("x"))
	m = model.(appModel)
	if stopped != 1 {
		t.Fatalf("运行中按 x 应调用中断钩子，实得 %d", stopped)
	}

	running = false
	model, _ = m.updateEngine(keyMsg("x"))
	m = model.(appModel)
	if stopped != 1 {
		t.Fatalf("未运行时不应再次中断，实得 %d", stopped)
	}
	if !strings.Contains(m.engine.msg, "未运行") {
		t.Fatalf("未运行时应明确提示，实得 %q", m.engine.msg)
	}
}

// 覆盖：未接入钩子时，屏上要明确说明（不能让人以为按了没反应）。
func TestEngineScreenDesktopOperatorWithoutHooks(t *testing.T) {
	SetDesktopOperatorHooks(nil, nil, nil)

	m := newAppModel()
	m.width, m.height = 120, 46
	m.screen = screenStats
	out := m.View()
	if !strings.Contains(out, "桌面操纵") || !strings.Contains(out, "宿主未接入桌面操纵") {
		t.Fatalf("未接入桌面操纵钩子时应明确提示:\n%s", out)
	}
}
