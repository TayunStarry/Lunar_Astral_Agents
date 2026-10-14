package TuiSystem

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"LunarSubsystem/TuiSystem/archive"
	"LunarSubsystem/TuiSystem/updatepack"
)

// archiveState 打包归档屏状态
type archiveState struct {
	configs    []string        // 候选配置文件列表（默认配置在首）
	cfgIdx     int             // 当前选中配置
	loaded     *archive.Config // 已加载配置（进入确认态）
	loadedPath string
	running    bool
	progress   int
	err        error

	inputMode bool            // 自定义配置路径输入态
	input     textinput.Model // 自定义配置路径输入框
}

// newArchiveState 构建打包屏状态
func newArchiveState() archiveState {
	s := archiveState{}
	s.input = textinput.New()
	s.input.Placeholder = archive.DefaultConfigPath
	s.input.Width = 56
	s.input.CharLimit = 200
	return s
}

// archiveEnter 进入打包屏：每次进入重新扫描候选配置
// （配置文件可能在程序运行期间才被放入，重新进入即应看到）
func (m *appModel) archiveEnter() tea.Cmd {
	m.archive.configs = archive.ListConfigs()
	if m.archive.cfgIdx >= len(m.archive.configs) {
		m.archive.cfgIdx = 0
	}
	m.archive.inputMode = false
	m.archive.loaded = nil
	m.archive.err = nil
	return nil
}

// archiveDoneMsg 打包任务完成
type archiveDoneMsg struct{ err error }

// updateArchive 打包屏事件
func (m appModel) updateArchive(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case archiveDoneMsg:
		m.archive.running = false
		m.archive.progress = 100
		m.archive.err = msg.err
		return m, nil
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.archive.running {
				return m, nil
			}
			if m.archive.inputMode {
				// 输入态 Esc → 回列表
				m.archive.leaveArchiveInput()
				return m, nil
			}
			// 确认态 Esc → 回列表
			m.archive.loaded = nil
			m.screen = screenMenu
			return m, nil
		}

		if m.archive.running {
			return m, nil
		}

		// 自定义配置路径输入态
		if m.archive.inputMode {
			if key == "enter" {
				return m.loadArchiveConfig(strings.TrimSpace(m.archive.input.Value()))
			}
			var cmd tea.Cmd
			m.archive.input, cmd = m.archive.input.Update(msg)
			return m, cmd
		}

		if m.archive.loaded != nil {
			// 确认态
			switch key {
			case "enter":
				m.archive.running = true
				m.archive.progress = 0
				m.archive.err = nil
				return m, runArchiveCmd(m.archive.loadedPath, m.archive.loaded)
			case "left":
				m.archive.loaded = nil // 返回列表
			}
			return m, nil
		}

		// 列表态
		switch key {
		case "up", "left":
			if len(m.archive.configs) > 0 {
				m.archive.cfgIdx = (m.archive.cfgIdx + len(m.archive.configs) - 1) % len(m.archive.configs)
			}
		case "down", "right", "tab":
			if len(m.archive.configs) > 0 {
				m.archive.cfgIdx = (m.archive.cfgIdx + 1) % len(m.archive.configs)
			}
		case "p":
			// 进入自定义配置文件路径输入态
			m.archive.inputMode = true
			m.archive.input.SetValue("")
			m.archive.input.Focus()
			m.archive.err = nil
			return m, textinput.Blink
		case "enter":
			if len(m.archive.configs) == 0 {
				return m, nil
			}
			return m.loadArchiveConfig(m.archive.configs[m.archive.cfgIdx])
		}
	}
	return m, nil
}

// loadArchiveConfig 加载指定配置文件（路径为空视为取消，回到列表态）
func (m *appModel) loadArchiveConfig(path string) (tea.Model, tea.Cmd) {
	if path == "" {
		m.archive.leaveArchiveInput()
		return m, nil
	}
	cfg, err := archive.LoadConfig(path)
	if err != nil {
		m.archive.err = err
		return m, nil
	}
	m.archive.leaveArchiveInput()
	m.archive.loaded = cfg
	m.archive.loadedPath = path
	m.archive.err = nil
	return m, nil
}

// leaveArchiveInput 退出自定义路径输入态
func (s *archiveState) leaveArchiveInput() {
	s.inputMode = false
	s.input.Blur()
}

