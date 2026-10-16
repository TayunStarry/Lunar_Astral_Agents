package TuiSystem

import (
	"fmt"
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

// proxyPortDefaults 代理屏的端口提示与校验基准（宿主注入真实配置）。
//
// 教训（实测故障）：这里原先把**代理端口**的提示写成写死的 "36790"——那正是
// ModelPort（BasicPort+1）即**模型服务端口**。用户照着提示填 36790，代理就把模型服务的
// 端口接管了：程序自身发往模型服务的请求打到 TLS 代理上，于是出现
// `client sent an HTTP request to an HTTPS server`、连接被重置、嵌入请求 unexpected EOF、
// 记忆库查询失败。提示值必须来自真实配置，且必须拦住与自身服务撞车的端口。
type proxyPortDefaults struct {
	BackendHint string // 后端端口（Web 服务）
	FrontHint   string // 代理端口
	Reserved    map[int]string
}

var proxyDefaults = proxyPortDefaults{
	BackendHint: "36789",
	FrontHint:   "36794",
	Reserved:    map[int]string{},
}

// SetProxyDefaults 注入代理屏端口默认值（预填充 + 留空即采用）与保留端口（宿主启动时调用）
func SetProxyDefaults(backendPort, proxyPort int, reserved map[int]string) {
	if backendPort > 0 {
		proxyDefaults.BackendHint = strconv.Itoa(backendPort)
	}
	if proxyPort > 0 {
		// 默认值本身不得落在自身服务端口上，否则"直接回车"也会撞车
		if owner := reserved[proxyPort]; owner != "" {
			proxyDefaults.FrontHint = strconv.Itoa(firstFreePort(proxyPort, reserved))
		} else {
			proxyDefaults.FrontHint = strconv.Itoa(proxyPort)
		}
	}
	if reserved != nil {
		proxyDefaults.Reserved = reserved
		proxy.SetReservedPorts(reserved)
	}
}

// firstFreePort 从 from 起找一个未被保留端口占用的端口（默认值兜底）
func firstFreePort(from int, reserved map[int]string) int {
	for p := from + 1; p < from+100 && p <= 65535; p++ {
		if _, taken := reserved[p]; !taken {
			return p
		}
	}
	return from + 1
}

// defaultBackendPort / defaultFrontPort 取当前默认端口（数值，解析失败时回退内置值）
func defaultBackendPort() int { return atoiOr(proxyDefaults.BackendHint, 36789) }
func defaultFrontPort() int   { return atoiOr(proxyDefaults.FrontHint, 36794) }

func atoiOr(s string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && v > 0 && v <= 65535 {
		return v
	}
	return def
}

// resolveProxyPort 解析端口输入：**留空即采用默认值**，否则按输入解析。
// 抽成纯函数便于单测——这条"直接回车即可用"的路径必须稳定。
func resolveProxyPort(input, def string) (int, error) {
	v := strings.TrimSpace(input)
	if v == "" {
		v = strings.TrimSpace(def)
	}
	if v == "" {
		return 0, errInvalidPort
	}
	p, err := strconv.Atoi(v)
	if err != nil || p < 1 || p > 65535 {
		return 0, errInvalidPort
	}
	return p, nil
}

// newProxyState 构建代理屏状态：端口**预填充**为默认值，用户可直接回车启动，
// 也可以清空（留空同样采用默认值）或改成别的端口。
func newProxyState() proxyState {
	s := proxyState{}
	s.inputs[0] = textinput.New()
	s.inputs[0].Placeholder = proxyDefaults.BackendHint
	s.inputs[0].SetValue(proxyDefaults.BackendHint)
	s.inputs[0].Width = 8
	s.inputs[1] = textinput.New()
	s.inputs[1].Placeholder = proxyDefaults.FrontHint
	s.inputs[1].SetValue(proxyDefaults.FrontHint)
	s.inputs[1].Width = 8
	return s
}

// validateProxyPorts 代理端口校验：范围、前后不同、且不得占用自身服务的端口。
// 抽成纯函数便于单测——这类误配的代价是整机功能异常，必须被测试钉住。
func validateProxyPorts(backend, front int) error {
	if backend < 1 || backend > 65535 || front < 1 || front > 65535 {
		return errInvalidPort
	}
	if front == backend {
		return errInvalidPort
	}
	if owner := proxy.ReservedPortOwner(front); owner != "" {
		return fmt.Errorf("代理端口 %d 是本程序「%s」的端口，会与它抢端口；请改用其它端口（如 %s）",
			front, owner, proxyDefaults.FrontHint)
	}
	return nil
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
		backend, berr := resolveProxyPort(s.inputs[0].Value(), proxyDefaults.BackendHint)
		if berr != nil {
			return proxyFailMsg{err: berr}
		}
		front, ferr := resolveProxyPort(s.inputs[1].Value(), proxyDefaults.FrontHint)
		if ferr != nil {
			return proxyFailMsg{err: ferr}
		}
		if verr := validateProxyPorts(backend, front); verr != nil {
			return proxyFailMsg{err: verr}
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
		"  "+dimStyle.Render(fmt.Sprintf("已预填默认端口（%d / %d）；留空即用默认值，直接回车即可启动",
			defaultBackendPort(), defaultFrontPort())),
		"  "+dimStyle.Render("后端端口为本地服务端口，代理端口为对外端口；两者不能相同"),
		"  "+dimStyle.Render("按 Enter 启动；启动后可扫码接入"),
	)
	return body
}

// proxyRunningBody 运行态：接入信息 + 二维码
func (m appModel) proxyRunningBody(w, _ int) []string {
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
