package console

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
)

// logsState 日志视图状态（多个日志屏共用同一结构）
//
// 不走 bubbles/viewport：日志需要「按显示宽度软折行 + 跟随/暂停 + 位置指示」
// 三件套，自建定高行窗口反而更少意外，且与列表窗口共用同一套滚动语义。
type logsState struct {
	source  LogSource
	offset  int
	follow  bool
	lines   int    // 当前内容行数（位置指示与滚动步长用）
	lastSeq uint64 // 上次构建时的日志追加版本号（不能用条数：环形缓冲满后长度不变）
	lastW   int    // 上次构建时的折行宽度
	cache   []string
	// renderLine 单行渲染器（nil=按级别/来源着色；绘图引擎屏注入专用排版）
	renderLine func(LogLine) string
}

// newLogsState 构建指定来源的日志视图
func newLogsState(source LogSource) logsState {
	return logsState{source: source, follow: true}
}

// reset 丢弃缓存并要求重新构建（清空日志或重新进入日志屏时调用）
func (s *logsState) reset() {
	s.offset = 0
	s.follow = true
	s.cache = nil
	s.lastSeq = 0
	s.lastW = 0
	s.lines = 0
}

// render 单行渲染：优先使用注入的专用渲染器
func (s *logsState) render(line LogLine) string {
	if s.renderLine != nil {
		return s.renderLine(line)
	}
	return renderLogLine(line)
}

// content 构建（或复用缓存的）折行后的内容行。
// 必须同时被 Update（刷新行数供滚动计算）与 View（实际取内容）调用：
// 只在 View 里更新的派生状态不会写回模型，按键时便看不到真实行数。
func (s *logsState) content(width int) []string {
	logs, version := GetLogsWithVersion(s.source)
	if s.cache != nil && version == s.lastSeq && width == s.lastW {
		return s.cache
	}
	if width < 8 {
		width = 8
	}
	lines := make([]string, 0, len(logs)+1)
	for _, line := range logs {
		lines = append(lines, wrapPlain(s.render(line), width)...)
	}
	s.cache = lines
	s.lastSeq = version
	s.lastW = width
	s.lines = len(lines)
	return s.cache
}

// syncViewMetrics 刷新「滚动窗口」相关的派生度量（行数）。
// Update 与 View 共用：日志/实况这类内容的行数决定滚动上限，
// 若只在 View 中计算，按方向键时拿到的行数会一直是 0，滚动便不会生效。
func (m *appModel) syncViewMetrics() {
	switch m.screen {
	case screenLogs:
		w, _ := m.frameFor("", "", nil).bodySize()
		m.logView.content(w)
	case screenAgent:
		w, _ := m.frameFor("", "", nil).bodySize()
		m.agent.total = len(m.agentLines(maxInt(20, w-4), m.agent.expand))
	}
}

// updateLogs 「系统日志」屏事件：左右方向键切换分类，上下方向键滚动。
func (m appModel) updateLogs(msg tea.Msg) (tea.Model, tea.Cmd) {
	s := &m.logView

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		case "left", "h":
			m.switchLogCategory(-1)
			return m, nil
		case "right", "l":
			m.switchLogCategory(1)
			return m, nil
		case "c":
			ClearLogs(s.source)
			s.reset()
			m.setNotice("已清空「"+logCategoryTitles[s.source]+"」日志缓冲", false)
			return m, nil
		}
		if next, follow, ok := scrollKey(s.offset, key, s.lines, m.bodyPage(), s.follow); ok {
			s.offset, s.follow = next, follow
			return m, nil
		}
	}
	return m, nil
}

// switchLogCategory 切换日志分类（重置视图缓存，绘图分类注入专用行排版）。
func (m *appModel) switchLogCategory(delta int) {
	n := len(logCategories)
	if n == 0 {
		return
	}
	cur := 0
	for i, category := range logCategories {
		if category == m.logCat {
			cur = i
			break
		}
	}
	next := ((cur+delta)%n + n) % n
	m.logCat = logCategories[next]
	m.logView = newLogsState(m.logCat)
	if m.logCat == SourceDraw {
		m.logView.renderLine = renderDrawLogLine
	}
	m.setNotice("日志分类："+logCategoryTitles[m.logCat], false)
}

// logsKeys 系统日志屏键位
func logsKeys() []keyHint {
	return []keyHint{
		kh("←/→", "切换分类"),
		kh("↑↓", "滚动"),
		kh("f", "跟随"),
		kh("c", "清空"),
		kh("Esc", "返回"),
	}
}

// viewSystemLogs 「系统日志」屏渲染：分类标签条 + 日志窗口。
func (m appModel) viewSystemLogs() string {
	title := "系统日志"
	f := m.frameFor(title, "", logsKeys())
	w, h := f.bodySize()

	tabs := logCategoryTabs(w, m.logCat)
	strip := []string{}
	if m.logCat == SourceDraw {
		if p := parseDrawProgress(GetLogs(SourceDraw)); p.Found {
			line := progressBar(p.Percent, minInt(w-8, 40))
			if p.Total > 0 {
				line += "  " + valueStyle.Render(fmt.Sprintf("步 %d/%d", p.Step, p.Total))
			}
			if p.Speed != "" {
				line += "  " + drawProgressStyle.Render(p.Speed)
			}
			strip = append(strip, "  "+line)
		}
	}

	lines := m.logView.content(w)
	headRows := len(tabs) + len(strip)
	if len(lines) == 0 {
		body := append([]string{}, tabs...)
		body = append(body, strip...)
		body = append(body, emptyState("暂无日志", "当前通道还没有任何输出。", "启动对应功能后此处实时刷新 · 左右方向键切换分类", w, maxInt(1, h-headRows-1))...)
		body = append(body, logStatusLine(w, scrollInfo{}, m.logView.follow))
		f.status = dimStyle.Render("共 0 行")
		return f.render(body)
	}

	winH := maxInt(1, h-headRows-1)
	win, offset, info := windowLines(lines, m.logView.offset, winH, m.logView.follow)
	m.logView.offset = offset

	body := append([]string{}, tabs...)
	body = append(body, strip...)
	body = append(body, win...)
	body = append(body, logStatusLine(w, info, m.logView.follow))

	f.status = dimStyle.Render(fmt.Sprintf("共 %d 行", info.Total))
	return f.render(body)
}

// logCategoryTabs 分类标签条：当前分类反白高亮，其余弱化。
func logCategoryTabs(width int, current LogSource) []string {
	parts := make([]string, 0, len(logCategories))
	for _, category := range logCategories {
		label := " " + logCategoryTitles[category] + " "
		if category == current {
			parts = append(parts, selRowStyle.Render(label))
		} else {
			parts = append(parts, faintStyle.Render(label))
		}
	}
	line := "  " + strings.Join(parts, " ")
	return []string{fitWidth(line, width)}
}

// logStatusLine 日志窗底部状态条：位置百分比 + 跟随状态
func logStatusLine(w int, info scrollInfo, follow bool) string {
	pct := 100
	if info.Total > 0 {
		pct = clampPercent((info.Top + info.View) * 100 / info.Total)
	}
	state := okStyle.Render("● 跟随中")
	if !follow {
		state = warnStyle.Render("‖ 已暂停（f 恢复跟随）")
	}
	pos := ""
	if info.Total > info.View {
		pos = dimStyle.Render(fmt.Sprintf("%d-%d/%d 行 · %d%%",
			info.Top+1, minInt(info.Top+info.View, info.Total), info.Total, pct))
	}
	return newRow(w, false).L("  ").R(pos, "  ", state).String()
}