// runArchiveCmd 后台执行打包
func runArchiveCmd(path string, cfg *archive.Config) tea.Cmd {
	return func() tea.Msg {
		return archiveDoneMsg{err: archive.Execute(path, cfg)}
	}
}

// viewArchive 打包屏渲染
func (m appModel) viewArchive() string {
	var body string

	switch {
	case m.archive.running:
		if m.archive.progress > 0 {
			body = m.spinner.View() + " " + valueStyle.Render("正在打包…") + "\n\n" +
				progressBar(m.archive.progress, 40)
		} else {
			body = m.spinner.View() + " " + valueStyle.Render("正在准备打包（扫描文件清单）…")
		}
	case m.archive.inputMode:
		body = labelStyle.Render("输入打包配置文件路径（留空回车返回列表）：") + "\n\n" +
			"  " + m.archive.input.View() + "\n\n" +
			dimStyle.Render("[Enter] 输入项加载配置   [Esc] 返回列表")
	case m.archive.loaded != nil:
		cfg := m.archive.loaded
		var rows []string
		rows = append(rows, labelStyle.Render("已加载配置: ")+valueStyle.Render(m.archive.loadedPath))
		rows = append(rows, "")
		rows = append(rows, m.statRow("输出路径", cfg.Output))
		rows = append(rows, m.statRow("分卷大小", fmt.Sprintf("%d MB", cfg.Size)))
		rows = append(rows, m.statRow("压缩等级", fmt.Sprintf("%d", cfg.Level)))
		rows = append(rows, m.statRow("打包项数", fmt.Sprintf("%d", len(cfg.Import))))
		if len(cfg.Exclude) > 0 {
			rows = append(rows, m.statRow("排除项", strings.Join(cfg.Exclude, " ")))
		}
		rows = append(rows, "")
		rows = append(rows, dimStyle.Render("[Enter] 开始打包   [←] 返回列表"))
		body = lipgloss.JoinVertical(lipgloss.Left, rows...)
	default:
		if len(m.archive.configs) == 0 {
			body = dimStyle.Render("未找到打包配置（默认 "+archive.DefaultConfigPath+"，或 data/*.json）") +
				"\n\n" + dimStyle.Render("将配置文件放入上述任一位置后重新进入本屏，或用 [P] 输入其他路径")
		} else {
			var rows []string
			for i, path := range m.archive.configs {
				name := path
				if path == archive.DefaultConfigPath {
					name += "（默认）"
				}
				if i == m.archive.cfgIdx {
					rows = append(rows, selectStyle.Render("▸ "+name))
				} else {
					rows = append(rows, dimStyle.Render("  "+name))
				}
			}
			rows = append(rows, "", dimStyle.Render("[↑/↓] 选择   [Enter] 加载配置   [P] 指定其他配置文件"))
			body = lipgloss.JoinVertical(lipgloss.Left, rows...)
		}
	}

	if m.archive.err != nil && !m.archive.running {
		body += "\n\n" + errStyle.Render("✗ "+m.archive.err.Error())
	}

	panel := panelStyle.Render(body)
	header := titleStyle.Render("「 打包归档 」")
	footer := footerStyle.Render("[Esc] 返回菜单   详细过程见「原始日志」")
	return header + "\n" + panel + "\n" + footer
}

// ==== 更新包屏 ====

// updateMode 更新包屏的操作模式
type updateMode int

const (
	updateMenuMode   updateMode = iota // 操作菜单
	updateInputTag                     // 输入 tag
	updateInputHash                    // 输入 commit 哈希
	updateInputToken                   // 输入 Gitee 令牌（发布更新包）
	updateRunning                      // 任务进行中
	updateVersions                     // 版本列表展示
)

// updateState 更新包屏状态
type updateState struct {
	mode      updateMode
	input     textinput.Model
	running   bool
	runningOp string
	progress  int
	err       error
	versions  []updatepack.Version
	activated bool
}

// newUpdateState 构建更新包屏状态
func newUpdateState() updateState {
	s := updateState{}
	s.input = textinput.New()
	s.input.Width = 44
	s.input.CharLimit = 64
	return s
}

// updateEnter 进入更新包屏
func (m *appModel) updateEnter() tea.Cmd {
	m.update.mode = updateMenuMode
	m.update.err = nil
	m.update.input.Blur() // 离开输入态：未聚焦即丢弃按键，避免残焦
	return nil
}

// updateDoneMsg 更新包任务完成
type updateDoneMsg struct{ err error }

