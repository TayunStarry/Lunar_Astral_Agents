package TuiSystem

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// logsState 日志视图状态（四个日志屏共用同一结构）
type logsState struct {
	vp       viewport.Model
	source   LogSource
	ready    bool
	lastSeq  int  // 上次渲染条数（增量刷新用）
	atBottom bool // 跟随底部开关（用户上滚时暂停跟随）
}

// newLogsState 构建指定来源的日志视图
func newLogsState(source LogSource) logsState {
	return logsState{source: source}
}

// refresh 重建视口内容（尺寸变化或首次进入时）
func (s *logsState) refresh(width, height int) {
	logs := GetLogs(s.source)
	lines := make([]string, 0, len(logs))
	for _, line := range logs {
		lines = append(lines, renderLogLine(line))
	}
	if len(lines) == 0 {
		lines = append(lines, dimStyle.Render("（暂无日志）"))
	}

	vpHeight := height - 4
	if vpHeight < 3 {
		vpHeight = 3
	}
	if !s.ready {
		s.vp = viewport.New(width-2, vpHeight)
		s.ready = true
	} else {
		s.vp.Width = width - 2
		s.vp.Height = vpHeight
	}
	s.vp.SetContent(strings.Join(lines, "\n"))

	// 用户滚动到底部附近时自动跟随；否则保持位置（暂停跟随便于回看）
	s.atBottom = true
	s.vp.GotoBottom()
}

// updateLogs 日志屏事件处理
func (m appModel) updateLogs(s *logsState, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		case "c":
			ClearLogs(s.source)
			s.refresh(m.width, m.height)
			return m, nil
		case "home":
			s.vp.GotoTop()
			s.atBottom = false
			return m, nil
		case "end":
			s.vp.GotoBottom()
			s.atBottom = true
			return m, nil
		}
	}

	// 其余按键交给视口（up/down/pgup/pgdn）
	prevY := s.vp.YOffset
	var cmd tea.Cmd
	s.vp, cmd = s.vp.Update(msg)
	if s.vp.YOffset != prevY {
		s.atBottom = s.vp.AtBottom()
	}
	// 新日志到达或视口尺寸未初始化时重建
	if !s.ready {
		s.refresh(m.width, m.height)
	}
	return m, cmd
}

// viewLogs 日志屏渲染
func (m appModel) viewLogs(s *logsState, title string) string {
	if !s.ready {
		s.refresh(m.width, m.height)
	} else {
		// 每帧同步一次新日志（渲染期间增量追加）
		s.syncContent()
	}
	header := titleStyle.Render("「 " + title + " 」")
	footer := footerStyle.Render("[↑/↓/PgUp/PgDn] 滚动   [Home/End] 顶部/底部   [c] 清空   [Esc] 返回菜单")
	return header + "\n" + s.vp.View() + "\n" + footer
}

// syncContent 将缓冲中的新日志追加进视口（全量重建代价可控：≤1000 行）
func (s *logsState) syncContent() {
	logs := GetLogs(s.source)
	if len(logs) == s.lastSeq {
		return
	}
	s.lastSeq = len(logs)
	lines := make([]string, 0, len(logs))
	for _, line := range logs {
		lines = append(lines, renderLogLine(line))
	}
	if len(lines) == 0 {
		lines = append(lines, dimStyle.Render("（暂无日志）"))
	}
	follow := s.vp.AtBottom()
	s.vp.SetContent(strings.Join(lines, "\n"))
	if follow {
		s.vp.GotoBottom()
	}
}

// ==== 智能体实况屏 ====

// agentState 智能体实况屏状态（渲染数据实时取自 GetTurns）
type agentState struct {
	lastCount int
}

// updateAgent 智能体实况屏事件
func (m appModel) updateAgent(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		}
	}
	return m, nil
}

// viewAgent 智能体实况屏渲染：轮次分段（思考暗色 / 输出常色 / 工具调用高亮）
func (m appModel) viewAgent() string {
	turns := GetTurns()

	vpHeight := m.height - 5
	if vpHeight < 5 {
		vpHeight = 5
	}
	width := m.width - 4
	if width < 20 {
		width = 20
	}

	var blocks []string
	for _, turn := range turns {
		blocks = append(blocks, m.renderTurn(turn, width)...)
	}
	if len(blocks) == 0 {
		blocks = append(blocks, dimStyle.Render("（等待月华智能体的模型调用…进行对话即可在此看到思考与输出的实时流）"))
	}

	vp := viewport.New(width+2, vpHeight)
	vp.SetContent(strings.Join(blocks, "\n"))
	vp.GotoBottom()

	header := titleStyle.Render("「 智能体实况 」")
	footer := footerStyle.Render("[Esc] 返回菜单   思考流以暗色显示，输出为常色，工具调用高亮")
	return header + "\n" + vp.View() + "\n" + footer
}

