package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"environment_repair/component"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 归档屏状态编号
const (
	archiveStepInput   = iota // 输入配置路径
	archiveStepSummary        // 配置摘要确认
	archiveStepRunning        // 打包中
	archiveStepDone           // 完成
)

// archiveState 打包归档屏状态
type archiveState struct {
	step       int
	pathInput  textinput.Model          // 配置文件路径输入框
	config     *component.ArchiveConfig // 已加载配置
	chosenPath string                   // 已确认的配置路径
	percent    int                      // 进度（-1 准备阶段）
	startAt    time.Time                // 开始时间（运行中用于实时耗时）
	elapsed    time.Duration            // 完成时刻冻结的总耗时（避免随重绘增长）
	err        error                    // 执行错误
}

// newArchiveState 构建归档屏初始状态
func newArchiveState() archiveState {
	input := textinput.New()
	input.Placeholder = "local_data/archive_config.json"
	input.Width = 44
	return archiveState{pathInput: input, percent: -1}
}

// archiveEnter 进入归档屏
func (m *appModel) archiveEnter() tea.Cmd {
	if m.archive.step == archiveStepRunning {
		return nil // 打包进行中不可重置
	}
	if m.archive.step == archiveStepInput {
		m.archive.pathInput.Focus()
		return textinput.Blink
	}
	return nil
}

// updateArchive 归档屏事件
func (m appModel) updateArchive(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}

		// 打包进行中禁止退出/重置（7z 子进程无法安全中断）
		if m.archive.step == archiveStepRunning {
			if msg.String() == "esc" {
				uiLogf("  [WARN] 打包进行中，请等待压缩完成后操作")
			}
			return m, nil
		}

		switch msg.String() {
		case "esc":
			if m.archive.step == archiveStepSummary {
				m.archive.step = archiveStepInput
				m.archive.pathInput.Focus()
				return m, textinput.Blink
			}
			m.screen = screenMenu
			return m, nil
		}

		switch m.archive.step {
		case archiveStepInput:
			if msg.String() == "enter" {
				return m, m.loadArchiveConfig()
			}
			var cmd tea.Cmd
			m.archive.pathInput, cmd = m.archive.pathInput.Update(msg)
			return m, cmd

		case archiveStepSummary:
			switch msg.String() {
			case "y", "enter":
				return m, m.launchArchive()
			case "n":
				m.archive.step = archiveStepInput
				m.archive.pathInput.Focus()
				return m, textinput.Blink
			}
		}

	case progressMsg:
		m.archive.percent = int(msg)

	case archiveDoneMsg:
		m.archive.step = archiveStepDone
		m.archive.err = msg.err
		// 在完成时刻冻结总耗时：完成态视图若实时计算 time.Since(startAt)，
		// 会随 spinner tick / 日志到达等重绘持续增长
		m.archive.elapsed = time.Since(m.archive.startAt).Round(time.Millisecond)
		if msg.err != nil {
			uiLogf("✗ 打包失败: %v", msg.err)
		} else {
			uiLogf("✓ 打包流程完成，总耗时 %s", m.archive.elapsed)
		}
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

// loadArchiveConfig 加载配置文件并进入摘要确认
// 注意：必须使用指针接收器，否则 step/config 推进会发生在副本上导致无法进入下一步
func (m *appModel) loadArchiveConfig() tea.Cmd {
	path := strings.TrimSpace(m.archive.pathInput.Value())
	if path == "" {
		path = "local_data/archive_config.json"
	}
	// 去除可能的引号（拖拽文件时可能带引号）
	path = strings.Trim(path, "\"'")

	config, err := LoadArchiveConfig(path)
	if err != nil {
		uiLogf("  [ERROR] 加载配置失败: %v", err)
		return nil
	}

	if _, statErr := os.Stat(path); statErr != nil {
		uiLogf("  [WARN] 配置文件不存在: %s（已按配置内容尝试加载，请核对路径）", path)
	}

	m.archive.config = config
	m.archive.chosenPath = path
	m.archive.step = archiveStepSummary
	uiLogf("配置文件已加载: %s", path)
	return nil
}

// launchArchive 确认执行打包任务（指针接收器，理由同上）
func (m *appModel) launchArchive() tea.Cmd {
	m.archive.step = archiveStepRunning
	m.archive.percent = -1
	m.archive.err = nil
	m.archive.elapsed = 0
	m.archive.startAt = time.Now()

	path := m.archive.chosenPath
	config := m.archive.config

	run := func() tea.Msg {
		err := RunArchive(path, config)
		return archiveDoneMsg{err: err}
	}
	return tea.Batch(run, m.spinner.Tick)
}

// viewArchive 归档屏
func (m appModel) viewArchive() string {
	title := tuiTitleStyle.Render("[4] 分卷打包归档")
	var body, footer string

	switch m.archive.step {
	case archiveStepInput:
		// 列出候选配置文件
		candidates := ListArchiveConfigs()
		var hint string
		if len(candidates) > 0 {
			hint = tuiDimStyle.Render("发现配置文件: " + strings.Join(candidates, "  ·  "))
		}

		input := tuiLabelStyle.Render("配置文件路径: ") + m.archive.pathInput.View()
		body = tuiPanelStyle.Render(input + "\n\n" + hint)
		footer = tuiFooterStyle.Render("[Enter] 加载配置   [Esc] 返回菜单   详细日志见「日志查询」")

	case archiveStepSummary:
		lines := configSummaryLines(m.archive.config, m.archive.chosenPath)
		summary := tuiPanelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
		confirm := "\n" + tuiLabelStyle.Render("确认开始打包？") +
			tuiOkStyle.Render("  [Y/Enter] 开始") + tuiDimStyle.Render(" / ") +
			tuiErrStyle.Render("[N] 取消") + tuiDimStyle.Render(" / [Esc] 修改路径")
		body = summary + confirm
		footer = tuiFooterStyle.Render("[Y] 开始打包   [N/Esc] 返回")

	case archiveStepRunning:
		status := m.taskStatus(true, "正在打包（7z 压缩中），请稍候...", nil)
		var progress string
		if m.archive.percent < 0 {
			progress = tuiDimStyle.Render(fmt.Sprintf("正在准备压缩... 已耗时 %.0fs", time.Since(m.archive.startAt).Seconds()))
		} else {
			progress = progressBar(m.archive.percent, m.width-24) +
				tuiDimStyle.Render(fmt.Sprintf("  已耗时 %.0fs", time.Since(m.archive.startAt).Seconds()))
		}
		body = tuiPanelStyle.Render(status + "\n\n" + progress + "\n" + tuiErrStyle.Render("打包进行中，暂不可退出本屏"))
		footer = tuiFooterStyle.Render("打包中...")

	default: // archiveStepDone
		if m.archive.err != nil {
			body = tuiPanelStyle.Render(tuiErrStyle.Render("✗ 打包失败: "+m.archive.err.Error()) +
				"\n\n" + tuiDimStyle.Render("按 Esc 返回菜单，可修改配置后重试"))
		} else {
			// 使用完成时刻冻结的耗时，避免重绘导致数字持续增长
			body = tuiPanelStyle.Render(tuiOkStyle.Render(fmt.Sprintf("✓ 打包完成，总耗时 %s", m.archive.elapsed)) +
				"\n" + tuiDimStyle.Render("输出路径: "+m.archive.config.Output+".7z"))
		}
		footer = tuiFooterStyle.Render("[Esc] 返回菜单")
	}

	return title + "\n" + body + "\n" + footer
}
