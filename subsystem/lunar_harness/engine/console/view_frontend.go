package console

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"LunarSubsystem/LunarHarness/engine/console/portrelease"
)

// ==== 前端管理屏 ====

// frontendState 前端管理屏状态
type frontendState struct {
	busyLunar   bool
	busyCrystal bool
	msg         string
	idx         int // 选中项：0=月华 1=琉璃
	list        listPane
}

// frontendOnline 读取宿主注入的在线探针（未注入视为未知）
func frontendOnline(probe func() bool) (online bool, known bool) {
	if probe == nil {
		return false, false
	}
	return probe(), true
}

// frontendKeys 前端管理屏键位
func frontendKeys() []keyHint {
	return []keyHint{
		kh("↑/↓", "选择"),
		kh("Enter", "重启选中前端"),
		kh("1 / 2", "直接重启月华 / 琉璃"),
		kh("Esc", "返回菜单"),
	}
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
		case "1":
			m.frontend.idx = 0
			return m.frontendRelaunch("lunar")
		case "2":
			m.frontend.idx = 1
			return m.frontendRelaunch("crystal")
		case "enter":
			if m.frontend.idx == 0 {
				return m.frontendRelaunch("lunar")
			}
			return m.frontendRelaunch("crystal")
		}
		if idx, ok := listNavKey(m.frontend.idx, navAlias(msg.String()), 2, 2, true); ok {
			m.frontend.idx = idx
		}
	}
	return m, nil
}

// frontendRelaunch 唤起指定前端（忙碌中忽略重复请求）
func (m appModel) frontendRelaunch(which string) (tea.Model, tea.Cmd) {
	if which == "lunar" {
		if m.frontend.busyLunar {
			return m, nil
		}
		m.frontend.busyLunar = true
	} else {
		if m.frontend.busyCrystal {
			return m, nil
		}
		m.frontend.busyCrystal = true
	}
	m.frontend.msg = ""
	m.setBusy("唤起前端窗口")
	return m, relaunchFrontendCmd(which)
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
		return dimStyle.Render("？ 未知")
	}
	if online {
		return okStyle.Render("● 在线")
	}
	return errStyle.Render("○ 离线")
}

// viewFrontend 前端管理屏渲染：可选中列表 + 运行状态
func (m appModel) viewFrontend() string {
	f := m.frameFor("前端管理", "", frontendKeys())
	w, h := f.bodySize()

	lunarOnline, lunarKnown := frontendOnline(statusLunar)
	crystalOnline, crystalKnown := frontendOnline(statusCrystal)

	entries := []struct {
		key    string
		title  string
		detail string
		busy   bool
		badge  string
	}{
		{"1", "月华前端", "消息终端（/）", m.frontend.busyLunar, statusBadge(lunarOnline, lunarKnown)},
		{"2", "琉璃前端", "桌面工作台（/desktop/）", m.frontend.busyCrystal, statusBadge(crystalOnline, crystalKnown)},
	}

	rowW := maxInt(8, w-listGutter)
	blocks := make([][]string, len(entries))
	for i, e := range entries {
		sel := i == m.frontend.idx
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		title := fmt.Sprintf("[%s] 重启%s", e.key, e.title)
		if sel {
			title = selectStyle.Render(title)
		} else {
			title = labelStyle.Render(title)
		}
		right := e.badge
		if e.busy {
			title = selectStyle.Render(m.spinner.View() + " 正在唤起" + e.title + "…")
			right = warnStyle.Render("处理中")
		}
		blocks[i] = []string{
			listRow(rowW, sel, cursor, title, right),
			newRow(rowW, sel).L("     ", dimStyle.Render(e.detail)).String(),
		}
	}

	head := []string{
		sectionRule("桌面窗口", w),
		dimStyle.Render("  前端窗口意外关闭后可在此重新唤起；在线状态由宿主探针实时提供"),
		"",
	}
	idx := clampInt(m.frontend.idx, 0, len(entries)-1)
	lines, _ := m.frontend.list.layoutView(blocks, idx, w, maxInt(1, h-len(head)-2))

	body := append([]string{}, head...)
	body = append(body, lines...)
	body = append(body, "")
	if m.frontend.msg != "" {
		body = append(body, "  "+m.frontend.msg)
	} else {
		body = append(body, "  "+dimStyle.Render("选中后回车即重新唤起对应窗口"))
	}
	return f.render(fitLines(body, h))
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

// portsKeys 端口释放屏键位
func portsKeys() []keyHint {
	return []keyHint{
		kh("↑/↓ Tab", "切换输入框"),
		kh("Enter", "开始扫描并释放"),
		kh("Esc", "返回菜单"),
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
		case "up", "left", "down", "right", "tab":
			m.applyPortsFocus((m.ports.focus + 1) % 2)
			return m, textinput.Blink
		case "enter":
			if !m.ports.running {
				m.ports.running = true
				m.ports.err = nil
				m.ports.doneMsg = ""
				m.setBusy("扫描并释放端口")
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

// updatePortsDone 完成分支处理
func (m appModel) updatePortsDone(err error) (tea.Model, tea.Cmd) {
	m.ports.running = false
	if err != nil {
		m.ports.err = err
		m.setNotice("端口释放失败", true)
	} else {
		m.ports.err = nil
		m.ports.doneMsg = "端口范围释放完成"
		m.setNotice("端口范围释放完成", false)
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
	f := m.frameFor("端口释放", "", portsKeys())
	w, h := f.bodySize()

	keyW := 10
	boxW := minInt(w, 60)
	form := []string{
		fieldRow("起始端口", m.ports.inputs[0], m.ports.focus == 0, keyW),
		fieldRow("结束端口", m.ports.inputs[1], m.ports.focus == 1, keyW),
	}

	body := []string{
		dimStyle.Render("  扫描并终止占用指定端口范围的进程（PowerShell + taskkill）"),
		"",
	}
	body = append(body, formBox(form, boxW)...)
	body = append(body, "")

	switch {
	case m.ports.running:
		body = append(body, "  "+selectStyle.Render(m.spinner.View())+" "+valueStyle.Render("正在扫描与释放端口…"))
	case m.ports.err != nil:
		body = append(body, "  "+errStyle.Render("✗ 释放失败: "+m.ports.err.Error()),
			"  "+dimStyle.Render("详细过程见「原始日志」"))
	case m.ports.doneMsg != "":
		body = append(body, "  "+okStyle.Render("✓ "+m.ports.doneMsg),
			"  "+dimStyle.Render("详细过程见「原始日志」"))
	default:
		body = append(body, "  "+dimStyle.Render("回车开始释放；只填起始端口即按单端口处理"))
	}
	return f.render(fitLines(body, h))
}
