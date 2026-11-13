package console

// tui_update.go 控制台 TUI 的事件分发、全局输入处理与屏间切换。

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Update 事件分发
func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// 首帧尺寸自检写入原始日志：远端报「界面一直向上滚 / 显示混乱」时，
		// 这一行直接说明「程序以为终端多大、打算画几行」——是判断尺寸失配还是字形宽度失配的关键。
		if !m.sizeLogged {
			m.sizeLogged = true
			pushScrubbedLog(SourceRaw, "RAW", "TUI", "", fmt.Sprintf(
				"首帧尺寸自检: 终端报 %d×%d，框体渲染 %d 行（≤%d 列，末列留空）",
				msg.Width, msg.Height, msg.Height, maxInt(1, msg.Width-1)))
			// 首帧已经进入渲染循环：通知宿主控制台界面就绪。
			markScreenReady()
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case notifyMsg, streamEvtMsg, taskLogMsg:
		// 日志到达：刷新当前屏的滚动度量（View 时直接取缓存内容），仅触发重绘
		m.syncViewMetrics()
		return m, nil

	case statsTickMsg:
		m.stats.snap = GetSnapshot()
		m.syncViewMetrics()
		return m, tickStats()

	case progressMsg:
		m.applyProgress(int(msg))
		return m, nil

	case frontendDoneMsg:
		switch msg.which {
		case "lunar":
			m.frontend.busyLunar = false
		case "crystal":
			m.frontend.busyCrystal = false
		case "library":
			m.frontend.busyLibrary = false
		}
		if msg.err != nil {
			m.frontend.msg = errStyle.Render("✗ 唤起失败: " + msg.err.Error())
			m.setNotice("前端唤起失败", true)
		} else {
			m.frontend.msg = okStyle.Render("✓ 已发出唤起请求，等待窗口接入")
			m.setNotice("已请求唤起前端窗口", false)
		}

	case proxyReadyMsg:
		m.proxy.running = true
		m.proxy.handle = msg.token
		m.proxy.info = msg.info
		m.proxy.errText = ""
		m.setNotice("HTTPS 代理已启动", false)

	case proxyFailMsg:
		m.proxy.errText = msg.err.Error()
		m.setNotice("代理启动失败", true)

	case proxyStoppedMsg:
		m.proxy.stopping = false
		m.setNotice("HTTPS 代理已停止", false)
		return m, nil
	}

	// 刷新滚动度量（窗口尺寸变化、切屏、内容变化后按键才拿得到真实行数）
	m.syncViewMetrics()

	// 全局键位（帮助页为模态层，吞掉其余按键，避免误触发底层屏幕）
	if key, ok := msg.(tea.KeyMsg); ok {
		if handled, cmd := m.handleGlobalKey(key); handled {
			return m, cmd
		}
	}

	switch m.screen {
	case screenMenu:
		return m.updateMenu(msg)
	case screenAgent:
		return m.updateAgent(msg)
	case screenStats:
		return m.updateStats(msg)
	case screenLogs:
		return m.updateLogs(msg)
	case screenProxy:
		return m.updateProxy(msg)
	case screenArchive:
		return m.updateArchive(msg)
	case screenPorts:
		return m.updatePorts(msg)
	case screenDump:
		return m.updateDump(msg)
	case screenTools:
		return m.updateTools(msg)
	}
	return m, nil
}

// handleGlobalKey 全局键位：退出、帮助开关与屏间切换；返回是否已消费该键
func (m *appModel) handleGlobalKey(key tea.KeyMsg) (bool, tea.Cmd) {
	k := key.String()
	if k == "ctrl+c" {
		return true, tea.Quit
	}
	if m.help {
		switch k {
		case "?", "esc":
			m.help = false
		}
		return true, nil // 模态：其余按键一律吞掉
	}
	// 输入框聚焦时 ? 属于正文字符，不触发帮助
	if k == "?" && !m.inputFocused() {
		m.help = true
		return true, nil
	}
	// x/X 退出（界面显示 X）：除「运行状态」屏外全部在全局处理，避免每个屏各写一份。
	// 输入框正在接收字符时让位（x 是合法正文）；「运行状态」屏在桌面操纵
	// 运行时把 x 留给「中断 / 强制结束」，由 updateStats 判断是否退出。
	if (k == "x" || k == "X") && !m.textEntryActive() && m.screen != screenStats {
		return true, tea.Quit
	}
	// 屏间顺序切换：任何屏都可用，不依赖「先回主菜单再进」。
	// 文本框聚焦时让位（[ ] 也是合法正文），其余屏一律接管。
	if (k == "[" || k == "]") && !m.inputFocused() {
		delta := 1
		if k == "[" {
			delta = -1
		}
		return true, m.switchScreen(delta)
	}
	// PgUp/PgDn 作为「快速换页」：按侧栏顺序切上一屏/下一屏，无需先回主菜单。
	// 屏内翻页改由 Ctrl+U / Ctrl+D 承担（见 list.go 的 scrollKey / listNavKey）。
	if (k == "pgup" || k == "pgdown") && !m.inputFocused() {
		delta := 1
		if k == "pgup" {
			delta = -1
		}
		return true, m.switchScreen(delta)
	}
	return false, nil
}

