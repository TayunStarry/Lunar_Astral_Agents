package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"environment_repair/component"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/skip2/go-qrcode"
)

// ==== 统一日志通道 ====

// uiLogRaw 统一运行日志出口：
// TUI 激活时送入日志通道由日志窗格实时渲染；未激活时直接打印控制台。
func uiLogRaw(msg string) {
	uiLogMu.Lock()
	ch := tuiLogCh
	uiLogMu.Unlock()

	if ch != nil {
		select {
		case ch <- msg:
		default:
		}
		return
	}
	fmt.Println(msg)
}

// uiLogf 格式化运行日志
func uiLogf(format string, v ...any) {
	uiLogRaw(fmt.Sprintf(format, v...))
}

// RunTUIApp 启动全屏 TUI 应用（主入口，阻塞直至用户退出）
// 挂载统一日志通道与 component 包的日志/进度钩子
func RunTUIApp() {
	// 激活日志通道
	logCh := make(chan string, 256)
	uiLogMu.Lock()
	tuiLogCh = logCh
	uiLogMu.Unlock()

	model := newAppModel()
	prog := tea.NewProgram(model, tea.WithAltScreen())

	// 日志转发 goroutine：通道 → TUI
	go func() {
		for msg := range logCh {
			prog.Send(logMsg(msg))
		}
	}()

	// 注入 component 包的日志/进度钩子（打包归档屏消费）
	component.SetUiLogHook(func(msg string) { prog.Send(logMsg(msg)) })
	component.SetProgressHook(func(percent int) { prog.Send(progressMsg(percent)) })

	// Ctrl+C / 终止信号 → 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		prog.Quit()
	}()

	if _, err := prog.Run(); err != nil {
		fmt.Printf("[ERROR] TUI 异常退出: %v\n", err)
	}

	// 注销日志通道与钩子（不 close 通道，避免转发 goroutine 竞态 panic）
	uiLogMu.Lock()
	tuiLogCh = nil
	uiLogMu.Unlock()
	component.SetUiLogHook(nil)
	component.SetProgressHook(nil)
}

// generateQRText 生成终端半块字符二维码（占用行数减半，便于终端展示）
func generateQRText(url string) string {
	qr, err := qrcode.New(url, qrcode.Low)
	if err != nil {
		return ""
	}
	qr.DisableBorder = true
	return qr.ToSmallString(false)
}

// ==== 消息定义 ====

// logMsg 日志消息（由日志转发 goroutine 送入）
type logMsg string

// clearLogsMsg 清空日志指令
type clearLogsMsg struct{}

// taskDoneMsg 修复/端口释放任务完成
type taskDoneMsg struct{ err error }

// progressMsg 打包进度（-1 表示准备阶段未知进度）
type progressMsg int

// archiveDoneMsg 打包任务完成
type archiveDoneMsg struct{ err error }

// proxyReadyMsg 代理服务器启动成功
type proxyReadyMsg struct {
	server *http.Server
	info   proxyInfo
	qr     string
}

// proxyFailMsg 代理服务器启动失败
type proxyFailMsg struct{ err error }

// ==== 屏幕与样式 ====

// screenID 应用屏幕编号
type screenID int

const (
	screenMenu screenID = iota
	screenRepair
	screenPorts
	screenProxy
	screenArchive
	screenLogs
)

var (
	tuiTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")).MarginBottom(1)
	tuiLabelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	tuiValueStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	tuiDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	tuiPanelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 2)
	tuiFooterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243")).MarginTop(1)
	tuiQRStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	tuiSelectStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	tuiOkStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82"))
	tuiErrStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("204"))

	tuiLogInfoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))  // 青色：常规
	tuiLogProxyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))  // 蓝色：转发记录
	tuiLogWarnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // 橙色：警告
	tuiLogErrStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("204")) // 红色：错误
)

// tuiLogLine 单条日志渲染：按前缀着色
func tuiLogLine(line string) string {
	switch {
	case strings.Contains(line, "[ERROR]"):
		return tuiLogErrStyle.Render(line)
	case strings.Contains(line, "[WARN]"):
		return tuiLogWarnStyle.Render(line)
	case strings.Contains(line, "[PROXY]"):
		return tuiLogProxyStyle.Render(line)
	default:
		return tuiLogInfoStyle.Render(line)
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
	return tuiValueStyle.Render(fmt.Sprintf("[%s] %3d%%", bar, percent))
}

// maxTUILogs TUI 日志缓冲最大条数
const maxTUILogs = 500

// ==== 应用主模型 ====

// appModel 全应用模型：屏幕切换 + 共享日志缓冲
type appModel struct {
	screen screenID
	menu   int // 菜单选中项

	logs          []string // 共享日志缓冲
	width, height int

	spinner spinner.Model // 运行中动画

	repair  repairState
	ports   portsState
	proxy   proxyState
	archive archiveState
	logView logsState
}

// newAppModel 构建初始模型
func newAppModel() appModel {
	return appModel{
		screen:  screenMenu,
		spinner: newSpinner(),
		repair:  newRepairState(),
		ports:   newPortsState(),
		proxy:   newProxyState(),
		archive: newArchiveState(),
		logView: newLogsState(),
	}
}

// newSpinner 创建加载动画
func newSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = tuiSelectStyle
	return s
}

