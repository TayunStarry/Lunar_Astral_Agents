package TuiSystem

import (
	"strconv"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.proxy.running {
				// 运行中 Esc → 停止服务回到配置态
				if m.proxy.handle != nil {
					_ = m.proxy.handle.Stop()
				}
				m.proxy.running = false
				m.proxy.handle = nil
				m.proxy.qrText = ""
				return m, nil
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
			m.proxy.errText = ""
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

// errInvalidPort 端口输入无效
var errInvalidPort = errPortInvalid{}

type errPortInvalid struct{}

func (errPortInvalid) Error() string { return "端口无效（1-65535，且前后端口不能相同）" }

// viewProxy 代理屏渲染
func (m appModel) viewProxy() string {
	var body string

	if m.proxy.running {
		info := m.proxy.info
		body = lipgloss.JoinVertical(lipgloss.Left,
			m.statRow("后端端口", strconv.Itoa(info.BackendPort)),
			m.statRow("代理端口", strconv.Itoa(info.ProxyPort)),
			m.statRow("绑定 IP", info.IP),
			m.statRow("接入地址", "https://"+info.IP+":"+strconv.Itoa(info.ProxyPort)),
		)
		if m.proxy.qrText != "" {
			body += "\n\n" + m.proxy.qrText
		}
		body += "\n\n" + okStyle.Render("✓ 代理服务运行中") + "  " + dimStyle.Render("[Esc] 停止并返回配置")
		if m.proxy.progress > 0 {
			body += "\n" + progressBar(m.proxy.progress, 30)
		}
	} else {
		ipText := dimStyle.Render("（未检测到可用 IP）")
		if len(m.proxy.ips) > 0 {
			ipText = valueStyle.Render(m.proxy.ips[m.proxy.ipIdx]) + "  " + dimStyle.Render("[↑/↓ 在最上/最下时切换]")
		}
		body = labelStyle.Render("配置 WSS→WS 反向代理（手机等外部设备经 HTTPS/WSS 接入本地服务）：") + "\n\n" +
			renderFocusField("后端端口 ", m.proxy.inputs[0], m.proxy.focus == 0) + "\n" +
			renderFocusField("代理端口 ", m.proxy.inputs[1], m.proxy.focus == 1) + "\n" +
			"  " + labelStyle.Render("绑定 IP   ") + ipText
		if m.proxy.errText != "" {
			body += "\n\n" + errStyle.Render("✗ "+m.proxy.errText)
		} else {
			body += "\n\n" + dimStyle.Render("[Enter] 启动服务   [↑/↓] 切换输入框/切换 IP")
		}
	}

	panel := panelStyle.Render(body)
	header := titleStyle.Render("「 HTTPS 代理 」")
	footer := footerStyle.Render("[Esc] " + map[bool]string{true: "停止服务", false: "返回菜单"}[m.proxy.running])
	return header + "\n" + panel + "\n" + footer
}
