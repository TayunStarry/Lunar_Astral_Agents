package main

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"LunarSubsystem/GeneralConfig"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 代理屏步骤编号
const (
	proxyStepPorts    = iota // 后端/代理端口配置
	proxyStepIP              // IP 选择
	proxyStepStarting        // 启动中（证书 + 监听）
	proxyStepRunning         // 服务运行中
)

// ipOption IP 选择项（列表项或手动输入项）
type ipOption struct {
	ip     ipCandidate // 列表项时有效
	manual bool        // 是否为手动输入项
}

// proxyState 代理服务屏状态
type proxyState struct {
	step         int
	backendInput textinput.Model // 后端 HTTP 端口
	proxyInput   textinput.Model // 代理 HTTPS 端口
	focus        int             // 端口输入焦点 0/1

	ipOptions []ipOption      // IP 候选（含手动输入项）
	ipIndex   int             // 当前选中候选
	manualIP  textinput.Model // 手动输入框
	inManual  bool            // 是否在手动输入
	chosenIP  string          // 已选定 IP

	server *http.Server // 运行中的代理服务器
	info   proxyInfo    // 访问信息
	qr     string       // 二维码字符画
}

// newProxyState 构建代理屏初始状态
func newProxyState() proxyState {
	backend := newPortInput(strconv.Itoa(*GeneralConfig.BasicPort))
	proxy := newPortInput(strconv.Itoa(*GeneralConfig.ProxyPort))
	manual := textinput.New()
	manual.Placeholder = "例如 192.168.1.100"
	manual.Width = 20
	return proxyState{
		backendInput: backend,
		proxyInput:   proxy,
		manualIP:     manual,
	}
}

// proxyEnter 进入代理屏（重置为新配置会话）
func (m *appModel) proxyEnter() tea.Cmd {
	// 已在运行则保持运行视图，不重置
	if m.proxy.step == proxyStepRunning || m.proxy.step == proxyStepStarting {
		return nil
	}
	m.proxy.step = proxyStepPorts
	m.applyProxyFocus(fieldPrimary)
	return textinput.Blink
}