// renderTurn 渲染单轮：头部 + 思考（尾部截断）+ 工具调用 + 输出（尾部截断）
func (m appModel) renderTurn(turn *AgentTurn, width int) []string {
	var lines []string

	status := "…进行中"
	if !turn.Active {
		status = "✓完成 " + turn.FinishedAt.Format("15:04:05")
	}
	lines = append(lines, roleStyle.Render("◆ "+turn.Role)+" "+dimStyle.Render(
		turn.StartedAt.Format("15:04:05")+" · "+status))

	// 思考区：显示尾部若干行，避免长思考刷屏（暗色渲染）
	if turn.Thinking != "" {
		for _, l := range wrapTail(turn.Thinking, width, 8, "💭 ") {
			lines = append(lines, thinkStyle.Render(l))
		}
	}
	// 工具调用
	for _, tool := range turn.Tools {
		lines = append(lines, valueStyle.Render("  ⚙ "+tool))
	}
	// 输出区
	if turn.Content != "" {
		lines = append(lines, wrapTail(turn.Content, width, 12, "")...)
	}
	lines = append(lines, "")
	return lines
}

// wrapTail 长文本按宽度折行并仅保留尾部 maxLines 行
func wrapTail(text string, width, maxLines int, prefix string) []string {
	// 归一化换行并按显示宽度粗略折行（CJK 按字符计）
	text = strings.ReplaceAll(text, "\r\n", "\n")
	parts := strings.Split(text, "\n")
	var lines []string
	for _, p := range parts {
		runes := []rune(p)
		for len(runes) > width-2 {
			lines = append(lines, string(runes[:width-2]))
			runes = runes[width-2:]
		}
		lines = append(lines, string(runes))
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
		lines[0] = "…" + lines[0]
	}
	if prefix != "" {
		lines[0] = prefix + lines[0]
	}
	return lines
}

// ==== 运行状态屏 ====

// statsState 运行状态屏状态
type statsState struct {
	snap Snapshot
}

// updateStats 运行状态屏事件
func (m appModel) updateStats(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		}
	}
	if tick, ok := msg.(statsTickMsg); ok {
		_ = tick
		m.stats.snap = GetSnapshot()
		return m, tickStats()
	}
	return m, nil
}

// viewStats 运行状态屏渲染
func (m appModel) viewStats() string {
	s := m.stats.snap
	stats := s.Stats

	rows := []string{
		m.statRow("运行时间", formatDuration(s.Uptime)),
		m.statRow("进程 CPU", fmt.Sprintf("%.1f %%", s.CPUPercent)),
		m.statRow("进程内存", fmt.Sprintf("%.1f MB", s.WorkingSetMB)),
		m.statRow("Go 堆内存", fmt.Sprintf("%.1f MB", s.HeapMB)),
		m.statRow("系统可用内存", fmt.Sprintf("%.0f MB", s.SysAvailMB)),
		m.statRow("Goroutines", fmt.Sprintf("%d", s.Goroutines)),
		"",
		m.statRow("模型调用次数", fmt.Sprintf("%d 次", stats.Calls)),
		m.statRow("token 消耗总量", fmt.Sprintf("%d", stats.Total())+estimateMark(stats)),
		m.statRow("平均每次调用", fmt.Sprintf("%.0f token", stats.AvgPerCall())),
		m.statRow("模型输出平均速度", fmt.Sprintf("%.1f token/s", stats.SpeedTokensPerSec())),
		m.statRow("平均上下文 token", fmt.Sprintf("%.0f", stats.AvgContext())),
	}
	if stats.LastCompletion > 0 {
		rows = append(rows,
			m.statRow("最近调用", fmt.Sprintf("%s · %d token · %s",
				stats.LastCallAt.Format("15:04:05"), stats.LastCompletion, formatDuration(stats.LastDuration))))
	}

	panel := panelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	header := titleStyle.Render("「 运行状态 」")
	footer := footerStyle.Render("[Esc] 返回菜单   采样间隔 1s")
	return header + "\n" + panel + "\n" + footer
}

// statRow 状态行渲染
func (m appModel) statRow(label, value string) string {
	return labelStyle.Render(pad(label, 14)) + valueStyle.Render(value)
}

// estimateMark 存在估算调用时的标注
func estimateMark(stats ModelCallStats) string {
	if stats.EstimatedCalls > 0 {
		return dimStyle.Render(fmt.Sprintf("（含 %d 次估算）", stats.EstimatedCalls))
	}
	return ""
}

// pad 定宽填充（CJK 简单按 rune 数处理）
func pad(s string, n int) string {
	runes := []rune(s)
	for len(runes) < n {
		runes = append(runes, ' ')
	}
	return string(runes)
}

// formatDuration 时长格式化
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	sec := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, sec)
	}
	return fmt.Sprintf("%dm%02ds", m, sec)
}
