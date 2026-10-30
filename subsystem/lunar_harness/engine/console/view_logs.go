package console

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 采样进度解析（引擎输出形如 "10/20"、"25%"、"1.02s/it"、"3.2it/s"）
var (
	drawStepTotalRe = regexp.MustCompile(`(\d+)\s*/\s*(\d+)`)
	drawPercentRe   = regexp.MustCompile(`(\d+)\s*%`)
	drawSpeedRe     = regexp.MustCompile(`([\d.]+)\s*(s/it|it/s|ms/it)`)
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
	case screenEdit:
		w, h := m.frameFor("", "", nil).bodySize()
		m.editView.sync(w, h)
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
	subtitle := logCategoryDescs[m.logCat]
	f := m.frameFor(title, subtitle, logsKeys())
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
		body = append(body, emptyState("暂无日志", subtitle+" 通道还没有任何输出。",
			"启动对应功能后此处实时刷新 · 左右方向键切换分类", w, maxInt(1, h-headRows-1))...)
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

// ==== 智能体实况屏 ====

// agentState 智能体实况屏状态（轮次内容实时取自 GetTurns）
type agentState struct {
	offset int
	follow bool
	total  int
	expand bool // 展开全文：思考与输出不再截尾，便于查看模型的完整想法
}

// agentKeys 智能体实况屏键位
func agentKeys(s agentState) []keyHint {
	expand := "展开思考"
	if s.expand {
		expand = "收起思考"
	}
	return []keyHint{
		kh("↑↓", "滚动"),
		kh("t", expand),
		kh("f", "跟随"),
		kh("Esc", "返回"),
	}
}

// updateAgent 智能体实况屏事件
func (m appModel) updateAgent(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		k := key.String()
		switch k {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		case "t":
			// 展开/收起思考：内容行数随之变化，回到跟随以显示最新内容
			m.agent.expand = !m.agent.expand
			m.agent.follow = true
			if m.agent.expand {
				m.setNotice("已展开模型思考", false)
			} else {
				m.setNotice("已收起模型思考", false)
			}
			return m, nil
		}
		if next, follow, ok := scrollKey(m.agent.offset, k, m.agent.total, m.bodyPage(), m.agent.follow); ok {
			m.agent.offset, m.agent.follow = next, follow
			return m, nil
		}
	}
	return m, nil
}

// viewAgent 智能体实况屏渲染：轮次分段（思考暗色 / 输出常色 / 工具调用高亮）
func (m appModel) viewAgent() string {
	f := m.frameFor("核心智能 实况", "", agentKeys(m.agent))
	w, h := f.bodySize()

	inner := maxInt(20, w-4)
	lines := m.agentLines(inner, m.agent.expand)
	m.agent.total = len(lines)

	win, offset, info := windowLines(lines, m.agent.offset, maxInt(1, h-1), m.agent.follow)
	m.agent.offset = offset

	body := append([]string{}, win...)
	state := okStyle.Render("● 跟随中")
	if !m.agent.follow {
		state = warnStyle.Render("‖ 已暂停（f 恢复跟随）")
	}
	status := newRow(w, false).L("  ").R(dimStyle.Render(fmt.Sprintf("%d 行", info.Total)), "  ", state).String()
	body = append(body, status)

	mode := "思考摘要"
	if m.agent.expand {
		mode = "思考全文"
	}
	f.status = dimStyle.Render("思考暗色 · 输出常色 · 工具高亮 · " + mode)
	return f.render(body)
}

// agentLines 构建实况内容行（历史轮次 + 进行中轮次）
func (m appModel) agentLines(width int, expand bool) []string {
	turns := GetTurns()
	var lines []string
	for _, turn := range turns {
		lines = append(lines, renderTurnLines(turn, width, expand)...)
	}
	if len(lines) == 0 {
		lines = append(lines,
			"",
			"  "+dimStyle.Render("（等待月华智能体的模型调用）"),
			"  "+dimStyle.Render("在月华前端发起一次对话，此处会实时显示思考、工具调用与输出。"),
		)
	}
	return lines
}

// 实况屏的行数预算：
//   - 摘要模式：思考与输出各只保留末尾若干行，避免长内容把轮次列表冲散；
//   - 全文模式：不再截尾（仅设一个上限保护帧率），用于查阅模型的完整想法。
const (
	agentThinkSummaryLines = 8
	agentThinkFullLines    = 400
	agentOutputSummary     = 14
	agentOutputFullLines   = 400
)

