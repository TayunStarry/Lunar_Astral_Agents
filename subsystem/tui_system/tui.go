package TuiSystem

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/skip2/go-qrcode"

	"LunarSubsystem/TuiSystem/archive"
	"LunarSubsystem/TuiSystem/portrelease"
	"LunarSubsystem/TuiSystem/proxy"
	"LunarSubsystem/TuiSystem/updatepack"
)

// ==== 消息定义 ====

// notifyMsg 日志管线唤醒信号（日志/流事件到达）
type notifyMsg struct{}

// statsTickMsg 运行状态采样刷新
type statsTickMsg struct{}

// streamEvtMsg 智能体流式事件
type streamEvtMsg struct{ evt StreamEvent }

// taskDoneMsg 后台任务完成（ports/archive/update/proxy 等共用）
type taskDoneMsg struct{ err error }

// progressMsg 任务进度（-1 表示准备阶段未知进度）
type progressMsg int

// taskLogMsg 功能任务日志行（功能屏内嵌回显）
type taskLogMsg string

// frontendDoneMsg 前端唤起任务完成
type frontendDoneMsg struct {
	which string // "lunar" / "crystal"
	err   error
}

// proxyReadyMsg 代理服务启动成功
type proxyReadyMsg struct {
	info  proxy.Info
	token *proxy.Handle
}

// proxyFailMsg 代理服务启动失败
type proxyFailMsg struct{ err error }

// ==== 屏幕编号 ====

type screenID int

const (
	screenMenu screenID = iota
	screenAgent
	screenStats
	screenRaw
	screenLunarJS
	screenLTP9
	screenGoWarn
	screenFrontend
	screenProxy
	screenArchive
	screenUpdate
	screenPorts
)

// menuItemCount 主菜单条目数（11 个功能 + 退出）
const menuItemCount = 12

// ==== 样式 ====

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")).MarginBottom(1)
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	valueStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	panelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 2)
	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243")).MarginTop(1)
	selectStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	okStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82"))
	errStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("204"))

	logInfoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))  // 青色：常规
	logDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245")) // 灰色：stdout 原始行
	logProxyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))  // 蓝色：任务回显
	logWarnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // 橙色：警告
	logErrStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("204")) // 红色：错误
	thinkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("243")) // 思考流（暗色）
	roleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("176"))
)

// renderLogLine 单条日志渲染：按级别/来源着色
func renderLogLine(line LogLine) string {
	switch {
	case line.Level == "ERROR" || line.Level == "FATAL":
		return logErrStyle.Render(line.Text)
	case line.Level == "WARN":
		return logWarnStyle.Render(line.Text)
	case line.Level == "RAW":
		return logDimStyle.Render(line.Text)
	case line.Source == "task":
		return logProxyStyle.Render(line.Text)
	default:
		return logInfoStyle.Render(line.Text)
	}
}

// progressBar 渲染进度条
func progressBar(percent int, width int) string {
	if width < 10 {
		width = 10
	}
	if width > 50 {
		width = 50
	}
	filled := percent * width / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return valueStyle.Render(fmt.Sprintf("[%s] %3d%%", bar, percent))
}

// generateQRText 生成终端半块字符二维码
func generateQRText(url string) string {
	qr, err := qrcode.New(url, qrcode.Low)
	if err != nil {
		return ""
	}
	qr.DisableBorder = true
	return qr.ToSmallString(false)
}

// ==== 应用主模型 ====

// appModel 全应用模型：屏幕切换 + 各屏状态聚合
type appModel struct {
	screen screenID
	menu   int // 主菜单选中项

	width, height int

	spinner spinner.Model

	rawView     logsState // [3] 原始日志
	lunarView   logsState // [4] 月华 JS 日志
	ltp9View    logsState // [5] ltp9 JS 日志
	warnView    logsState // [6] Go 警告错误
	agent       agentState
	stats       statsState
	frontend    frontendState
	proxy       proxyState
	archive     archiveState
	update      updateState
	ports       portsState
	streamUnsub func() // 流式事件订阅注销
}

// newAppModel 构建初始模型
func newAppModel() appModel {
	return appModel{
		screen:    screenMenu,
		spinner:   newSpinner(),
		rawView:   newLogsState(SourceRaw),
		lunarView: newLogsState(SourceLunarJS),
		ltp9View:  newLogsState(SourceLTP9JS),
		warnView:  newLogsState(SourceGoWarn),
		proxy:     newProxyState(),
		archive:   newArchiveState(),
		update:    newUpdateState(),
		ports:     newPortsState(),
	}
}

// newSpinner 创建加载动画
func newSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = selectStyle
	return s
}

// Init 初始化：启动状态采样 tick 与加载动画
func (m appModel) Init() tea.Cmd {
	return tea.Batch(tickStats(), m.spinner.Tick)
}

// tickStats 生成状态刷新命令
func tickStats() tea.Cmd {
	return tea.Every(statsInterval, func(time.Time) tea.Msg {
		return statsTickMsg{}
	})
}