// updateVersionsMsg 版本列表获取完成
type updateVersionsMsg struct {
	versions []updatepack.Version
	err      error
}

// updateUpdate 更新包屏事件
func (m appModel) updateUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case updateDoneMsg:
		m.update.running = false
		m.update.progress = 100
		m.update.err = msg.err
		if msg.err == nil {
			m.update.mode = updateMenuMode
		}
		return m, nil
	case updateVersionsMsg:
		m.update.running = false
		m.update.err = msg.err
		if msg.err == nil {
			m.update.mode = updateVersions
			m.update.versions = msg.versions
		}
		return m, nil
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.update.running {
				return m, nil
			}
			if m.update.mode != updateMenuMode {
				m.update.mode = updateMenuMode
				m.update.input.Blur() // 退出输入态：避免焦点残留
				return m, nil
			}
			m.screen = screenMenu
			return m, nil
		}

		if m.update.running {
			return m, nil
		}

		switch m.update.mode {
		case updateInputTag, updateInputHash, updateInputToken:
			switch key {
			case "enter":
				value := strings.TrimSpace(m.update.input.Value())
				if value == "" {
					m.update.mode = updateMenuMode
					m.update.input.Blur()
					return m, nil
				}
				m.update.running = true
				m.update.progress = 0
				m.update.err = nil
				m.update.input.Blur() // 提交后离开输入态
				switch m.update.mode {
				case updateInputTag:
					m.update.runningOp = "拉取更新包（tag: " + value + "）"
					return m, runUpdateCmd("tag", value)
				case updateInputToken:
					// 令牌由发布时手动输入：配置文件不再保存密钥
					m.update.runningOp = "发布更新包"
					return m, runUpdateCmd("publish", value)
				}
				m.update.runningOp = "拉取更新包（commit: " + value + "）"
				return m, runUpdateCmd("hash", value)
			}
			var cmd tea.Cmd
			m.update.input, cmd = m.update.input.Update(msg)
			return m, cmd
		case updateMenuMode:
			switch key {
			case "1":
				// 发布需 Gitee 私人令牌：先进入令牌输入态（不回显明文）
				m.update.mode = updateInputToken
				m.update.input.EchoMode = textinput.EchoPassword
				m.update.input.SetValue("")
				m.update.input.Placeholder = "输入 Gitee 私人令牌（需 Release 读写权限）"
				m.update.input.Focus() // 必须 Focus：未聚焦时 textinput 直接丢弃按键
				m.update.err = nil
				return m, textinput.Blink
			case "up", "down", "left", "right":
				// 方向键容错：留在菜单
			case "2":
				m.update.running = true
				m.update.progress = 0
				m.update.err = nil
				m.update.runningOp = "拉取最新更新包"
				return m, runUpdateCmd("latest", "")
			case "3":
				m.update.mode = updateInputTag
				m.update.input.EchoMode = textinput.EchoNormal
				m.update.input.SetValue("")
				m.update.input.Placeholder = "例如 v2026.10.14 或 v1.2.0"
				m.update.input.Focus()
				return m, textinput.Blink
			case "4":
				m.update.mode = updateInputHash
				m.update.input.EchoMode = textinput.EchoNormal
				m.update.input.SetValue("")
				m.update.input.Placeholder = "输入 commit 哈希（支持短哈希前缀）"
				m.update.input.Focus()
				return m, textinput.Blink
			case "5":
				m.update.running = true
				m.update.runningOp = "获取版本列表"
				return m, listVersionsCmd()
			}
		case updateVersions:
			switch key {
			case "left", "backspace":
				m.update.mode = updateMenuMode
			}
		}
	}
	return m, nil
}

// runUpdateCmd 后台执行更新包操作
func runUpdateCmd(kind, value string) tea.Cmd {
	return func() tea.Msg {
		cfg := GetUpdatePackConfig()
		if cfg == nil {
			return updateDoneMsg{err: fmt.Errorf("未配置 update_pack（lunar_config.json 缺少配置节）")}
		}
		var err error
		switch kind {
		case "publish":
			// value 为发布时手动输入的 Gitee 令牌（配置文件不保存密钥）：
			// 传入副本以免污染全局配置
			cfgCopy := *cfg
			cfgCopy.GiteeToken = value
			err = updatepack.Publish(cfgCopy, "")
		case "latest":
			err = updatepack.Pull(*cfg, updatepack.PullSpec{Kind: updatepack.PullLatest})
		case "tag":
			err = updatepack.Pull(*cfg, updatepack.PullSpec{Kind: updatepack.PullTag, Value: value})
		case "hash":
			err = updatepack.Pull(*cfg, updatepack.PullSpec{Kind: updatepack.PullHash, Value: value})
		}
		return updateDoneMsg{err: err}
	}
}