// buildIPOptions 构建 IP 候选列表（自动推荐私网地址，附手动输入项）
func buildIPOptions() []ipOption {
	candidates := listLocalIPs()
	opts := make([]ipOption, 0, len(candidates)+1)

	// 按得分排序：私网优先
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if ipScore(candidates[j].IP) < ipScore(candidates[i].IP) {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	for _, c := range candidates {
		opts = append(opts, ipOption{ip: c})
	}
	opts = append(opts, ipOption{manual: true})
	return opts
}

// updateProxy 代理屏事件
func (m appModel) updateProxy(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.proxy.step == proxyStepRunning {
			// 运行中：Esc 停止服务返回菜单
			if msg.String() == "esc" {
				return m, m.stopProxy()
			}
			if msg.String() == "ctrl+c" {
				m.stopProxyServer()
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		}

		switch m.proxy.step {
		case proxyStepPorts:
			return m.updateProxyPorts(msg)
		case proxyStepIP:
			return m.updateProxyIP(msg)
		}

	case proxyReadyMsg:
		m.proxy.step = proxyStepRunning
		m.proxy.server = msg.server
		m.proxy.info = msg.info
		m.proxy.qr = msg.qr

	case proxyFailMsg:
		m.proxy.step = proxyStepIP
		uiLogf("  [ERROR] 代理服务启动失败: %v", msg.err)
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

// updateProxyPorts 端口配置步骤
func (m appModel) updateProxyPorts(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "up", "k":
		return m.moveProxyFocus(-1)
	case "down", "j":
		return m.moveProxyFocus(1)
	case "enter":
		return m, m.confirmProxyPorts()
	}

	// 未聚焦的输入框由 textinput 自身忽略按键，故仅向当前选中字段转发
	var cmd tea.Cmd
	if m.proxy.focus == fieldPrimary {
		m.proxy.backendInput, cmd = m.proxy.backendInput.Update(msg)
	} else {
		m.proxy.proxyInput, cmd = m.proxy.proxyInput.Update(msg)
	}
	return m, cmd
}

// moveProxyFocus 在代理端口输入框之间环形移动焦点
// 注意：Focus/Blur 为指针接收器，必须在本方法内直接调用（m.proxy 字段可寻址），
// 不能委托给只返回 tea.Cmd 的辅助方法，否则焦点修改会留在副本上被丢弃
func (m appModel) moveProxyFocus(delta int) (tea.Model, tea.Cmd) {
	idx := (m.proxy.focus + delta + fieldCount) % fieldCount
	m.applyProxyFocus(idx)
	return m, textinput.Blink
}

// applyProxyFocus 将焦点落到指定端口字段并同步 focus 索引
func (m *appModel) applyProxyFocus(idx int) {
	m.proxy.focus = idx
	if idx == fieldPrimary {
		m.proxy.backendInput.Focus()
		m.proxy.proxyInput.Blur()
	} else {
		m.proxy.backendInput.Blur()
		m.proxy.proxyInput.Focus()
	}
}

// confirmProxyPorts 校验端口并进入 IP 选择
// 注意：必须使用指针接收器，否则 step 推进会发生在副本上导致无法进入下一步
func (m *appModel) confirmProxyPorts() tea.Cmd {
	if _, ok := parsePortInput(m.proxy.backendInput, *GeneralConfig.BasicPort); !ok {
		uiLogf("  [ERROR] 后端端口输入无效")
		return nil
	}
	if _, ok := parsePortInput(m.proxy.proxyInput, *GeneralConfig.ProxyPort); !ok {
		uiLogf("  [ERROR] 代理端口输入无效")
		return nil
	}

	m.proxy.step = proxyStepIP
	m.proxy.ipOptions = buildIPOptions()
	m.proxy.ipIndex = 0
	m.proxy.inManual = false
	m.proxy.manualIP.Blur()
	return nil
}

// updateProxyIP IP 选择步骤
func (m appModel) updateProxyIP(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	// 手动输入模式：输入框消费按键
	if m.proxy.inManual {
		switch key.String() {
		case "enter":
			ip := strings.TrimSpace(m.proxy.manualIP.Value())
			if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
				m.proxy.chosenIP = parsed.To4().String()
				return m, m.launchProxy()
			}
			uiLogf("  [ERROR] 输入不是有效的 IPv4 地址")
			return m, nil
		case "esc":
			m.proxy.inManual = false
			m.proxy.manualIP.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.proxy.manualIP, cmd = m.proxy.manualIP.Update(msg)
		return m, cmd
	}

	switch key.String() {
	case "up", "k":
		m.proxy.ipIndex = (m.proxy.ipIndex + len(m.proxy.ipOptions) - 1) % len(m.proxy.ipOptions)
	case "down", "j":
		m.proxy.ipIndex = (m.proxy.ipIndex + 1) % len(m.proxy.ipOptions)
	case "enter":
		opt := m.proxy.ipOptions[m.proxy.ipIndex]
		if opt.manual {
			m.proxy.inManual = true
			m.proxy.manualIP.Focus()
			return m, textinput.Blink
		}
		m.proxy.chosenIP = opt.ip.IP
		return m, m.launchProxy()
	}
	return m, nil
}

// launchProxy 启动代理服务（goroutine：证书 + 监听）
// 注意：必须使用指针接收器，否则 step 切换与 chosenIP 会发生在副本上
func (m *appModel) launchProxy() tea.Cmd {
	backendPort, _ := parsePortInput(m.proxy.backendInput, *GeneralConfig.BasicPort)
	proxyPort, _ := parsePortInput(m.proxy.proxyInput, *GeneralConfig.ProxyPort)
	localIP := m.proxy.chosenIP

	m.proxy.step = proxyStepStarting
	uiLogf("使用 IP 地址: %s", localIP)

	run := func() tea.Msg {
		server, info, err := startProxyServer(backendPort, proxyPort, localIP)
		if err != nil {
			return proxyFailMsg{err: err}
		}

		// 先同步创建监听器：端口被占用等错误立即以 proxyFailMsg 呈现，而不是异步丢失
		ln, lnErr := net.Listen("tcp", fmt.Sprintf(":%d", proxyPort))
		if lnErr != nil {
			return proxyFailMsg{err: fmt.Errorf("监听端口 %d 失败: %w", proxyPort, lnErr)}
		}

		go func() {
			uiLogf("  HTTPS 代理服务器已启动")
			if err := server.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
				uiLogf("  [ERROR] 代理服务器异常: %v", err)
			}
		}()

		qr := ""
		if info.LANURL != "" {
			qr = generateQRText(info.LANURL)
		}
		return proxyReadyMsg{server: server, info: info, qr: qr}
	}
	return tea.Batch(run, m.spinner.Tick)
}

// stopProxy 停止代理服务并返回菜单
func (m *appModel) stopProxy() tea.Cmd {
	m.stopProxyServer()
	m.screen = screenMenu
	return nil
}

// stopProxyServer 关闭运行中的代理服务器
func (m *appModel) stopProxyServer() {
	if m.proxy.server != nil {
		_ = m.proxy.server.Close()
		m.proxy.server = nil
		uiLogf("代理服务已停止")
	}
	m.proxy = newProxyState()
}

// viewProxy 代理屏
func (m appModel) viewProxy() string {
	var title, body, footer string

	switch m.proxy.step {
	case proxyStepPorts:
		title = tuiTitleStyle.Render("[3] HTTPS 代理服务 · 端口配置")
		inputs := renderFocusField("后端 HTTP 服务端口: ", m.proxy.backendInput, m.proxy.focus == fieldPrimary) + "\n" +
			renderFocusField("代理 HTTPS 监听端口: ", m.proxy.proxyInput, m.proxy.focus == fieldSecondary)
		body = tuiPanelStyle.Render(inputs)
		footer = tuiFooterStyle.Render("[↑/↓] 切换输入框   [Enter] 下一步   [Esc] 返回菜单")

	case proxyStepIP:
		title = tuiTitleStyle.Render("[3] HTTPS 代理服务 · 选择本地 IP")

		var rows []string
		for i, opt := range m.proxy.ipOptions {
			cursor := "  "
			var line string
			if opt.manual {
				line = tuiLabelStyle.Render("手动输入 IP 地址")
			} else {
				mark := ""
				if ipScore(opt.ip.IP) == 0 {
					mark = tuiOkStyle.Render("  ← 推荐")
				}
				line = tuiValueStyle.Render(opt.ip.IP) + tuiDimStyle.Render("  ("+opt.ip.Interface+" · "+ipCategory(opt.ip.IP)+")") + mark
			}
			if i == m.proxy.ipIndex {
				cursor = tuiSelectStyle.Render("▸ ")
			}
			rows = append(rows, cursor+line)
		}

		content := tuiPanelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
		if m.proxy.inManual {
			content += "\n\n" + tuiLabelStyle.Render("IP 地址: ") + m.proxy.manualIP.View()
		}
		body = content
		footer = tuiFooterStyle.Render("[↑/↓] 选择   [Enter] 确认   [Esc] 返回菜单")

	case proxyStepStarting:
		title = tuiTitleStyle.Render("[3] HTTPS 代理服务")
		body = tuiPanelStyle.Render(m.taskStatus(true, "正在生成证书并启动代理服务器...", nil))
		footer = tuiFooterStyle.Render("[Esc] 返回菜单")

	default: // proxyStepRunning
		title = tuiTitleStyle.Render("✦ HTTPS 代理服务运行中 ✦")

		infoRows := []string{
			tuiLabelStyle.Render("后端转发目标 ") + tuiValueStyle.Render(m.proxy.info.Target),
			tuiLabelStyle.Render("本地访问     ") + tuiValueStyle.Render(m.proxy.info.LocalURL),
		}
		if m.proxy.info.LANURL != "" {
			infoRows = append(infoRows, tuiLabelStyle.Render("局域网访问   ")+tuiValueStyle.Render(m.proxy.info.LANURL))
		}
		infoRows = append(infoRows, tuiDimStyle.Render(m.proxy.info.CertInfo))
		infoPanel := tuiPanelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, infoRows...))

		content := infoPanel
		if m.proxy.qr != "" && m.width >= 80 {
			qrRows := []string{tuiLabelStyle.Render("手机扫码访问")}
			for _, l := range strings.Split(m.proxy.qr, "\n") {
				qrRows = append(qrRows, tuiQRStyle.Render(l))
			}
			qrPanel := tuiPanelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, qrRows...))
			content = lipgloss.JoinHorizontal(lipgloss.Top, infoPanel, "  ", qrPanel)
		}
		body = content
		footer = tuiFooterStyle.Render("[Esc] 停止服务并返回菜单   详细日志见「日志查询」")
	}

	return title + "\n" + body + "\n" + footer
}