// Update 事件分发
func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case notifyMsg:
		// 日志到达：日志视图按需刷新（View 时拉取），此处仅触发重绘
		return m, nil

	case statsTickMsg:
		m.stats.snap = GetSnapshot()
		return m, tickStats()

	case streamEvtMsg:
		return m, nil

	case taskDoneMsg:
		// 交给当前屏幕处理
	case progressMsg:
		switch m.screen {
		case screenArchive:
			m.archive.progress = int(msg)
		case screenUpdate:
			m.update.progress = int(msg)
		case screenProxy:
			m.proxy.progress = int(msg)
		}
	case taskLogMsg:
		// 日志已入管线缓冲，此处仅触发重绘
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
	case proxyReadyMsg:
		m.proxy.running = true
		m.proxy.handle = msg.token
		m.proxy.info = msg.info
		m.proxy.errText = ""
	case proxyFailMsg:
		m.proxy.errText = msg.err.Error()
	}

	switch m.screen {
	case screenMenu:
		return m.updateMenu(msg)
	case screenAgent:
		return m.updateAgent(msg)
	case screenStats:
		return m.updateStats(msg)
	case screenRaw:
		return m.updateLogs(&m.rawView, msg)
	case screenLunarJS:
		return m.updateLogs(&m.lunarView, msg)
	case screenLTP9:
		return m.updateLogs(&m.ltp9View, msg)
	case screenGoWarn:
		return m.updateLogs(&m.warnView, msg)
	case screenFrontend:
		return m.updateFrontend(msg)
	case screenProxy:
		return m.updateProxy(msg)
	case screenArchive:
		return m.updateArchive(msg)
	case screenUpdate:
		return m.updateUpdate(msg)
	case screenPorts:
		return m.updatePorts(msg)
	}
	return m, nil
}

// updateMenu 菜单屏事件
func (m appModel) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "e":
		return m, tea.Quit
	case "up", "k":
		m.menu = (m.menu + menuItemCount - 1) % menuItemCount
	case "down", "j":
		m.menu = (m.menu + 1) % menuItemCount
	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "q":
		m.menu = menuIndexFromKey(key.String())
		return m, m.menuAction()
	case "enter":
		return m, m.menuAction()
	}
	return m, nil
}

// menuIndexFromKey 按键 → 菜单索引（1-9 直映，0=第10项 更新包，q=第11项 端口释放）
func menuIndexFromKey(k string) int {
	switch k {
	case "0":
		return 9
	case "q":
		return 10
	default:
		return int(k[0] - '1')
	}
}

// menuAction 执行当前选中菜单项（最后一项为退出；指针接收器保证 screen 切换生效）
func (m *appModel) menuAction() tea.Cmd {
	switch m.menu {
	case 0:
		m.screen = screenAgent
		return nil
	case 1:
		m.screen = screenStats
		m.stats.snap = GetSnapshot()
		return tickStats()
	case 2:
		m.screen = screenRaw
		m.rawView.refresh(m.width, m.height)
		return nil
	case 3:
		m.screen = screenLunarJS
		m.lunarView.refresh(m.width, m.height)
		return nil
	case 4:
		m.screen = screenLTP9
		m.ltp9View.refresh(m.width, m.height)
		return nil
	case 5:
		m.screen = screenGoWarn
		m.warnView.refresh(m.width, m.height)
		return nil
	case 6:
		m.screen = screenFrontend
		m.frontend.msg = ""
		return nil
	case 7:
		m.screen = screenProxy
		return m.proxyEnter()
	case 8:
		m.screen = screenArchive
		return m.archiveEnter()
	case 9:
		m.screen = screenUpdate
		return m.updateEnter()
	case 10:
		m.screen = screenPorts
		m.ports.activated = true
		m.applyPortsFocus(0)
		return textinput.Blink
	default:
		return tea.Quit
	}
}

// quitRequested TUI 退出后的统一清理
func (m *appModel) quitCleanup() {
	if m.streamUnsub != nil {
		m.streamUnsub()
	}
	archive.SetLogHook(nil)
	archive.SetProgressHook(nil)
	proxy.SetLogHook(nil)
	portrelease.SetLogHook(nil)
	updatepack.SetLogHook(nil)
	updatepack.SetProgressHook(nil)
}