// renderTurnLines 渲染单轮：头部 + 思考 + 工具调用 + 输出。
//
// expand=false 时思考/输出只保留末尾摘要，并在标题行注明总行数（提示可按 t 展开）；
// expand=true 时输出完整内容，配合屏幕滚动即可通读模型的想法。
func renderTurnLines(turn *AgentTurn, width int, expand bool) []string {
	var lines []string

	status := warnStyle.Render("… 进行中")
	if !turn.Active {
		status = okStyle.Render("✓ 完成") + dimStyle.Render(" "+turn.FinishedAt.Format("15:04:05"))
	}
	head := newRow(width, false).
		L(roleStyle.Render("◆ "+turn.Role), "  ", dimStyle.Render(turn.StartedAt.Format("15:04:05"))).
		R(status).
		String()
	lines = append(lines, "", head)

	// 思考区：暗色，带「思考」标题与行数（不再用 emoji 表达）
	if s := strings.TrimRight(turn.Thinking, "\n"); s != "" {
		full := wrapPlain(s, maxInt(8, width-6))
		shown, hidden := full, 0
		if !expand && len(full) > agentThinkSummaryLines {
			hidden = len(full) - agentThinkSummaryLines
			shown = full[hidden:]
		} else if expand && len(full) > agentThinkFullLines {
			hidden = len(full) - agentThinkFullLines
			shown = full[hidden:]
		}
		lines = append(lines, thinkLabel(hidden, len(full), expand, width))
		for _, l := range shown {
			lines = append(lines, thinkStyle.Render("  │ "+l))
		}
	}
	// 工具调用
	for _, tool := range turn.Tools {
		for i, l := range wrapPlain(tool, maxInt(8, width-6)) {
			if i == 0 {
				lines = append(lines, valueStyle.Render("  » "+l))
			} else {
				lines = append(lines, valueStyle.Render("    "+l))
			}
		}
	}
	// 输出区：常色，同样支持全文
	if s := strings.TrimRight(turn.Content, "\n"); s != "" {
		full := wrapPlain(s, maxInt(8, width-4))
		shown, hidden := full, 0
		if !expand && len(full) > agentOutputSummary {
			hidden = len(full) - agentOutputSummary
			shown = full[hidden:]
		} else if expand && len(full) > agentOutputFullLines {
			hidden = len(full) - agentOutputFullLines
			shown = full[hidden:]
		}
		lines = append(lines, outputLabel(hidden, len(full), expand, width))
		for _, l := range shown {
			lines = append(lines, labelStyle.Render("  │ "+l))
		}
	}
	return lines
}

// thinkLabel 思考区标题：暗色 + 加粗，注明总行数与折叠情况（无 emoji）
func thinkLabel(hidden, total int, expand bool, width int) string {
	label := fmt.Sprintf("  思考 · 共 %d 行", total)
	switch {
	case hidden > 0 && !expand:
		label += fmt.Sprintf(" · 显示末尾 %d 行（t 展开全文）", total-hidden)
	case hidden > 0 && expand:
		label += fmt.Sprintf(" · 过长，仅显示末尾 %d 行", total-hidden)
	}
	return fitWidth(thinkHeadStyle.Render(truncTo(label, width)), width)
}

// outputLabel 输出区标题（同上）
func outputLabel(hidden, total int, expand bool, width int) string {
	label := fmt.Sprintf("  输出 · 共 %d 行", total)
	switch {
	case hidden > 0 && !expand:
		label += fmt.Sprintf(" · 显示末尾 %d 行（t 展开全文）", total-hidden)
	case hidden > 0 && expand:
		label += fmt.Sprintf(" · 过长，仅显示末尾 %d 行", total-hidden)
	}
	return fitWidth(valueStyle.Render(truncTo(label, width)), width)
}

// ==== 绘图引擎（sd.cpp）日志屏 ====

// renderDrawLogLine 绘图引擎单行排版：
//   - 错误/警告沿用红/橙并加标记；
//   - 进度/速度行以亮青特别标注（引擎主要输出就是这类采样节奏信息）；
//   - 其余引擎原始输出弱化为灰色，模块编排日志（Source=log）正常青色。
func renderDrawLogLine(line LogLine) string {
	switch {
	case line.Level == "ERROR":
		return logErrStyle.Render("✗ " + line.Text)
	case line.Level == "WARN":
		return logWarnStyle.Render("! " + line.Text)
	case isDrawProgressLine(line.Text):
		return drawProgressStyle.Render("▸ " + line.Text)
	case line.Source == "engine":
		return logDimStyle.Render("  " + line.Text)
	default:
		return logInfoStyle.Render("→ " + line.Text)
	}
}

// isDrawProgressLine 进度/速度行判定
func isDrawProgressLine(text string) bool {
	for _, pat := range drawProgressMarks {
		if strings.Contains(text, pat) {
			return true
		}
	}
	return false
}

// drawProgressMarks 进度/速度行的特征子串
var drawProgressMarks = []string{"s/it", "it/s", "ms/it", "%|", "==", "=>"}

