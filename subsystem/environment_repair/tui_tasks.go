package main

import (
	"strconv"

	"LunarSubsystem/GeneralConfig"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// newPortInput 创建端口输入框
func newPortInput(placeholder string) textinput.Model {
	input := textinput.New()
	input.Placeholder = placeholder
	input.Width = 12
	input.CharLimit = 5
	return input
}

// parsePortInput 解析输入框端口（空则用默认值）
func parsePortInput(input textinput.Model, def int) (int, bool) {
	text := validatePortText(input.Value())
	if text == "" {
		return def, true
	}
	v, err := strconv.Atoi(text)
	if err != nil || v <= 0 || v >= 65536 {
		return 0, false
	}
	return v, true
}

// validatePortText 校验并整理端口文本（仅数字）
func validatePortText(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			out = append(out, r)
		}
	}
	return string(out)
}

// ==== 资源补全修复屏 ====

// repairState 资源修复屏状态
type repairState struct {
	started bool
	running bool
	err     error
	done    bool
}

func newRepairState() repairState { return repairState{} }

// startRepair 启动修复任务 goroutine（仅首次进入时执行）
func (m *appModel) startRepair() tea.Cmd {
	if m.repair.started {
		return nil
	}
	m.repair.started = true
	m.repair.running = true
	run := func() tea.Msg {
		err := EnsureLocalData()
		return taskDoneMsg{err: err}
	}
	return tea.Batch(run, m.spinner.Tick)
}

// updateRepair 修复屏事件
func (m appModel) updateRepair(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		case "r":
			// 重新执行修复
			m.repair = newRepairState()
			return m, m.startRepair()
		}

	case taskDoneMsg:
		m.repair.running = false
		m.repair.done = true
		m.repair.err = msg.err
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

// viewRepair 修复屏
func (m appModel) viewRepair() string {
	title := tuiTitleStyle.Render("[1] 资源补全修复")
	desc := tuiDimStyle.Render("检查 local_data 目录资源完整性，从内嵌资源中仅补全缺失文件（不覆盖已有文件）。")

	status := m.taskStatus(m.repair.running, "修复流程完成，可按 r 重新执行", m.repair.err)
	if m.repair.running {
		status = m.taskStatus(true, "正在检查并补全 local_data 资源...", nil)
	}

	body := tuiPanelStyle.Render(desc + "\n\n" + status)
	footer := tuiFooterStyle.Render("[Esc] 返回菜单   [r] 重新执行   详细日志见「日志查询」")

	return title + "\n" + body + "\n" + footer
}

// ==== 端口占用释放屏 ====

// portsState 端口释放屏状态
type portsState struct {
	activated bool
	start     textinput.Model
	end       textinput.Model
	focus     int // 0 起始 1 结束
	running   bool
	err       error
	done      bool
}

// newPortsState 构建端口释放屏状态（默认范围与 config 子系统默认值一致）
func newPortsState() portsState {
	return portsState{
		start: newPortInput(strconv.Itoa(*GeneralConfig.ProxyPort - 10)),
		end:   newPortInput(strconv.Itoa(*GeneralConfig.ProxyPort + 5)),
	}
}

// updatePorts 端口释放屏事件
func (m appModel) updatePorts(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		}

		if !m.ports.running {
			switch key.String() {
			case "up", "k":
				return m.movePortsFocus(-1)
			case "down", "j":
				return m.movePortsFocus(1)
			case "enter":
				return m, m.launchPortRelease()
			}

			// 未聚焦的输入框由 textinput 自身忽略按键，故仅向当前选中字段转发
			var cmd tea.Cmd
			if m.ports.focus == fieldPrimary {
				m.ports.start, cmd = m.ports.start.Update(msg)
			} else {
				m.ports.end, cmd = m.ports.end.Update(msg)
			}
			return m, cmd
		}
	}

	switch msg := msg.(type) {
	case taskDoneMsg:
		m.ports.running = false
		m.ports.done = true
		m.ports.err = msg.err
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

// movePortsFocus 在端口输入框之间环形移动焦点
// 注意：Focus/Blur 为指针接收器，必须在本方法内直接调用（m.ports 字段可寻址），
// 不能委托给只返回 tea.Cmd 的辅助方法，否则焦点修改会留在副本上被丢弃
func (m appModel) movePortsFocus(delta int) (tea.Model, tea.Cmd) {
	idx := (m.ports.focus + delta + fieldCount) % fieldCount
	m.applyPortsFocus(idx)
	return m, textinput.Blink
}

// applyPortsFocus 将焦点落到指定字段并同步 focus 索引
func (m *appModel) applyPortsFocus(idx int) {
	m.ports.focus = idx
	if idx == fieldPrimary {
		m.ports.start.Focus()
		m.ports.end.Blur()
	} else {
		m.ports.start.Blur()
		m.ports.end.Focus()
	}
}

// launchPortRelease 校验输入并启动端口释放任务
// 注意：必须使用指针接收器，否则 running/err 等状态修改会发生在副本上
func (m *appModel) launchPortRelease() tea.Cmd {
	startPort, okStart := parsePortInput(m.ports.start, *GeneralConfig.ProxyPort-10)
	endPort, okEnd := parsePortInput(m.ports.end, *GeneralConfig.ProxyPort+5)

	if !okStart || !okEnd {
		uiLogf("  [ERROR] 端口输入无效，请输入 1-65535 之间的数字")
		return nil
	}
	if endPort < startPort {
		uiLogf("  [ERROR] 结束端口不能小于起始端口")
		return nil
	}

	m.ports.running = true
	m.ports.done = false
	m.ports.err = nil
	m.ports.start.Blur()
	m.ports.end.Blur()

	run := func() tea.Msg {
		err := ExecutePortRelease(PortRange{Start: startPort, End: endPort})
		return taskDoneMsg{err: err}
	}
	return tea.Batch(run, m.spinner.Tick)
}

// viewPorts 端口释放屏
func (m appModel) viewPorts() string {
	title := tuiTitleStyle.Render("[2] 端口占用释放")

	inputs := renderFocusField("起始端口: ", m.ports.start, m.ports.focus == fieldPrimary) + "\n" +
		renderFocusField("结束端口: ", m.ports.end, m.ports.focus == fieldSecondary)

	var status string
	switch {
	case m.ports.running:
		status = m.taskStatus(true, "正在扫描并释放端口...", nil)
	case m.ports.done:
		status = m.taskStatus(false, "端口释放流程完成，可修改范围后按 Enter 再次执行", m.ports.err)
	default:
		status = tuiDimStyle.Render("↑/↓ 选择输入框，输入端口范围后按 Enter 开始扫描（留空使用默认范围）")
	}

	body := tuiPanelStyle.Render(inputs + "\n\n" + status)
	footer := tuiFooterStyle.Render("[↑/↓] 切换输入框   [Enter] 开始执行   [Esc] 返回菜单   详细日志见「日志查询」")

	return title + "\n" + body + "\n" + footer
}
