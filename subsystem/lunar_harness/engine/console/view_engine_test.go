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

// 覆盖：运行中按 x 请求中断；桌面操纵空闲时 x 与其他屏一致退出。
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

	model, cmd := m.updateStats(keyMsg("x"))
	m = model.(appModel)
	if stopped != 1 {
		t.Fatalf("运行中按 x 应调用中断钩子，实得 %d", stopped)
	}
	if isQuitCmd(cmd) {
		t.Fatal("运行中按 x 应保留中断语义，不应退出程序")
	}

	running = false
	if _, cmd := m.updateStats(keyMsg("x")); !isQuitCmd(cmd) {
		t.Fatal("桌面操纵空闲时按 x 应退出程序")
	}
}

// isQuitCmd 判断命令是否为 tea.Quit。
func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// 覆盖：x/X（界面显示 X）在所有非输入态屏幕都能退出程序（不再只有主菜单生效）。
func TestQuitKeyAcrossScreens(t *testing.T) {
	// 未接入桌面操纵钩子：运行状态屏没有本地 x 语义，应与其他屏一致退出
	SetDesktopOperatorHooks(nil, nil, nil)
	defer SetDesktopOperatorHooks(nil, nil, nil)

	for _, id := range []screenID{screenMenu, screenAgent, screenStats, screenLogs, screenArchive, screenDump, screenTools} {
		for _, k := range []string{"x", "X"} {
			m := newAppModel()
			m.width, m.height = 120, 40
			m.screen = id
			if _, cmd := m.Update(keyMsg(k)); !isQuitCmd(cmd) {
				t.Fatalf("屏幕 %d 按 %q 应退出程序", id, k)
			}
		}
	}
}

// 覆盖：输入框正在接收字符时 x 让位，不触发退出。
func TestQuitKeyYieldsToTextEntry(t *testing.T) {
	t.Setenv(colorModeEnv, "")
	SetDesktopOperatorHooks(nil, nil, nil)
	defer SetDesktopOperatorHooks(nil, nil, nil)

	stats := newAppModel()
	stats.width, stats.height = 120, 40
	stats.screen = screenStats
	stats.engine.inputMode = true
	if _, cmd := stats.Update(keyMsg("x")); isQuitCmd(cmd) {
		t.Fatal("运行状态输入态按 x 应作为输入，不应退出")
	}

	archive := newAppModel()
	archive.width, archive.height = 120, 40
	archive.screen = screenArchive
	archive.archive.inputMode = true
	if _, cmd := archive.Update(keyMsg("x")); isQuitCmd(cmd) {
		t.Fatal("打包归档输入态按 x 应作为输入，不应退出")
	}

	for _, id := range []screenID{screenPorts, screenProxy} {
		m := newAppModel()
		m.width, m.height = 120, 40
		m.screen = id
		if _, cmd := m.Update(keyMsg("x")); !isQuitCmd(cmd) {
			t.Fatalf("屏幕 %d 的数字输入框不应吞掉退出键 x", id)
		}
	}
}

// 覆盖：未接入钩子时，屏上要明确说明（不能让人以为按了没反应）。
func TestEngineScreenDesktopOperatorWithoutHooks(t *testing.T) {
	SetDesktopOperatorHooks(nil, nil, nil)

	m := newAppModel()
	m.width, m.height = 120, 46
	m.screen = screenStats
	out := m.View()
	if !strings.Contains(out, "桌面操作") || !strings.Contains(out, "宿主未接入桌面操作") {
		t.Fatalf("未接入桌面操作钩子时应明确提示:\n%s", out)
	}
}
