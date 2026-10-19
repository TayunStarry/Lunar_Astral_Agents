package console

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/skip2/go-qrcode"

	"CorePersonality/engine/console/archive"
	"CorePersonality/engine/console/ltpxmarket"
	"CorePersonality/engine/console/portrelease"
	"CorePersonality/engine/console/proxy"
	"CorePersonality/engine/console/updatepack"
	"LunarSubsystem/LoggerGeneral"
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
	screenMarket
	screenLTP9Auth
	screenDump
	screenEngine
)

// menuItem 主菜单条目：快捷键、分组与目标屏幕均显式声明。
// 进入哪个屏由 screen 字段决定（而非下标推算），故调整排版或条目顺序
// 都不会造成「显示与实际功能对应不上」。
type menuItem struct {
	key    string   // 快捷键（单字符，空串=仅回车进入）
	title  string   // 条目名称
	desc   string   // 条目说明（宽屏显示于右侧详情栏）
	group  string   // 分组（渲染时在该组首条之前插入分隔标题）
	screen screenID // 目标屏幕；值为 screenMenu 时表示退出程序（哨兵）
}

// menuItems 主菜单条目表：顺序即显示顺序，按「更新发布 / 运行观测 / 系统管理」
// 分组组织，让同类功能聚拢、缩短查找路径。
var menuItems = []menuItem{
	{key: "0", title: "资源更新 管理", group: "更新与发布", screen: screenUpdate,
		desc: "发布更新包到 Gitee / 拉取更新包（最新 / tag / commit 哈希），更新含主程序时自动重启"},
	{key: "m", title: "LTPX 应用市场", group: "更新与发布", screen: screenMarket,
		desc: "浏览 / 标签筛选 / 安装 / 发布 LTPX 工具包（Gitee Release 通道）"},
	{key: "l", title: "LTP9 授权签发", group: "更新与发布", screen: screenLTP9Auth,
		desc: "扫描包目录中的 LTP9 包，按颜色区分「已签发 / 未签发 / 需重签」，回车依据脚本现有引用的权限签发 permissions.key"},

	{key: "1", title: "核心智能 实况", group: "运行观测", screen: screenAgent,
		desc: "月华思考 / 输出 / 工具调用的流式显示（对话者 · 绘制者 · 演奏者 · 记忆者…）"},
	{key: "2", title: "运行状态 显示", group: "运行观测", screen: screenStats,
		desc: "资源占用 / token 消耗总量 / 均次消耗 / 输出速度 / 平均上下文"},
	{key: "3", title: "原始日志 显示", group: "运行观测", screen: screenRaw,
		desc: "终端全量原始行（stdout 拦截 + 结构化日志合流）"},
	{key: "4", title: "月华引擎 日志", group: "运行观测", screen: screenLunarJS,
		desc: "月华智能体 JavaScript 层日志"},
	{key: "5", title: "插件引擎 日志", group: "运行观测", screen: screenLTP9,
		desc: "LTP9 沙箱 JavaScript 层日志"},
	{key: "6", title: "星月系统 警告", group: "运行观测", screen: screenGoWarn,
		desc: "Go 模块 WARN / ERROR / FATAL 日志"},
	{key: "7", title: "绘图引擎 日志", group: "运行观测", screen: screenDraw,
		desc: "sd.cpp 绘图引擎输出（进度 / 采样速度 / 时间统计），顶部附采样进度"},
	{key: "d", title: "数据导出 管理", group: "运行观测", screen: screenDump,
		desc: "把原始日志导出为 .log（JSON Lines）、把月华主上下文或任一子智能体上下文导出为 .json"},

	{key: "8", title: "前端页面 管理", group: "系统管理", screen: screenFrontend,
		desc: "月华 / 琉璃桌面窗口状态探测与意外关闭重启"},
	{key: "9", title: "局域共享 管理", group: "系统管理", screen: screenProxy,
		desc: "WSS → WS 反向代理，支持 IP 选择与手机扫码接入"},
	{key: "a", title: "打包归档 管理", group: "系统管理", screen: screenArchive,
		desc: "项目文件 7z 分卷打包，支持进度显示"},
	{key: "b", title: "端口释放 管理", group: "系统管理", screen: screenPorts,
		desc: "扫描指定端口范围，终止占用进程并验证释放"},
	{key: "r", title: "月华引擎 管理", group: "系统管理", screen: screenEngine,
		desc: "查看 goja 运行时状态（就绪 / 重载次数 / 连续失败），按 R 手动重载；连续应答失败达阈值时宿主会自动重载"},

	{key: "z", title: "退出程序", group: "会话", screen: screenMenu,
		desc: "关闭控制台并优雅停止全部子系统（感谢使用「星月智能 · 控制台」）"},
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

// ==== 应用主模型 ====

// appModel 全应用模型：屏幕切换 + 各屏状态聚合 + 框体级状态
type appModel struct {
	screen screenID
	menu   int // 主菜单选中项
	menuPl listPane

	width, height int
	help          bool // 键位帮助页（模态）

	// 任务指示：标题栏承载「进行中 / 刚结束」的跨屏反馈，
	// 使用户离开功能屏后仍能看见任务进展与结果
	busy      string
	notice    string
	noticeErr bool
	noticeAt  time.Time

	spinner spinner.Model

	rawView   logsState // [3] 原始日志
	lunarView logsState // [4] 月华 JS 日志
	ltp9View  logsState // [5] ltp9 JS 日志
	drawView  logsState // [7] sd.cpp 绘图引擎日志
	warnView  logsState // [6] Go 警告错误
	agent     agentState
	stats     statsState
	frontend  frontendState
	proxy     proxyState
	archive   archiveState
	update    updateState
	ports     portsState
	market    marketState
	ltp9Auth  ltp9AuthState
	dump      dumpState
	engine    engineState

	streamUnsub func() // 流式事件订阅注销
	sizeLogged  bool   // 首帧尺寸自检是否已落盘
}

// newAppModel 构建初始模型
func newAppModel() appModel {
	drawView := newLogsState(SourceDraw)
	drawView.renderLine = renderDrawLogLine // 绘图引擎日志按专用规则排版
	return appModel{
		screen:    screenMenu,
		spinner:   newSpinner(),
		agent:     agentState{follow: true}, // 实况屏默认跟随最新轮次
		rawView:   newLogsState(SourceRaw),
		lunarView: newLogsState(SourceLunarJS),
		ltp9View:  newLogsState(SourceLTP9JS),
		drawView:  drawView,
		warnView:  newLogsState(SourceGoWarn),
		proxy:     newProxyState(),
		archive:   newArchiveState(),
		update:    newUpdateState(),
		ports:     newPortsState(),
		market:    newMarketState(),
		ltp9Auth:  newLTP9AuthState(),
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

// tickStats 生成状态刷新命令（同时驱动标题栏时钟与提示超时）
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
		// 首帧尺寸自检落盘：远端报「界面一直向上滚 / 显示混乱」时，
		// 这一行直接说明「程序以为终端多大、打算画几行」——是判断尺寸失配还是字形宽度失配的关键。
		if !m.sizeLogged {
			m.sizeLogged = true
			AppendConsoleLog(fmt.Sprintf(
				"[TUI] 首帧尺寸自检: 终端报 %d×%d，框体渲染 %d 行（≤%d 列，末列留空）",
				msg.Width, msg.Height, msg.Height, maxInt(1, msg.Width-1)))
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

	case handoverTickMsg:
		// 交接倒计时在顶层处理：无论当前屏为何都必须走完并退出——
		// tick 链一旦因屏幕切换而断掉，就会卡在「更新器在等待、控制台不退出」的死局
		if pendingHandover == nil {
			return m, nil
		}
		if m.update.handoverLeft <= 1 {
			return m, tea.Quit
		}
		m.update.handoverLeft--
		return m, handoverTick()
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
	case screenMarket:
		return m.updateMarket(msg)
	case screenLTP9Auth:
		return m.updateLTP9Auth(msg)
	case screenDump:
		return m.updateDump(msg)
	case screenEngine:
		return m.updateEngine(msg)
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
	// 屏间顺序切换：任何屏都可用，不依赖「先回主菜单再进」。
	// 文本框聚焦时让位（[ ] 也是合法正文），其余屏一律接管。
	if (k == "[" || k == "]") && !m.inputFocused() {
		delta := 1
		if k == "[" {
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
	case screenMarket:
		switch m.market.mode {
		case marketInputTag, marketInputQuery, marketInputArchive, marketInputDesc, marketInputToken:
			return true
		}
	case screenArchive:
		return m.archive.inputMode
	case screenUpdate:
		switch m.update.mode {
		case updateInputTag, updateInputHash, updateInputToken:
			return true
		}
	}
	return false
}

// applyProgress 把任务进度写回发起任务的屏幕
func (m *appModel) applyProgress(percent int) {
	switch m.screen {
	case screenArchive:
		m.archive.progress = percent
	case screenUpdate:
		m.update.progress = percent
	case screenProxy:
		m.proxy.progress = percent
	case screenMarket:
		m.market.progress = percent
	}
}

// updateMenu 菜单屏事件
func (m appModel) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "ctrl+c", "e", "q":
		return m, tea.Quit
	case "up", "k":
		m.menu = listMove(m.menu, -1, len(menuItems), true)
	case "down", "j":
		m.menu = listMove(m.menu, 1, len(menuItems), true)
	case "home", "g":
		m.menu = 0
	case "end", "G":
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
	case screenRaw:
		m.rawView.reset()
	case screenLunarJS:
		m.lunarView.reset()
	case screenLTP9:
		m.ltp9View.reset()
	case screenGoWarn:
		m.warnView.reset()
	case screenDraw:
		m.drawView.reset()
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
	case screenMarket:
		return m.marketEnter()
	case screenLTP9Auth:
		return m.ltp9AuthEnter()
	case screenDump:
		return m.dumpEnter()
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

// quitCleanup TUI 退出后的统一清理
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
	ltpxmarket.SetLogHook(nil)
	ltpxmarket.SetProgressHook(nil)
}

// ErrNoTerminal 控制台输出未连接到终端（无法渲染界面）
var ErrNoTerminal = fmt.Errorf("输出未连接到终端：控制台界面无法显示")

// ensureTerminal 校验控制台可用，避免出现「隐形实例」。
//
// 这里只处理**最坏情况**：既没有可用的继承句柄，也取不到 CONOUT$（例如以服务/计划任务
// 方式在无控制台环境启动）。此时宁可明确报错退出，也不要留下一个看不见、按不动的后台
// 进程——它会占着端口挡住下一次启动。诊断直接写 CONOUT$，不依赖已被接管的标准流。
func ensureTerminal() error {
	if f, ok := GetRealStdout().(*os.File); ok && isConsoleFile(f) {
		return nil
	}
	msg := "[星月智能] 控制台界面无法显示：标准输出未连接到终端，且无法从控制台设备取得句柄。\r\n" +
		"  常见原因：以服务/计划任务等无控制台方式启动，或输出被重定向。\r\n" +
		"  本进程即将退出，以免留下看不见、按不动的后台实例占用端口。\r\n" +
		"  请在终端窗口中直接运行主程序；已打开的前端窗口会随本进程关闭。\r\n"
	// 标准流此刻不可信，直接向控制台设备写：有控制台就能看到，没有则静默跳过
	if out := consoleOut(); out != nil {
		_, _ = out.WriteString("\r\n" + msg + "\r\n")
		_ = out.Close()
	}
	LoggerGeneral.Error("CorePersonality", "控制台输出未连接到终端，界面不可用，进程退出")
	return ErrNoTerminal
}

// Run 启动全屏 TUI 应用（阻塞直至用户退出；宿主需已先调用 Install）
func Run() error {
	model := newAppModel()

	// 进事件循环前先确认控制台可用：否则会留下「进程在跑、界面全无」的隐形实例
	if err := ensureTerminal(); err != nil {
		return err
	}

	// 控制台编码与字形宽度：中文 Windows 的经典 conhost 默认用 GBK 代码页，且中文点阵字体
	// 会把 ◆ ■ ▶ · → 这类「东亚歧义宽度」字形渲染成 2 列。两者都会让每一行超出终端宽度，
	// 一折行整屏就翻倍高、备用屏幕放不下便每帧向上滚（用户实测：一打开就滚动 + 显示混乱）。
	// 先切 UTF-8 再实测字形宽度——顺序不能反，否则测到的是乱码的宽度。
	renderTarget := GetRealStdout()
	if f, ok := renderTarget.(*os.File); ok {
		restoreEncoding, fingerprint := ensureConsoleEncoding(f)
		defer restoreEncoding()
		widths := calibrateGlyphWidths(f)
		// 环境指纹先落盘：判断「乱码 / 错位 / 滚动」时，第一手证据就是
		// 「跑在哪个终端、代码页是多少、字形实测多宽」
		if fingerprint != "" {
			AppendConsoleLog("[TUI] " + fingerprint)
		}
		if len(widths) > 0 {
			wide := make([]string, 0, 6)
			summary := make([]string, 0, 8)
			for _, g := range probeGlyphs() {
				w, ok := widths[g]
				if !ok {
					continue
				}
				SetMeasuredGlyphWidth(g, w)
				if w > 1 {
					wide = append(wide, fmt.Sprintf("%c=%d列", g, w))
				}
			}
			summary = append(summary, fmt.Sprintf("[TUI] 终端字形校准: 实测 %d 个字形", len(widths)))
			if len(wide) > 0 {
				summary = append(summary, "  比标准宽度更宽（已按终端宽度排版）: "+strings.Join(wide, " "))
			} else {
				summary = append(summary, "  无更宽字形（终端与本程序宽度表一致）")
			}
			msg := strings.Join(summary, "\n")
			AppendConsoleLog(msg)
			pushScrubbedLog(SourceRaw, "RAW", "", "", msg)
		} else {
			AppendConsoleLog("[TUI] 终端字形校准失败：未能取得控制台信息，已退回内置宽度表（中文控制台字体下可能错位）")
		}
	}

	// 配色档位与样式绑定：必须在 VT 处理开启之后、tea 启动之前。
	// 这一步看着琐碎，却是「界面一片纯白」的根治点——见 renderer.go 的说明。
	// 顺序也要紧：颜色档位取决于「控制台是否已启用 ANSI 处理」，反了就永远退化为无色。
	if diag := activateTheme(renderTarget); diag != "" {
		AppendConsoleLog("[TUI] 配色档位: " + diag)
		pushScrubbedLog(SourceRaw, "RAW", "", "", "[TUI] 配色档位: "+diag)
	}

	// 更新器子进程的进度接回真实终端：主程序接管 stdout 后，子进程若继承 NUL 设备，
	// 用户就完全看不到「更新器已启动 → 正在覆盖 → 已重启」，整条链路成了黑盒。
	updatepack.SetConsoleWriter(func() io.Writer { return GetRealStdout() })

	// 输入同样不信任继承句柄：句柄不可用时改读控制台设备，否则 TUI 收不到按键
	prog := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithOutput(GetRealStdout()),
		tea.WithInput(resolveInputTarget()),
	)

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

	// 终端尺寸轮询：Windows 无 SIGWINCH，bubbletea 只在启动时取一次尺寸；
	// 窗口改小后框架仍按旧尺寸渲染整屏 → 每帧都高出一截 → 控制台不停向上滚。
	if out, ok := GetRealStdout().(*os.File); ok {
		go watchTerminalSize(prog.Send, func() (int, int, bool) { return consoleSize(out) }, done)
	}

	// 功能子包钩子注入：日志并入统一管线，进度推送到 TUI。
	// 同时落盘到 console.log：这些是「发布/拉取/代理/端口释放」等操作的唯一过程记录，
	// 用户报障时（例如 HTTPS 代理端口冲突）没有它就只能靠现场复现。
	taskLog := func(msg string) {
		PushTaskLog(msg)
		AppendConsoleLog(msg)
		prog.Send(taskLogMsg(msg))
	}
	archive.SetLogHook(taskLog)
	archive.SetProgressHook(func(percent int) { prog.Send(progressMsg(percent)) })
	proxy.SetLogHook(taskLog)
	portrelease.SetLogHook(taskLog)
	updatepack.SetLogHook(taskLog)
	updatepack.SetProgressHook(func(percent int) { prog.Send(progressMsg(percent)) })
	ltpxmarket.SetLogHook(taskLog)
	ltpxmarket.SetProgressHook(func(percent int) { prog.Send(progressMsg(percent)) })

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

	// 交接说明必须写在「退出备用屏幕之后」：此时真实终端已恢复，
	// 这段话会留在用户可回看的正常缓冲区里（屏上的倒计时画面随退出被清掉）
	printHandoverNotice()
	return err
}

// printHandoverNotice 输出主程序更新交接说明（非交接场景不输出）。
//
// 为何值得单独打印：含主程序的更新会让控制台自行退出，用户看到的只是「程序没了」；
// 把「更新器已接管 / 去哪看日志 / 会否自动重启」写到终端，故障时可自行判断与取证。
func printHandoverNotice() {
	note := pendingHandover
	if note == nil {
		return
	}
	out := GetRealStdout()
	fmt.Fprintf(out, "\n[星月智能] 主程序更新已交由更新器接续，控制台退出以释放文件占用。\n")
	fmt.Fprintf(out, "  更新包  : %s\n", note.PackFile)
	fmt.Fprintf(out, "  覆盖目标: %s\n", note.Root)
	if len(note.Files) > 0 {
		fmt.Fprintf(out, "  纳入文件: %s\n", strings.Join(note.Files, "、"))
	}
	fmt.Fprintf(out, "  更新器  : %s\n", note.Applier)
	fmt.Fprintf(out, "  过程日志: %s\n", note.LogPath)
	fmt.Fprintf(out, "更新器将在此进程退出后解压覆盖并重启程序，请勿关闭本终端窗口。\n\n")
}

// View 渲染分发
func (m appModel) View() string {
	if m.width == 0 {
		return "正在初始化界面…"
	}
	if m.help {
		return m.viewHelp()
	}

	switch m.screen {
	case screenMenu:
		return m.viewMenu()
	case screenAgent:
		return m.viewAgent()
	case screenStats:
		return m.viewStats()
	case screenRaw:
		return m.viewLogs(&m.rawView, "原始日志", "全量 stdout + 结构化日志")
	case screenLunarJS:
		return m.viewLogs(&m.lunarView, "月华智能体 JS 日志", "LunarCore · JavaScript")
	case screenLTP9:
		return m.viewLogs(&m.ltp9View, "插件引擎 JS 日志", "StarLTP · console")
	case screenDraw:
		return m.viewDraw()
	case screenGoWarn:
		return m.viewLogs(&m.warnView, "Go 警告与错误", "WARN / ERROR / FATAL")
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
	case screenMarket:
		return m.viewMarket()
	case screenLTP9Auth:
		return m.viewLTP9Auth()
	case screenDump:
		return m.viewDump()
	case screenEngine:
		return m.viewEngine()
	}
	return ""
}

// screenMeta 当前屏的展示名、上下文与键位（同一份数据驱动标题栏、底栏与帮助页）
func (m appModel) screenMeta() (name, context string, keys []keyHint) {
	switch m.screen {
	case screenMenu:
		return "主菜单", "", menuKeys()
	case screenAgent:
		return "核心智能 实况", "", agentKeys(m.agent)
	case screenStats:
		return "运行状态", "采样间隔 1s", statsKeys()
	case screenRaw:
		return "原始日志", "全量 stdout + 结构化日志", logsKeys()
	case screenLunarJS:
		return "月华智能体 JS 日志", "LunarCore · JavaScript", logsKeys()
	case screenLTP9:
		return "插件引擎 JS 日志", "StarLTP · console", logsKeys()
	case screenDraw:
		return "绘图引擎日志", "sd.cpp", logsKeys()
	case screenGoWarn:
		return "Go 警告与错误", "WARN / ERROR / FATAL", logsKeys()
	case screenFrontend:
		return "前端管理", "", frontendKeys()
	case screenProxy:
		return "HTTPS 代理", proxyContext(m.proxy), proxyKeys(m.proxy)
	case screenArchive:
		return "打包归档", archiveContext(m.archive), archiveKeys(m.archive)
	case screenUpdate:
		return "更新包", updateContext(m.update), updateKeys(m.update)
	case screenPorts:
		return "端口释放", "", portsKeys()
	case screenMarket:
		return "LTPX 应用市场", marketContext(m.market), marketKeys(m.market)
	case screenLTP9Auth:
		return "LTP9 授权签发", ltp9AuthContext(m.ltp9Auth), ltp9AuthKeys(m.ltp9Auth)
	case screenDump:
		return "数据导出", dumpContextLine(m.dump), dumpKeys(m.dump)
	case screenEngine:
		return "月华引擎 管理", "goja lunar_loop", engineKeys(m.engine)
	}
	return "未知", "", nil
}

// menuKeys 主菜单键位
func menuKeys() []keyHint {
	return []keyHint{
		kh("↑/↓", "选择"),
		kh("Enter/→", "进入"),
		kh("0-9 a b d l m z", "直达"),
		kh("?", "帮助"),
		kh("q", "退出"),
	}
}

// menuDetail 主菜单右侧详情：说明 + 元信息（宽屏两栏布局的右栏）
func (m appModel) menuDetail(idx, w, h int) []string {
	if idx < 0 || idx >= len(menuItems) || w <= 0 {
		return blankLines(h, w)
	}
	it := menuItems[idx]
	lines := []string{
		selectStyle.Render("✦ " + it.title),
		"",
		kvRow("快捷键", fitWidth(marketOrDash(it.key), 4), 8),
		kvRow("分组", it.group, 8),
		kvRow("入口", "Enter 或 ["+it.key+"]", 8),
		"",
		sectionRule("说明", w),
	}
	for _, l := range wrapPlain(it.desc, maxInt(8, w-2)) {
		lines = append(lines, "  "+labelStyle.Render(l))
	}
	return fitLines(lines, h)
}

// viewMenu 菜单屏：左侧分组列表 + 右侧详情（宽屏）。
// 列表经定高窗口渲染，条目再多也不会溢出终端。
func (m appModel) viewMenu() string {
	f := m.frameFor("主菜单", "", menuKeys())
	w, h := f.bodySize()

	// 宽屏两栏：左列表 + 竖分隔 + 右详情；窄屏退化为单栏（说明置于标题之下）
	twoPane := w >= 88
	listW := w
	detailW := 0
	if twoPane {
		listW = clampInt(w*45/100, 36, 50)
		detailW = w - listW - 4 // 竖分隔 3 列 + 右栏尾部留白 1 列
	}
	rowW := maxInt(8, listW-listGutter)

	blocks := make([][]string, len(menuItems))
	for i, it := range menuItems {
		var b []string
		if i == 0 || menuItems[i-1].group != it.group { // 组首插入分组标题
			b = append(b, sectionRule(it.group, rowW))
		}
		sel := i == m.menu
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		keyTag := dimStyle.Render("[" + it.key + "]")
		title := labelStyle.Render(it.title)
		if sel {
			keyTag = keyStyle.Render("[" + it.key + "]")
			title = selectStyle.Render(it.title)
		}
		b = append(b, listRow(rowW, sel, cursor, title, keyTag))
		if !twoPane {
			b = append(b, newRow(rowW, sel).
				L("     ", dimStyle.Render(truncTo(it.desc, maxInt(4, rowW-6)))).
				String())
		}
		blocks[i] = b
	}

	lines, info := m.menuPl.layoutView(blocks, m.menu, listW, h)
	body := lines
	if twoPane {
		detail := m.menuDetail(m.menu, detailW, h)
		body = make([]string, h)
		for i := 0; i < h; i++ {
			body[i] = lines[i] + ruleStyle.Render(" │ ") + detail[i] + " "
		}
	}

	f.status = dimStyle.Render(info.positionText() + " 项")
	return f.render(body)
}

// ==== 共享渲染组件 ====

// fieldRow 表单行：标签定宽 + 输入框（聚焦时高亮并加 ▸ 标记）
func fieldRow(label string, input textinput.Model, focused bool, labelW int) string {
	mark := "  "
	name := dimStyle.Render(fitWidth(label, labelW))
	if focused {
		mark = cursorStyle.Render("▸ ")
		name = selectStyle.Render(fitWidth(label, labelW))
	}
	return mark + name + input.View()
}

// formBox 表单框：给输入区加一层圆角描边，聚焦范围更明确
func formBox(lines []string, width int) []string {
	if width < 12 {
		return lines
	}
	inner := width - 4
	out := make([]string, 0, len(lines)+2)
	out = append(out, composeRule("╭", "╮", "", "", "─", width))
	for _, l := range lines {
		out = append(out, ruleStyle.Render("│ ")+fitWidth(l, inner)+ruleStyle.Render(" │"))
	}
	out = append(out, composeRule("╰", "╯", "", "", "─", width))
	return out
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

// centerBlock 整块内容按宽度居中（按显示宽度补足左侧空格）
func centerBlock(lines []string, width int) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		d := dispWidth(l)
		if d >= width {
			out = append(out, truncTo(l, width))
			continue
		}
		pad := (width - d) / 2
		out = append(out, strings.Repeat(" ", pad)+l)
	}
	return out
}
