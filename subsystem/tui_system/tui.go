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

// proxyStoppedMsg 代理服务已停止（后台收尾完成；离开代理屏后仍需生效，故在顶层处理）
type proxyStoppedMsg struct{}

// ==== 屏幕编号 ====

type screenID int

const (
	screenMenu screenID = iota
	screenAgent
	screenStats
	screenRaw
	screenLunarJS
	screenLTP9
	screenDraw
	screenGoWarn
	screenFrontend
	screenProxy
	screenArchive
	screenUpdate
	screenPorts
)

// menuItem 主菜单条目：快捷键与目标屏幕均显式声明，避免索引推算错位
type menuItem struct {
	// key 快捷键（单字符，空串=仅回车进入）
	key string
	// title 条目名称
	title string
	// desc 条目说明
	desc string
	// screen 目标屏幕；值为 screenMenu 时表示退出程序（哨兵）
	screen screenID
}

// menuItems 主菜单条目表（顺序即显示顺序，可自由调整排版；
// 进入哪个屏由 screen 字段决定，与显示顺序解耦——调整排版不会影响功能对应）
var menuItems = []menuItem{
	{"0", "资源更新 管理", "发布更新包到 Gitee / 拉取更新包（最新 / tag / commit 哈希）", screenUpdate},
	{"1", "核心智能 实况", "月华思考/输出/工具调用流式显示（对话者/绘制者/演奏者/记忆者…）", screenAgent},
	{"2", "运行状态 显示", "运行时间 / 资源占用 / token 消耗总量 / 均次消耗 / 输出速度 / 平均上下文", screenStats},
	{"3", "原始日志 显示", "终端全量原始行（stdout 拦截 + 结构化日志合流）", screenRaw},
	{"4", "月华引擎 日志", "月华智能体 JavaScript 层日志", screenLunarJS},
	{"5", "插件引擎 日志", "LTP9 沙箱 JavaScript 层日志", screenLTP9},
	{"6", "星月系统 警告", "Go 模块 WARN / ERROR / FATAL 日志", screenGoWarn},
	{"7", "绘图引擎 日志", "sd.cpp 绘图引擎输出（进度 / 采样速度 / 时间统计，按行排版）", screenDraw},
	{"8", "前端页面 管理", "月华/琉璃桌面窗口状态探测与意外关闭重启", screenFrontend},
	{"9", "局域共享 管理", "WSS → WS 反向代理，支持 IP 选择与手机扫码接入", screenProxy},
	{"a", "打包归档 管理", "项目文件 7z 分卷打包，支持进度显示", screenArchive},
	{"b", "端口释放 管理", "扫描指定端口范围，终止占用进程并验证释放", screenPorts},
	{"z", "退出程序", "感谢使用「星月智能 · 控制台」", screenMenu},
}

// menuHotkeyIndex 快捷键 → 菜单索引（未命中返回 -1）
func menuHotkeyIndex(k string) int {
	for i, it := range menuItems {
		if it.key != "" && it.key == k {
			return i
		}
	}
	return -1
}

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
	drawView    logsState // [7] sd.cpp 绘图引擎日志
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
	drawView := newLogsState(SourceDraw)
	drawView.renderLine = renderDrawLogLine // 绘图引擎日志按专用规则排版
	return appModel{
		screen:    screenMenu,
		spinner:   newSpinner(),
		rawView:   newLogsState(SourceRaw),
		lunarView: newLogsState(SourceLunarJS),
		ltp9View:  newLogsState(SourceLTP9JS),
		drawView:  drawView,
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
	case proxyStoppedMsg:
		m.proxy.stopping = false
		return m, nil
	}

	switch m.screen {
	case screenMenu:
		return m.updateMenu(msg)
	case screenAgent:
		return m.updateAgent(msg)
	case screenStats:
		return m.updateStats(msg)
	case screenRaw:
		return m.updateLogs(SourceRaw, msg)
	case screenLunarJS:
		return m.updateLogs(SourceLunarJS, msg)
	case screenLTP9:
		return m.updateLogs(SourceLTP9JS, msg)
	case screenDraw:
		return m.updateLogs(SourceDraw, msg)
	case screenGoWarn:
		return m.updateLogs(SourceGoWarn, msg)
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
	case "ctrl+c", "e":
		return m, tea.Quit
	case "up", "k":
		m.menu = (m.menu + len(menuItems) - 1) % len(menuItems)
	case "down", "j":
		m.menu = (m.menu + 1) % len(menuItems)
	case "enter":
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

// menuAction 进入当前选中条目对应的屏幕。
// 分发依据是条目自身的 screen 字段（而非下标），故调整菜单排版或条目顺序
// 不会造成「显示与实际功能对应不上」。
func (m *appModel) menuAction() tea.Cmd {
	item := menuItems[m.menu]
	if item.screen == screenMenu {
		return tea.Quit // 末项「退出程序」以 screenMenu 作哨兵
	}
	m.screen = item.screen

	switch item.screen {
	case screenStats:
		m.stats.snap = GetSnapshot()
		return tickStats()
	case screenRaw:
		m.rawView.refresh(m.width, m.height)
	case screenLunarJS:
		m.lunarView.refresh(m.width, m.height)
	case screenLTP9:
		m.ltp9View.refresh(m.width, m.height)
	case screenGoWarn:
		m.warnView.refresh(m.width, m.height)
	case screenDraw:
		m.drawView.refresh(m.width, m.height)
	case screenFrontend:
		m.frontend.msg = ""
	case screenProxy:
		return m.proxyEnter()
	case screenArchive:
		return m.archiveEnter()
	case screenUpdate:
		return m.updateEnter()
	case screenPorts:
		m.ports.activated = true
		m.applyPortsFocus(0)
		return textinput.Blink
	}
	return nil
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
	case screenDraw:
		return m.viewDraw()
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
	var rows []string
	for i, item := range menuItems {
		// 快捷键槽位固定 4 列宽（"[x] "），无快捷键的条目留空以保持对齐
		slot := "    "
		if item.key != "" {
			slot = "[" + item.key + "] "
		}
		cursor := "  "
		title := labelStyle.Render(slot + item.title)
		if i == m.menu {
			cursor = selectStyle.Render("▸ ")
			title = selectStyle.Render(slot + item.title)
		}
		rows = append(rows, cursor+title+"  "+dimStyle.Render(item.desc))
	}

	menuPanel := panelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	title := titleStyle.Render("✦ 星月智能 · 控制台 ✦")
	footer := footerStyle.Render("[↑/↓] 选择   [Enter] 进入   [0-9/a/b/z] 快捷键   [e] 退出")

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
