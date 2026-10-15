package TuiSystem

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"LunarSubsystem/TuiSystem/proxy"
)

// proxyState HTTPS 代理屏状态（配置 → 运行 两阶段）
type proxyState struct {
	activated bool // 首次进入时已初始化
	inputs    [2]textinput.Model
	focus     int
	ips       []string
	ipIdx     int

	running  bool
	stopping bool // 已发出停止指令、等待服务收尾（期间禁止再次启动）
	handle   *proxy.Handle
	info     proxy.Info
	errText  string
	progress int
	qrText   string
}

// newProxyState 构建代理屏状态
func newProxyState() proxyState {
	s := proxyState{}
	s.inputs[0] = textinput.New()
	s.inputs[0].Placeholder = "36789"
	s.inputs[0].Width = 8
	s.inputs[1] = textinput.New()
	s.inputs[1].Placeholder = "36790"
	s.inputs[1].Width = 8
	return s
}

// proxyEnter 进入代理屏：枚举本机 IP 并聚焦第一个输入框
func (m *appModel) proxyEnter() tea.Cmd {
	if !m.proxy.activated {
		m.proxy.activated = true
		m.proxy.ips = proxy.LocalIPs()
		if len(m.proxy.ips) > 0 {
			m.proxy.ipIdx = 0
		}
		m.applyProxyFocus(0)
	}
	return textinput.Blink
}

// applyProxyFocus 设置代理屏焦点
func (m *appModel) applyProxyFocus(idx int) {
	m.proxy.focus = idx
	for i := range m.proxy.inputs {
		if i == idx {
			m.proxy.inputs[i].Focus()
		} else {
			m.proxy.inputs[i].Blur()
		}
	}
}

// proxyContext 标题栏上下文：绑定地址或运行状态
func proxyContext(s proxyState) string {
	switch {
	case s.running:
		return "运行中 · " + s.info.IP + ":" + strconv.Itoa(s.info.ProxyPort)
	case s.stopping:
		return "正在停止"
	}
	if len(s.ips) > 0 {
		return "待配置 · " + s.ips[clampInt(s.ipIdx, 0, len(s.ips)-1)]
	}
	return "待配置 · 未检测到可用 IP"
}

// proxyKeys 代理屏键位
func proxyKeys(s proxyState) []keyHint {
	switch {
	case s.running:
		return []keyHint{kh("Esc", "停止服务并返回菜单")}
	case s.stopping:
		return []keyHint{kh("Esc", "返回菜单（停止完成后可重启）")}
	}
	return []keyHint{
		kh("↑/↓ Tab", "切换输入框 / 绑定 IP"),
		kh("Enter", "启动服务"),
		kh("Esc", "返回菜单"),
	}
}