// drawProgress 绘图引擎采样进度（从日志尾部解析最近一次报告）
type drawProgress struct {
	// Found 是否解析到进度信息
	Found bool
	// Step/Total 当前步 / 总步数（Total 未知时为 0）
	Step, Total int
	// Percent 采样百分比（无人为放大，0-100）
	Percent int
	// Speed 采样速度文本（如 "1.02s/it"）
	Speed string
}

// parseDrawProgress 从日志尾部解析最近的采样进度：
// 依次尝试「步/总步」与「百分比」两种写法，取最后一条命中记录。
func parseDrawProgress(logs []LogLine) drawProgress {
	var p drawProgress
	for _, l := range logs {
		text := l.Text
		if m := drawStepTotalRe.FindStringSubmatch(text); m != nil {
			step, total := atoiSafe(m[1]), atoiSafe(m[2])
			if total > 0 && step >= 0 {
				p = drawProgress{
					Found:   true,
					Step:    step,
					Total:   total,
					Percent: clampPercent(step * 100 / total),
					Speed:   p.Speed,
				}
			}
		} else if m := drawPercentRe.FindStringSubmatch(text); m != nil {
			percent := clampPercent(atoiSafe(m[1]))
			if p.Total > 0 {
				p.Percent = percent // 有总步数时以步数换算，百分比仅兜底
				p.Found = true
			} else if percent > 0 {
				p = drawProgress{Found: true, Percent: percent, Speed: p.Speed}
			}
		}
		if m := drawSpeedRe.FindStringSubmatch(text); m != nil {
			p.Speed = m[1] + m[2]
			p.Found = true
		}
	}
	return p
}

// clampPercent 百分比收敛到 0-100
func clampPercent(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// atoiSafe 宽松数字解析（解析失败返回 0）
func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// ==== 运行状态屏 ====

// statsState 运行状态屏状态
type statsState struct {
	snap Snapshot
}

// updateStats 运行状态屏事件：采样刷新 + 桌面操纵入口
func (m appModel) updateStats(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case statsTickMsg:
		m.stats.snap = GetSnapshot()
		return m, tickStats()
	case tea.KeyMsg:
		if m.engine.inputMode {
			return m.updateEngine(msg)
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		case "1":
			return m.frontendRelaunch("lunar")
		case "2":
			return m.frontendRelaunch("crystal")
		case "i", "I", "x", "X":
			return m.updateEngine(msg)
		}
	}
	return m, nil
}

// viewStats 运行状态屏：运行环境 / 模型计量 + 前端窗口 + 桌面操纵入口。
//
// 布局取舍：宽屏用两栏，左栏「机器与模型」、右栏「前端窗口 + 桌面操纵」；
// 窄屏退化为顺序堆叠。桌面任务输入作为底部全宽区块，避免表单被挤成窄条。
// 状态值按阈值着色（好=绿 / 注意=黄 / 异常=红），一眼能分辨当前是否健康。
func (m appModel) viewStats() string {
	f := m.frameFor("运行状态", "采样 1s · 运行观测", engineKeys(m.engine))
	w, h := f.bodySize()

	s := m.stats.snap
	stats := s.Stats
	keyW := 14

	footer := m.statsFooter(w)
	contentH := maxInt(6, h-len(footer))

	wide := w >= 96
	leftW, rightW := w, w
	if wide {
		leftW = (w - 3) / 2
		rightW = w - leftW - 3
	}

	version := s.BuildVersion
	if version == "" {
		version = "未知"
	}
	env := []string{
		sectionRule("运行环境", leftW),
		"",
		labelValue("构建版本", version, keyW),
		statusLabelValue("进程 CPU", fmt.Sprintf("%.1f %%", s.CPUPercent), highBadStyle(s.CPUPercent, 50, 80), keyW),
		labelValue("进程内存", fmt.Sprintf("%.1f MB", s.WorkingSetMB), keyW),
		statusLabelValue("可用内存", fmt.Sprintf("%.0f MB", s.SysAvailMB), lowBadStyle(s.SysAvailMB, 2048, 512), keyW),
		labelValue("Go 堆内存", fmt.Sprintf("%.1f MB", s.HeapMB), keyW),
		labelValue("Go 协程", fmt.Sprintf("%d", s.Goroutines), keyW),
	}
	model := []string{
		sectionRule("模型计量", leftW),
		"",
		labelValue("调用次数", fmt.Sprintf("%d 次", stats.Calls), keyW),
		contextTokensLine(stats, estimateMark(stats), keyW),
		labelValue("平均速度", fmt.Sprintf("%.1f token/s", stats.SpeedTokensPerSec()), keyW),
		statusLabelValue("缓存命中", fmt.Sprintf("%.1f %%", stats.CacheHitRate()*100), cacheHitStyle(stats.CacheHitRate()), keyW),
		statusLabelValue("峰值输入", fmt.Sprintf("%d token", stats.MaxPromptTokens), tokenInStyle, keyW),
		statusLabelValue("平均输入", fmt.Sprintf("%.0f token", stats.AvgContext()), tokenInStyle, keyW),
		statusLabelValue("峰值输出", fmt.Sprintf("%d token", stats.MaxCompletionTokens), tokenOutStyle, keyW),
		statusLabelValue("平均输出", fmt.Sprintf("%.0f token", stats.AvgCompletionPerCall()), tokenOutStyle, keyW),
	}

	// 右栏合并了原「前端页面」屏：先看两个前端窗口是否在线，再看桌面操纵状态。
	right := append(m.viewFrontendSection(rightW), "")
	right = append(right, sectionRule("桌面操纵智能体", rightW), "")
	right = append(right, m.viewDesktopOperatorState(rightW)...)

	var body []string
	if wide {
		left := fitLines(append(append([]string{}, env...), append([]string{""}, model...)...), contentH)
		rightLines := fitLines(right, contentH)
		separator := ruleStyle.Render(" │ ")
		body = make([]string, 0, contentH)
		for i := 0; i < contentH; i++ {
			body = append(body, fitWidth(left[i], leftW)+separator+rightLines[i])
		}
	} else {
		stacked := append(append([]string{}, env...), "")
		stacked = append(stacked, model...)
		stacked = append(stacked, "")
		stacked = append(stacked, right...)
		body = fitLines(stacked, contentH)
	}

	body = append(body, footer...)
	return f.render(fitLines(body, h))
}

