package TuiSystem

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"LunarSubsystem/TuiSystem/portrelease"
)

// frontendState 前端管理屏状态
type frontendState struct {
	busyLunar    bool
	busyCrystal  bool
	msg          string
}

// frontendOnline 读取宿主注入的在线探针（未注入视为未知）
func frontendOnline(probe func() bool) (online bool, known bool) {
	if probe == nil {
		return false, false
	}
	return probe(), true
}

// updateFrontend 前端管理屏事件
func (m appModel) updateFrontend(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case frontendDoneMsg:
		switch msg.which {
		case "lunar":
			m.frontend.busyLunar = false
		case "crystal":
			m.frontend.busyCrystal = false
		}
		if msg.err != nil {
			m.frontend.msg = errStyle.Render("✗ 唤起失败: " + msg.err.Error())
		} else {
			m.frontend.msg = okStyle.Render("✓ 已发出唤起请求，等待窗口接入")
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		case "1", "up", "left":
			if !m.frontend.busyLunar {
				m.frontend.busyLunar = true
				m.frontend.msg = ""
				return m, relaunchFrontendCmd("lunar")
			}
		case "2", "down", "right":
			if !m.frontend.busyCrystal {
				m.frontend.busyCrystal = true
				m.frontend.msg = ""
				return m, relaunchFrontendCmd("crystal")
			}
		}
	}
	return m, nil
}

// relaunchFrontendCmd 后台唤起前端窗口
func relaunchFrontendCmd(which string) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		frontendStatusMu.RLock()
		var fn func() error
		if which == "lunar" {
			fn = relaunchLunar
		} else {
			fn = relaunchCrystal
		}
		frontendStatusMu.RUnlock()

		if fn == nil {
			return frontendDoneMsg{which: which, err: errNotWired}
		}
		return frontendDoneMsg{which: which, err: fn()}
	})
}

// statusBadge 前端在线状态徽标
func statusBadge(online, known bool) string {
	if !known {
		return dimStyle.Render("？未知")
	}
	if online {
		return okStyle.Render("● 在线")
	}
	return errStyle.Render("○ 离线")
}

// viewFrontend 前端管理屏渲染
func (m appModel) viewFrontend() string {
	lunarOnline, lunarKnown := frontendOnline(statusLunar)
	crystalOnline, crystalKnown := frontendOnline(statusCrystal)

	lunarRow := "  月华前端（/）        " + statusBadge(lunarOnline, lunarKnown)
	if m.frontend.busyLunar {
		lunarRow = m.spinner.View() + " 月华前端唤起中…"
	}
	crystalRow := "  琉璃前端（/desktop/） " + statusBadge(crystalOnline, crystalKnown)
	if m.frontend.busyCrystal {
		crystalRow = m.spinner.View() + " 琉璃前端唤起中…"
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		selectStyle.Render("[1] 重启月华前端"),
		lunarRow,
		"",
		selectStyle.Render("[2] 重启琉璃前端"),
		crystalRow,
	)
	if m.frontend.msg != "" {
		body += "\n\n" + m.frontend.msg
	}

	panel := panelStyle.Render(body)
	header := titleStyle.Render("「 前端管理 」")
	footer := footerStyle.Render("[1/2 或 ↑/↓] 重启对应前端   [Esc] 返回菜单")
	return header + "\n" + panel + "\n" + footer
}

// ==== 端口释放屏 ====

// portsState 端口释放屏状态
type portsState struct {
	inputs    [2]textinput.Model // 起始端口 / 结束端口
	focus     int
	activated bool // 首次进入时已初始化
	running   bool
	err       error
	doneMsg   string
}

// newPortsState 构建端口释放屏（默认范围 30000-30010）
func newPortsState() portsState {
	var s portsState
	s.inputs[0] = newPortInput("30000")
	s.inputs[1] = newPortInput("30010")
	return s
}

// newPortInput 创建端口输入框
func newPortInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Width = 8
	ti.CharLimit = 5
	return ti
}