// Run 启动全屏 TUI 应用（阻塞直至用户退出；宿主需已先调用 Install）
func Run() error {
	model := newAppModel()

	prog := tea.NewProgram(model, tea.WithAltScreen(), tea.WithOutput(GetRealStdout()))

	// 流式事件订阅（TUI 激活期消费，退出时注销）
	streamCh, unsub := SubscribeStream()
	model.streamUnsub = unsub

	// 流式事件转发 goroutine（done 通道随退出关闭）
	done := make(chan struct{})
	go func() {
		for {
			select {
			case evt := <-streamCh:
				prog.Send(streamEvtMsg{evt: evt})
			case <-done:
				return
			}
		}
	}()

	// 日志管线唤醒转发 goroutine
	go func() {
		for range notifyCh {
			prog.Send(notifyMsg{})
		}
	}()

	// 功能子包钩子注入：日志并入统一管线，进度推送到 TUI
	taskLog := func(msg string) {
		PushTaskLog(msg)
		prog.Send(taskLogMsg(msg))
	}
	archive.SetLogHook(taskLog)
	archive.SetProgressHook(func(percent int) { prog.Send(progressMsg(percent)) })
	proxy.SetLogHook(taskLog)
	portrelease.SetLogHook(taskLog)
	updatepack.SetLogHook(taskLog)
	updatepack.SetProgressHook(func(percent int) { prog.Send(progressMsg(percent)) })

	// 终止信号 → 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		prog.Quit()
	}()

	_, err := prog.Run()
	close(done)
	model.quitCleanup()
	return err
}

// View 渲染分发
func (m appModel) View() string {
	if m.width == 0 {
		return "正在初始化界面..."
	}

	switch m.screen {
	case screenMenu:
		return m.viewMenu()
	case screenAgent:
		return m.viewAgent()
	case screenStats:
		return m.viewStats()
	case screenRaw:
		return m.viewLogs(&m.rawView, "原始日志")
	case screenLunarJS:
		return m.viewLogs(&m.lunarView, "月华智能体 JS 日志")
	case screenLTP9:
		return m.viewLogs(&m.ltp9View, "ltp9 JS 日志")
	case screenGoWarn:
		return m.viewLogs(&m.warnView, "Go 警告与错误")
	case screenFrontend:
		return m.viewFrontend()
	case screenProxy:
		return m.viewProxy()
	case screenArchive:
		return m.viewArchive()
	case screenUpdate:
		return m.viewUpdate()
	case screenPorts:
		return m.viewPorts()
	}
	return ""
}

// viewMenu 菜单屏
func (m appModel) viewMenu() string {
	items := []struct{ title, desc string }{
		{"智能体实况", "月华思考/输出/工具调用流式显示（对话者/绘制者/演奏者/记忆者…）"},
		{"运行状态", "运行时间 / 资源占用 / token 消耗总量 / 均次消耗 / 输出速度 / 平均上下文"},
		{"原始日志", "终端全量原始行（stdout 拦截 + 结构化日志合流）"},
		{"月华 JS 日志", "月华智能体 JavaScript 层日志"},
		{"ltp9 JS 日志", "LTP9 沙箱 JavaScript 层日志"},
		{"Go 警告错误", "Go 模块 WARN / ERROR / FATAL 日志"},
		{"前端管理", "月华/琉璃桌面窗口状态探测与意外关闭重启"},
		{"HTTPS 代理", "WSS→WS 反向代理，支持 IP 选择与手机扫码接入"},
		{"打包归档", "项目文件 7z 分卷打包，支持进度显示"},
		{"更新包", "发布更新包到 Gitee / 拉取更新包（最新 / tag / commit 哈希）"},
		{"端口释放", "扫描指定端口范围，终止占用进程并验证释放"},
		{"退出程序", "感谢使用「星月智能 · 控制台」"},
	}

	var rows []string
	for i, item := range items {
		hotkey := fmt.Sprintf("%d", i+1)
		switch i {
		case 9:
			hotkey = "0"
		case 10:
			hotkey = "q"
		}
		cursor := "  "
		title := labelStyle.Render("[" + hotkey + "] " + item.title)
		if i == m.menu {
			cursor = selectStyle.Render("▸ ")
			title = selectStyle.Render("[" + hotkey + "] " + item.title)
		}
		rows = append(rows, cursor+title+"  "+dimStyle.Render(item.desc))
	}

	menuPanel := panelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	title := titleStyle.Render("✦ 星月智能 · 控制台 ✦")
	footer := footerStyle.Render("[↑/↓] 选择   [Enter] 进入   [1-9/0/q] 快捷键   [e] 退出")

	return title + "\n" + menuPanel + "\n" + footer
}

// ---- 共享渲染组件 ----

// taskStatus 渲染任务状态行（spinner + 文案）
func taskStatus(m appModel, running bool, text string, err error) string {
	if running {
		return m.spinner.View() + " " + valueStyle.Render(text)
	}
	if err != nil {
		return errStyle.Render("✗ " + text + ": " + err.Error())
	}
	return okStyle.Render("✓ " + text)
}

// renderFocusField 渲染可聚焦输入项：选中项高亮并带 ▸ 标记，未选中项灰显
func renderFocusField(label string, input textinput.Model, focused bool) string {
	if focused {
		return selectStyle.Render("▸ ") + selectStyle.Render(label) + input.View()
	}
	return "  " + dimStyle.Render(label) + input.View()
}

// viewportOf 构建指定尺寸的日志视口
func viewportOf(lines []string, width, height int) string {
	vp := viewport.New(width, height)
	vp.SetContent(strings.Join(lines, "\n"))
	vp.GotoBottom()
	return vp.View()
}