// updateProxy 代理屏事件
func (m appModel) updateProxy(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case proxyReadyMsg:
		m.proxy.running = true
		m.proxy.handle = msg.token
		m.proxy.info = msg.info
		m.proxy.qrText = generateQRText(msg.info.QRText)
		m.proxy.progress = 0
		return m, nil
	case proxyFailMsg:
		m.proxy.errText = msg.err.Error()
		return m, nil
	case proxyStoppedMsg:
		m.proxy.stopping = false
		return m, nil
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.proxy.running {
				// 运行中 Esc → 后台停止服务并返回菜单。
				// Stop 会等待连接收尾（最长 3 秒），不能在事件循环内同步执行，
				// 否则界面假死、按键无响应——这正是「停止后回不了菜单」的成因。
				handle := m.proxy.handle
				m.proxy.running = false
				m.proxy.stopping = true
				m.proxy.handle = nil
				m.proxy.qrText = ""
				m.proxy.progress = 0
				m.screen = screenMenu
				return m, stopProxyCmd(handle)
			}
			m.screen = screenMenu
			return m, nil
		}

		if m.proxy.running {
			return m, nil
		}

		switch key {
		case "up", "left":
			if m.proxy.focus == 0 {
				// 已在最上：切换 IP
				if len(m.proxy.ips) > 0 {
					m.proxy.ipIdx = (m.proxy.ipIdx + len(m.proxy.ips) - 1) % len(m.proxy.ips)
				}
			} else {
				m.applyProxyFocus(m.proxy.focus - 1)
				return m, textinput.Blink
			}
		case "down", "right", "tab":
			if m.proxy.focus == 1 {
				if len(m.proxy.ips) > 0 {
					m.proxy.ipIdx = (m.proxy.ipIdx + 1) % len(m.proxy.ips)
				}
			} else {
				m.applyProxyFocus(m.proxy.focus + 1)
				return m, textinput.Blink
			}
		case "enter":
			if m.proxy.stopping {
				// 上一次服务尚未完全停止，避免端口冲突，等待停止完成后重试
				return m, nil
			}
			m.proxy.errText = ""
			m.setBusy("启动 HTTPS 代理")
			return m, startProxyCmd(m.proxy)
		}

		var cmd tea.Cmd
		m.proxy.inputs[m.proxy.focus], cmd = m.proxy.inputs[m.proxy.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

// startProxyCmd 后台启动代理服务
func startProxyCmd(s proxyState) tea.Cmd {
	return func() tea.Msg {
		backend, perr := strconv.Atoi(s.inputs[0].Value())
		if perr != nil || backend < 1 || backend > 65535 {
			return proxyFailMsg{err: errInvalidPort}
		}
		front, perr := strconv.Atoi(s.inputs[1].Value())
		if perr != nil || front < 1 || front > 65535 || front == backend {
			return proxyFailMsg{err: errInvalidPort}
		}
		ip := "127.0.0.1"
		if len(s.ips) > 0 {
			ip = s.ips[s.ipIdx]
		}
		handle, info, err := proxy.Start(backend, front, ip)
		if err != nil {
			return proxyFailMsg{err: err}
		}
		return proxyReadyMsg{info: info, token: handle}
	}
}

// stopProxyCmd 后台停止代理服务（异步执行，避免阻塞 TUI 事件循环）
func stopProxyCmd(handle *proxy.Handle) tea.Cmd {
	return func() tea.Msg {
		if handle != nil {
			_ = handle.Stop()
		}
		return proxyStoppedMsg{}
	}
}

// errInvalidPort 端口输入无效
var errInvalidPort = errPortInvalid{}

type errPortInvalid struct{}

func (errPortInvalid) Error() string {
	return "端口无效（1-65535，且前后端口不能相同）"
}

// viewProxy 代理屏渲染
func (m appModel) viewProxy() string {
	f := m.frameFor("HTTPS 代理", proxyContext(m.proxy), proxyKeys(m.proxy))
	w, h := f.bodySize()

	var body []string
	switch {
	case m.proxy.running:
		body = m.proxyRunningBody(w, h)
	case m.proxy.stopping:
		body = []string{
			"",
			"  " + selectStyle.Render(m.spinner.View()) + " " + valueStyle.Render("正在停止代理服务…"),
			"",
			"  " + dimStyle.Render("停止完成后可重新启动；已返回主菜单，可继续其它操作"),
		}
	default:
		body = m.proxyConfigBody(w)
	}
	if m.proxy.errText != "" && !m.proxy.running {
		body = withErrStrip(body, errString(m.proxy.errText), w)
	}
	return f.render(fitLines(body, h))
}

// errString 把已有文本包装成 error（复用错误提示条）
type errString string

func (e errString) Error() string { return string(e) }

// proxyConfigBody 配置态：表单 + IP 选择
func (m appModel) proxyConfigBody(w int) []string {
	keyW := 10
	boxW := minInt(w, 64)

	ipText := dimStyle.Render("（未检测到可用 IP）")
	if len(m.proxy.ips) > 0 {
		ipIdx := clampInt(m.proxy.ipIdx, 0, len(m.proxy.ips)-1)
		ipText = valueStyle.Render(m.proxy.ips[ipIdx]) +
			dimStyle.Render("   ←/→ 在最上/最下时切换（共 "+strconv.Itoa(len(m.proxy.ips))+" 个）")
	}

	form := []string{
		fieldRow("后端端口", m.proxy.inputs[0], m.proxy.focus == 0, keyW),
		fieldRow("代理端口", m.proxy.inputs[1], m.proxy.focus == 1, keyW),
		"  " + dimStyle.Render(fitWidth("绑定 IP", keyW)) + ipText,
	}

	body := []string{
		dimStyle.Render("  配置 WSS→WS 反向代理：手机等外部设备经 HTTPS/WSS 接入本地服务"),
		"",
	}
	body = append(body, formBox(form, boxW)...)
	body = append(body,
		"",
		"  "+dimStyle.Render("后端端口为本地服务端口，代理端口为对外端口；两者不能相同"),
		"  "+dimStyle.Render("按 Enter 启动；启动后可扫码接入"),
	)
	return body
}

// proxyRunningBody 运行态：接入信息 + 二维码
func (m appModel) proxyRunningBody(w, h int) []string {
	info := m.proxy.info
	keyW := 10
	access := "https://" + info.IP + ":" + strconv.Itoa(info.ProxyPort)

	left := []string{
		sectionRule("接入信息", minInt(w, 56)),
		"",
		kvRow("绑定 IP", info.IP, keyW),
		kvRow("后端端口", strconv.Itoa(info.BackendPort), keyW),
		kvRow("代理端口", strconv.Itoa(info.ProxyPort), keyW),
		kvRow("接入地址", access, keyW),
		"",
		okStyle.Render("  ✓ 代理服务运行中"),
		dimStyle.Render("  Esc 停止服务并返回菜单"),
	}

	body := append([]string{}, left...)
	if m.proxy.qrText != "" {
		qr := strings.Split(strings.TrimRight(m.proxy.qrText, "\n"), "\n")
		body = append(body, "", sectionRule("扫码接入", minInt(w, 56)), "")
		body = append(body, centerBlock(qr, w)...)
	}
	if m.proxy.progress > 0 {
		body = append(body, "", "  "+progressBar(m.proxy.progress, minInt(w-8, 40)))
	}
	return body
}