// applyPortsFocus 设置端口释放屏的焦点输入框
func (m *appModel) applyPortsFocus(idx int) {
	m.ports.focus = idx
	for i := range m.ports.inputs {
		if i == idx {
			m.ports.inputs[i].Focus()
		} else {
			m.ports.inputs[i].Blur()
		}
	}
}

// updatePorts 端口释放屏事件
func (m appModel) updatePorts(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case portsDoneMsg:
		return m.updatePortsDone(msg.err)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.ports.running {
				return m, nil // 运行中不允许退出屏（后台任务继续）
			}
			m.screen = screenMenu
			return m, nil
		case "up", "left":
			m.applyPortsFocus((m.ports.focus + 1) % 2)
			return m, textinput.Blink
		case "down", "right", "tab":
			m.applyPortsFocus((m.ports.focus + 1) % 2)
			return m, textinput.Blink
		case "enter":
			if !m.ports.running {
				m.ports.running = true
				m.ports.err = nil
				m.ports.doneMsg = ""
				return m, runPortsCmd(m.ports.inputs[0].Value(), m.ports.inputs[1].Value())
			}
		}
		// 其余按键交给焦点输入框
		var cmd tea.Cmd
		m.ports.inputs[m.ports.focus], cmd = m.ports.inputs[m.ports.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

// runPortsCmd 后台执行端口释放
func runPortsCmd(startRaw, endRaw string) tea.Cmd {
	return func() tea.Msg {
		start, end, perr := parsePortRange(startRaw, endRaw)
		if perr != nil {
			return portsDoneMsg{err: perr}
		}
		return portsDoneMsg{err: portrelease.Execute(start, end)}
	}
}

// portsDoneMsg 端口释放完成
type portsDoneMsg struct{ err error }

// updatePorts 的完成分支处理（portsDoneMsg）
func (m appModel) updatePortsDone(err error) (tea.Model, tea.Cmd) {
	m.ports.running = false
	if err != nil {
		m.ports.err = err
	} else {
		m.ports.err = nil
		m.ports.doneMsg = "端口范围释放完成"
	}
	return m, nil
}

// parsePortRange 解析起止端口输入（容错：仅填起始时按单端口处理）
func parsePortRange(startRaw, endRaw string) (int, int, error) {
	start, err := strconv.Atoi(strings.TrimSpace(startRaw))
	if err != nil || start < 1 || start > 65535 {
		return 0, 0, fmt.Errorf("起始端口无效: %q", startRaw)
	}
	endRaw = strings.TrimSpace(endRaw)
	if endRaw == "" {
		return start, start, nil
	}
	end, err := strconv.Atoi(endRaw)
	if err != nil || end < 1 || end > 65535 || end < start {
		return 0, 0, fmt.Errorf("结束端口无效: %q", endRaw)
	}
	return start, end, nil
}

// viewPorts 端口释放屏渲染
func (m appModel) viewPorts() string {
	body := labelStyle.Render("扫描并终止占用指定端口范围的进程（PowerShell + taskkill）：") + "\n\n" +
		renderFocusField("起始端口 ", m.ports.inputs[0], m.ports.focus == 0) + "\n" +
		renderFocusField("结束端口 ", m.ports.inputs[1], m.ports.focus == 1)

	if m.ports.running {
		body += "\n\n" + m.spinner.View() + " " + valueStyle.Render("正在扫描与释放端口…")
	} else if m.ports.err != nil {
		body += "\n\n" + errStyle.Render("✗ 释放失败: "+m.ports.err.Error()) + "\n" +
			dimStyle.Render("详细过程见「原始日志」")
	} else if m.ports.doneMsg != "" {
		body += "\n\n" + okStyle.Render("✓ "+m.ports.doneMsg) + "\n" +
			dimStyle.Render("详细过程见「原始日志」")
	} else {
		body += "\n\n" + dimStyle.Render("[Enter] 开始释放   [↑/↓] 切换输入框")
	}

	panel := panelStyle.Render(body)
	header := titleStyle.Render("「 端口释放 」")
	footer := footerStyle.Render("[Esc] 返回菜单")
	return header + "\n" + panel + "\n" + footer
}
