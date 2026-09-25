package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// logsState 日志查询屏状态
type logsState struct {
	offset int  // 窗口顶部对应的日志索引（仅在非跟随模式下生效）
	follow bool // 是否跟随最新日志（默认开启，滚动到底部自动恢复）
}

// newLogsState 构建日志查询屏初始状态（默认跟随最新）
func newLogsState() logsState {
	return logsState{offset: 0, follow: true}
}

// updateLogs 日志查询屏事件
func (m appModel) updateLogs(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		// 新日志到达时，跟随模式由 View 自动贴底，无需在此处理
		return m, nil
	}

	page := m.logViewHeight() - 1
	if page < 1 {
		page = 1
	}

	switch key.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		m.screen = screenMenu
		return m, nil
	case "c":
		return m, func() tea.Msg { return clearLogsMsg{} }

	case "up", "k":
		m.logView.scrollBy(-1, len(m.logs), m.logViewHeight())
	case "down", "j":
		m.logView.scrollBy(1, len(m.logs), m.logViewHeight())
	case "pgup", "ctrl+u":
		m.logView.scrollBy(-page, len(m.logs), m.logViewHeight())
	case "pgdown", "ctrl+d", " ":
		m.logView.scrollBy(page, len(m.logs), m.logViewHeight())
	case "home", "g":
		m.logView.offset = 0
		m.logView.follow = false
	case "end", "G":
		m.logView.follow = true
	}
	return m, nil
}

// scrollBy 按增量滚动窗口，并维护跟随模式（滚到最底部即恢复跟随）
func (s *logsState) scrollBy(delta, total, visible int) {
	maxOffset := total - visible
	if maxOffset < 0 {
		maxOffset = 0
	}

	// 跟随模式下从当前贴底位置起算
	cur := s.offset
	if s.follow {
		cur = maxOffset
	}
	cur += delta

	if cur <= 0 {
		s.offset = 0
		s.follow = false
		return
	}
	if cur >= maxOffset {
		s.offset = maxOffset
		s.follow = true
		return
	}
	s.offset = cur
	s.follow = false
}

// logViewHeight 日志查询屏可显示的行数（全屏高度扣除标题与底部提示）
func (m appModel) logViewHeight() int {
	reserved := lipgloss.Height(tuiTitleStyle.Render("x")) + lipgloss.Height(tuiFooterStyle.Render("x")) + 3
	height := m.height - reserved
	if height < 3 {
		height = 3
	}
	return height
}

// viewLogs 日志查询屏：全屏展示日志（不限制条数），支持滚动浏览
func (m appModel) viewLogs() string {
	title := tuiTitleStyle.Render("[5] 日志查询")

	total := len(m.logs)
	visible := m.logViewHeight()

	// 计算窗口起点：跟随模式贴底显示最新日志
	start := m.logView.offset
	if m.logView.follow {
		start = total - visible
	}
	if start < 0 {
		start = 0
	}
	if start > 0 && total > visible {
		if start > total-visible {
			start = total - visible
		}
	}
	end := start + visible
	if end > total {
		end = total
	}

	var body string
	if total == 0 {
		body = tuiDimStyle.Render("暂无日志")
	} else {
		lines := make([]string, 0, end-start)
		for _, l := range m.logs[start:end] {
			lines = append(lines, tuiLogLine(l))
		}
		body = strings.Join(lines, "\n")
	}

	// 头部：条目统计与位置指示
	followTag := tuiDimStyle.Render("跟随最新")
	if !m.logView.follow {
		followTag = tuiErrStyle.Render("已暂停滚动")
	}
	header := tuiLabelStyle.Render(fmt.Sprintf("运行日志  %d-%d / 共 %d 条", start+1, end, total)) +
		"  " + followTag + tuiDimStyle.Render("   [c] 清空")

	footer := tuiFooterStyle.Render("[↑/↓] 单行   [PgUp/PgDn] 翻页   [Home/g] 顶部   [End/G] 底部   [Esc] 返回菜单")

	return title + "\n" + header + "\n" + tuiPanelStyle.Render(body) + "\n" + footer
}