// Init 初始化
func (m appModel) Init() tea.Cmd { return nil }

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

	case logMsg:
		line := string(msg)
		// 跳过与上一条完全相同的日志，避免轮询/重试类重复日志刷屏
		if len(m.logs) > 0 && m.logs[len(m.logs)-1] == line {
			break
		}
		m.logs = append(m.logs, line)
		if len(m.logs) > maxTUILogs {
			m.logs = m.logs[len(m.logs)-maxTUILogs:]
		}

	case clearLogsMsg:
		m.logs = nil
		m.logView = newLogsState()
	}

	switch m.screen {
	case screenMenu:
		return m.updateMenu(msg)
	case screenRepair:
		return m.updateRepair(msg)
	case screenPorts:
		return m.updatePorts(msg)
	case screenProxy:
		return m.updateProxy(msg)
	case screenArchive:
		return m.updateArchive(msg)
	case screenLogs:
		return m.updateLogs(msg)
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
	case "q":
		return m, tea.Quit
	case "up", "k":
		m.menu = (m.menu + menuItemCount - 1) % menuItemCount
	case "down", "j":
		m.menu = (m.menu + 1) % menuItemCount
	case "1", "2", "3", "4", "5", "6":
		m.menu = int(key.String()[0] - '1')
		return m, m.menuAction()
	case "enter":
		return m, m.menuAction()
	}
	return m, nil
}

// menuAction 执行当前选中菜单项（最后一项为退出）
// 注意：必须使用指针接收器，否则对 m.screen 的修改会发生在副本上导致切换失效
func (m *appModel) menuAction() tea.Cmd {
	switch m.menu {
	case 0:
		m.screen = screenRepair
		return m.startRepair()
	case 1:
		m.screen = screenPorts
		m.ports.activated = true
		m.applyPortsFocus(fieldPrimary)
		return textinput.Blink
	case 2:
		m.screen = screenProxy
		return m.proxyEnter()
	case 3:
		m.screen = screenArchive
		return m.archiveEnter()
	case 4:
		m.screen = screenLogs
		m.logView = newLogsState()
		return nil
	default:
		return tea.Quit
	}
}

// View 渲染分发
func (m appModel) View() string {
	if m.width == 0 {
		return "正在初始化界面..."
	}

	switch m.screen {
	case screenMenu:
		return m.viewMenu()
	case screenRepair:
		return m.viewRepair()
	case screenPorts:
		return m.viewPorts()
	case screenProxy:
		return m.viewProxy()
	case screenArchive:
		return m.viewArchive()
	case screenLogs:
		return m.viewLogs()
	}
	return ""
}

// viewMenu 菜单屏
func (m appModel) viewMenu() string {
	items := []struct{ title, desc string }{
		{"资源补全修复", "从内嵌资源中释放缺失的 local_data 文件（audios/multimedia/package）"},
		{"端口占用释放", "扫描指定端口范围，终止占用端口的进程并验证释放结果"},
		{"HTTPS 代理服务", "WSS→WS 反向代理，支持 IP 选择与手机扫码接入"},
		{"分卷打包归档", "将项目文件打包为 7z 分卷压缩包，支持进度显示"},
		{"日志查询", "查看运行日志全量记录，支持滚动浏览与清空"},
		{"退出程序", "感谢使用「星月智能 · 环境修复工具」"},
	}

	var rows []string
	for i, item := range items {
		cursor := "  "
		title := tuiLabelStyle.Render(fmt.Sprintf("[%d] %s", i+1, item.title))
		if i == m.menu {
			cursor = tuiSelectStyle.Render("▸ ")
			title = tuiSelectStyle.Render(fmt.Sprintf("[%d] %s", i+1, item.title))
		}
		rows = append(rows, cursor+title+"  "+tuiDimStyle.Render(item.desc))
	}

	menuPanel := tuiPanelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	title := tuiTitleStyle.Render("✦ 星月智能 · 环境修复工具 ✦")
	footer := tuiFooterStyle.Render(fmt.Sprintf("[↑/↓] 选择   [Enter] 进入   [1-%d] 快捷键   [q] 退出", menuItemCount))

	return title + "\n" + menuPanel + "\n" + footer
}

// ---- 共享渲染组件 ----

// taskStatus 渲染任务状态行（spinner + 文案）
func (m appModel) taskStatus(running bool, text string, err error) string {
	if running {
		return m.spinner.View() + " " + tuiValueStyle.Render(text)
	}
	if err != nil {
		return tuiErrStyle.Render("✗ " + text + ": " + err.Error())
	}
	return tuiOkStyle.Render("✓ " + text)
}

// renderFocusField 渲染可聚焦输入项：选中项高亮并带 ▸ 标记，未选中项灰显
func renderFocusField(label string, input textinput.Model, focused bool) string {
	if focused {
		return tuiSelectStyle.Render("▸ ") + tuiSelectStyle.Render(label) + input.View()
	}
	return "  " + tuiDimStyle.Render(label) + input.View()
}