// inputFocused 当前屏是否存在正在接收字符的输入框：
// 此时快捷键让位于文本输入，避免「打字即触发功能」。
// 端口一类受控输入（数字、长度受限）不在此列——那里的 ? 更可能是想开帮助，
// 而不是往端口号里塞一个非法字符。
func (m appModel) inputFocused() bool {
	switch m.screen {
	case screenArchive:
		return m.archive.inputMode
	case screenStats:
		return m.engine.inputMode
	}
	return false
}

// textEntryActive 当前屏是否有「会接收正文」的输入框（供退出键让位）。
//
// 只认自由文本输入：端口/代理屏的数字框虽然常驻聚焦，但 x 不是端口字符，
// 那里按 x 更可能是想退出，而不是往端口号里塞一个非法字符——与 inputFocused
// 对 ? / [ ] 的取舍保持一致。
func (m appModel) textEntryActive() bool {
	switch m.screen {
	case screenStats:
		return m.engine.inputMode
	case screenArchive:
		return m.archive.inputMode
	}
	return false
}

// applyProgress 把任务进度写回发起任务的屏幕
func (m *appModel) applyProgress(percent int) {
	switch m.screen {
	case screenArchive:
		m.archive.progress = percent
	case screenProxy:
		m.proxy.progress = percent
	}
}

// updateMenu 菜单屏事件
func (m appModel) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "up", "k":
		m.menu = listMove(m.menu, -1, len(menuItems), true)
	case "down", "j":
		m.menu = listMove(m.menu, 1, len(menuItems), true)
	// 首尾跳转只保留 Home/End：g/G 留给列表屏的翻页语义，
	// 主菜单不另设别名，避免同一按键在不同屏含义摇摆。
	case "home":
		m.menu = 0
	case "end":
		m.menu = len(menuItems) - 1
	case "enter", "right":
		return m, m.menuAction()
	default:
		// 快捷键直达对应条目（条目表内显式声明，避免索引推算错位）
		if idx := menuHotkeyIndex(key.String()); idx >= 0 {
			m.menu = idx
			return m, m.menuAction()
		}
	}
	return m, nil
}

// menuAction 进入当前选中条目对应的屏幕（依条目 screen 字段分发）
func (m *appModel) menuAction() tea.Cmd {
	item := menuItems[m.menu]
	if item.screen == screenMenu {
		return tea.Quit // 末项「退出程序」以 screenMenu 作哨兵
	}
	return m.enterScreen(item.screen)
}

// enterScreen 进入指定屏幕并完成该屏的进入动作。
//
// 三条入口（主菜单回车 / 侧栏 / [ ] 顺序切换）共用这一份实现：
// 进入某屏要做的初始化（重置日志窗口、拉快照、聚焦输入框…）若分散在各入口，
// 迟早出现「从菜单进正常、从别处进就古怪」的裂缝。
func (m *appModel) enterScreen(id screenID) tea.Cmd {
	m.screen = id
	m.help = false
	// 返回主菜单时高亮当前屏所在条目：位置感由这一处保底（侧栏收起时尤其重要）
	if idx := menuIndexOf(id); idx >= 0 {
		m.menu = idx
	}

	switch id {
	case screenMenu:
		return nil
	case screenStats:
		m.stats.snap = GetSnapshot()
		return tickStats()
	case screenLogs:
		m.logView.reset()
	case screenProxy:
		return m.proxyEnter()
	case screenArchive:
		return m.archiveEnter()
	case screenPorts:
		m.ports.activated = true
		m.applyPortsFocus(0)
		return textinput.Blink
	case screenDump:
		return m.dumpEnter()
	case screenTools:
		return m.toolsEnter()
	}
	return nil
}

// switchScreen 按侧栏顺序（= 主菜单顺序）切换到上/下一屏。
// 不在任何功能屏时（主菜单），向前即首屏、向后即末屏——与翻页的直觉一致。
func (m *appModel) switchScreen(delta int) tea.Cmd {
	ids := navigableScreens()
	if len(ids) == 0 {
		return nil
	}
	cur := -1
	for i, id := range ids {
		if id == m.screen {
			cur = i
			break
		}
	}
	next := 0
	switch {
	case cur < 0 && delta < 0:
		next = len(ids) - 1
	case cur >= 0:
		next = ((cur+delta)%len(ids) + len(ids)) % len(ids)
	}
	return m.enterScreen(ids[next])
}