// listVersionsCmd 后台获取版本列表
func listVersionsCmd() tea.Cmd {
	return func() tea.Msg {
		cfg := GetUpdatePackConfig()
		if cfg == nil {
			return updateVersionsMsg{err: fmt.Errorf("未配置 update_pack（lunar_config.json 缺少配置节）")}
		}
		versions, err := updatepack.ListVersions(*cfg)
		return updateVersionsMsg{versions: versions, err: err}
	}
}

// viewUpdate 更新包屏渲染
func (m appModel) viewUpdate() string {
	cfg := GetUpdatePackConfig()

	// 顶部仓库信息
	repoInfo := dimStyle.Render("Gitee 仓库: ")
	if cfg != nil && cfg.GiteeOwner != "" {
		repoInfo += valueStyle.Render(cfg.GiteeOwner + "/" + cfg.GiteeRepo)
		repoInfo += dimStyle.Render("  解压目标: " + cfg.TargetDir)
	} else {
		repoInfo += errStyle.Render("未配置（需在 lunar_config.json 的 update_pack 节填写 gitee_owner/gitee_repo）")
	}

	var body string
	switch {
	case m.update.running:
		if m.update.progress > 0 {
			body = m.spinner.View() + " " + valueStyle.Render(m.update.runningOp) + "\n\n" +
				progressBar(m.update.progress, 40)
		} else {
			body = m.spinner.View() + " " + valueStyle.Render(m.update.runningOp+"（准备中）…")
		}
	case m.update.mode == updateInputTag || m.update.mode == updateInputHash:
		kindName := "版本 tag"
		if m.update.mode == updateInputHash {
			kindName = "commit 哈希"
		}
		body = labelStyle.Render("输入要拉取的"+kindName+"（留空回车返回）：") + "\n\n" + m.update.input.View()
	case m.update.mode == updateInputToken:
		body = labelStyle.Render("输入 Gitee 私人令牌（发布需 Release 读写权限，留空回车取消）：") + "\n\n" +
			"  " + m.update.input.View() + "\n\n" +
			dimStyle.Render("令牌仅用于本次发布，不写入配置文件；拉取公开仓库无需令牌")
	case m.update.mode == updateVersions:
		if len(m.update.versions) == 0 {
			body = dimStyle.Render("仓库暂无发布版本")
		} else {
			var rows []string
			for _, v := range m.update.versions {
				rows = append(rows,
					selectStyle.Render("◆ "+v.TagName)+"  "+labelStyle.Render(v.Name),
					dimStyle.Render("  "+firstLine(v.Body)),
					"")
			}
			rows = append(rows, dimStyle.Render("[←] 返回操作菜单"))
			body = strings.Join(rows, "\n")
		}
	default:
		body = lipgloss.JoinVertical(lipgloss.Left,
			selectStyle.Render("[1] 发布更新包")+"  "+dimStyle.Render("收集本地资源 → 打包 → 上传 Gitee Release（需输入令牌）"),
			selectStyle.Render("[2] 拉取最新")+"  "+dimStyle.Render("取最新 Release 附件解压到本地"),
			selectStyle.Render("[3] 按 tag 拉取")+"  "+dimStyle.Render("输入版本标签拉取指定发布"),
			selectStyle.Render("[4] 按哈希拉取")+"  "+dimStyle.Render("输入 commit 哈希（zipball 兜底通道）"),
			selectStyle.Render("[5] 版本列表")+"  "+dimStyle.Render("查看仓库已发布的更新包"),
		)
	}

	if m.update.err != nil && !m.update.running {
		body += "\n\n" + errStyle.Render("✗ "+m.update.err.Error())
	} else if m.update.progress >= 100 && !m.update.running {
		body += "\n\n" + okStyle.Render("✓ 完成")
	}

	panel := panelStyle.Render(repoInfo + "\n\n" + body)
	header := titleStyle.Render("「 更新包 」")
	footer := footerStyle.Render("[Esc] 返回菜单   详细过程见「原始日志」")
	return header + "\n" + panel + "\n" + footer
}

// firstLine 取文本首行
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