// contextTokensLine 总上下文行：输入与输出使用不同色系，便于一眼区分上下文构成。
func contextTokensLine(stats ModelCallStats, mark string, labelW int) string {
	return dimStyle.Render(fitWidth("总上下文", labelW)) +
		valueStyle.Render(fmt.Sprintf("%d token", stats.Total())) +
		dimStyle.Render("（") +
		tokenInStyle.Render(fmt.Sprintf("%d", stats.PromptTokens)) +
		dimStyle.Render("/") +
		tokenOutStyle.Render(fmt.Sprintf("%d", stats.CompletionTokens)) +
		dimStyle.Render("）") + mark
}

// statusLabelValue 定宽标签 + 按状态着色的数值（绿=正常 / 黄=注意 / 红=异常）。
func statusLabelValue(label, value string, style lipgloss.Style, labelW int) string {
	return dimStyle.Render(fitWidth(label, labelW)) + style.Render(value)
}

// highBadStyle 越高越差：≥ errAt 红，≥ warnAt 黄，其余绿。
func highBadStyle(value, warnAt, errAt float64) lipgloss.Style {
	switch {
	case value >= errAt:
		return errStyle
	case value >= warnAt:
		return warnStyle
	default:
		return okStyle
	}
}

// lowBadStyle 越低越差：≤ errBelow 红，≤ warnBelow 黄，其余绿。
func lowBadStyle(value, warnBelow, errBelow float64) lipgloss.Style {
	switch {
	case value <= errBelow:
		return errStyle
	case value <= warnBelow:
		return warnStyle
	default:
		return okStyle
	}
}

// cacheHitStyle 缓存命中率越高越好（rate 为 0-1）。
func cacheHitStyle(rate float64) lipgloss.Style {
	switch {
	case rate >= 0.5:
		return okStyle
	case rate >= 0.2:
		return warnStyle
	default:
		return errStyle
	}
}

// statsFooter 底部：仅在需要输入或需要回显操作结果时出现。
//
// 空闲状态不再重复「按 i 启动 / 按 x 中断」：底部键位栏已经写明 i / x，
// 再写一遍只会挤占内容区高度（用户实测反馈的「赘述」）。
func (m appModel) statsFooter(w int) []string {
	if !m.engine.inputMode && m.engine.msg == "" {
		return nil
	}
	const labelW = 12
	lines := []string{"", sectionRule("桌面任务", w), ""}

	if m.engine.inputMode {
		lines = append(lines, formBox([]string{
			fieldRow("任务要求", m.engine.input, true, labelW),
		}, w)...)
		return lines
	}

	if m.engine.err {
		lines = append(lines, "  "+errStyle.Render("✗ "+m.engine.msg))
	} else {
		lines = append(lines, "  "+okStyle.Render("✓ "+m.engine.msg))
	}
	return lines
}

// estimateMark 存在估算调用时的标注
func estimateMark(stats ModelCallStats) string {
	if stats.EstimatedCalls > 0 {
		return dimStyle.Render(fmt.Sprintf("（含 %d 次估算）", stats.EstimatedCalls))
	}
	return ""
}
