package TuiSystem

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
	lines   int // 当前内容行数（位置指示与滚动步长用）
	lastSeq int // 上次构建时的日志条数
	lastW   int // 上次构建时的折行宽度
	cache   []string
	// renderLine 单行渲染器（nil=按级别/来源着色；绘图引擎屏注入专用排版）
	renderLine func(LogLine) string
}

// newLogsState 构建指定来源的日志视图
func newLogsState(source LogSource) logsState {
	return logsState{source: source, follow: true, lastSeq: -1}
}

// reset 丢弃缓存并要求重新构建（清空日志或重新进入日志屏时调用）
func (s *logsState) reset() {
	s.offset = 0
	s.follow = true
	s.cache = nil
	s.lastSeq = -1
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
	logs := GetLogs(s.source)
	if s.cache != nil && len(logs) == s.lastSeq && width == s.lastW {
		return s.cache
	}
	if width < 8 {
		width = 8
	}
	lines := make([]string, 0, len(logs)+1)
	for _, line := range logs {
		lines = append(lines, wrapPlain(s.render(line), width)...)
	}
	if len(lines) == 0 {
		lines = append(lines, dimStyle.Render("（暂无日志：本通道还没有输出）"))
	}
	s.cache = lines
	s.lastSeq = len(logs)
	s.lastW = width
	s.lines = len(lines)
	return s.cache
}

// syncViewMetrics 刷新「滚动窗口」相关的派生度量（行数）。
// Update 与 View 共用：日志/实况这类内容的行数决定滚动上限，
// 若只在 View 中计算，按方向键时拿到的行数会一直是 0，滚动便不会生效。
func (m *appModel) syncViewMetrics() {
	switch m.screen {
	case screenRaw, screenLunarJS, screenLTP9, screenGoWarn, screenDraw:
		w, _ := m.frameFor("", "", nil).bodySize()
		m.logsStateOf(m.screenSource()).content(w)
	case screenAgent:
		w, _ := m.frameFor("", "", nil).bodySize()
		m.agent.total = len(m.agentLines(maxInt(20, w-4), m.agent.expand))
	}
}

// screenSource 当前屏对应的日志来源（非日志屏返回 SourceRaw）
func (m appModel) screenSource() LogSource {
	switch m.screen {
	case screenLunarJS:
		return SourceLunarJS
	case screenLTP9:
		return SourceLTP9JS
	case screenDraw:
		return SourceDraw
	case screenGoWarn:
		return SourceGoWarn
	}
	return SourceRaw
}

// updateLogs 日志屏事件（source 指定日志屏之一）。
// 注意：状态必须从接收者副本内取（s 指向 m 自己的字段）——若由调用方传入
// 指向其副本的指针，滚动等变更会落在中间副本上，随返回值一并丢失
// （表现为 ↑/↓/PgUp/PgDn 按了没反应）。
func (m appModel) updateLogs(source LogSource, msg tea.Msg) (tea.Model, tea.Cmd) {
	s := m.logsStateOf(source)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		case "c":
			ClearLogs(s.source)
			s.reset()
			m.setNotice("已清空日志缓冲", false)
			return m, nil
		}
		if next, follow, ok := scrollKey(s.offset, key, s.lines, m.bodyPage(), s.follow); ok {
			s.offset, s.follow = next, follow
			return m, nil
		}
	}
	return m, nil
}

// logsStateOf 取指定日志屏的状态指针（指向接收者自身的字段）
func (m *appModel) logsStateOf(source LogSource) *logsState {
	switch source {
	case SourceLunarJS:
		return &m.lunarView
	case SourceLTP9JS:
		return &m.ltp9View
	case SourceDraw:
		return &m.drawView
	case SourceGoWarn:
		return &m.warnView
	default:
		return &m.rawView
	}
}

// logsKeys 日志屏键位
func logsKeys() []keyHint {
	return []keyHint{
		kh("↑/↓ PgUp/PgDn", "滚动"),
		kh("Home/End", "顶部 / 底部"),
		kh("f", "跟随开关（回看时暂停）"),
		kh("c", "清空"),
		kh("Esc", "返回菜单"),
	}
}

// viewLogs 日志屏渲染：定高状态条 + 折行日志窗口
func (m appModel) viewLogs(s *logsState, title, subtitle string) string {
	f := m.frameFor(title, subtitle, logsKeys())
	w, h := f.bodySize()

	lines := s.content(w)
	win, offset, info := windowLines(lines, s.offset, maxInt(1, h-1), s.follow)
	s.offset = offset

	body := append([]string{}, win...)
	body = append(body, logStatusLine(w, info, s.follow))
	f.status = dimStyle.Render(fmt.Sprintf("共 %d 行", info.Total))
	return f.render(body)
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
		kh("↑/↓ PgUp/PgDn", "滚动"),
		kh("t", expand),
		kh("f", "跟随开关"),
		kh("Esc", "返回菜单"),
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

// thinkHeadStyle 思考/输出标题行（加粗暗色，用于分区而非依赖 emoji）
var thinkHeadStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))

// ==== 绘图引擎（sd.cpp）日志屏 ====

// drawProgressStyle 采样进度行（亮青，突出「进行到哪一步」）
var drawProgressStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))

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

// viewDraw 绘图引擎日志屏渲染：定高进度条 + 日志窗口
func (m appModel) viewDraw() string {
	f := m.frameFor("绘图引擎日志", "sd.cpp", logsKeys())
	w, h := f.bodySize()

	// 顶部：引擎名 + 最近一次采样进度（无进度时提示等待）
	strip := []string{}
	if p := parseDrawProgress(GetLogs(SourceDraw)); p.Found {
		line := progressBar(p.Percent, minInt(w-8, 40))
		if p.Total > 0 {
			line += "  " + valueStyle.Render(fmt.Sprintf("步 %d/%d", p.Step, p.Total))
		}
		if p.Speed != "" {
			line += "  " + drawProgressStyle.Render(p.Speed)
		}
		strip = append(strip, "  "+line)
	} else {
		strip = append(strip, "  "+dimStyle.Render("（尚无采样进度：发起图像生成后此处实时显示）"))
	}

	lines := m.drawView.content(w)
	winH := maxInt(1, h-len(strip)-1)
	win, offset, info := windowLines(lines, m.drawView.offset, winH, m.drawView.follow)
	m.drawView.offset = offset

	body := append([]string{}, strip...)
	body = append(body, win...)
	body = append(body, logStatusLine(w, info, m.drawView.follow))

	f.status = dimStyle.Render(fmt.Sprintf("共 %d 行", info.Total))
	return f.render(body)
}

// ==== 运行状态屏 ====

// statsState 运行状态屏状态
type statsState struct {
	snap Snapshot
}

// statsKeys 运行状态屏键位
func statsKeys() []keyHint {
	return []keyHint{kh("Esc", "返回菜单")}
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
	if _, ok := msg.(statsTickMsg); ok {
		m.stats.snap = GetSnapshot()
		return m, tickStats()
	}
	return m, nil
}

// viewStats 运行状态屏渲染：宽屏双栏（运行环境 / 模型计量）
func (m appModel) viewStats() string {
	f := m.frameFor("运行状态", "采样间隔 1s", statsKeys())
	w, h := f.bodySize()

	s := m.stats.snap
	stats := s.Stats
	keyW := 16

	// 宽屏双栏：左右两栏各自按栏宽生成区块线，避免分隔线被截成省略号
	twoCol := w >= 88
	colW, rightW := w, w
	if twoCol {
		colW = (w - 3) / 2
		rightW = w - colW - 3
	}

	env := []string{
		sectionRule("运行环境", colW),
		"",
		labelValue("运行时间", formatDuration(s.Uptime), keyW),
		labelValue("进程 CPU", fmt.Sprintf("%.1f %%", s.CPUPercent), keyW),
		labelValue("进程内存", fmt.Sprintf("%.1f MB", s.WorkingSetMB), keyW),
		labelValue("Go 堆内存", fmt.Sprintf("%.1f MB", s.HeapMB), keyW),
		labelValue("系统可用内存", fmt.Sprintf("%.0f MB", s.SysAvailMB), keyW),
		labelValue("Goroutines", fmt.Sprintf("%d", s.Goroutines), keyW),
	}

	model := []string{
		sectionRule("模型计量", rightW),
		"",
		labelValue("调用次数", fmt.Sprintf("%d 次", stats.Calls), keyW),
		labelValue("token 消耗总量", fmt.Sprintf("%d", stats.Total())+estimateMark(stats), keyW),
		labelValue("平均每次调用", fmt.Sprintf("%.0f token", stats.AvgPerCall()), keyW),
		labelValue("输出平均速度", fmt.Sprintf("%.1f token/s", stats.SpeedTokensPerSec()), keyW),
		labelValue("平均上下文", fmt.Sprintf("%.0f token", stats.AvgContext()), keyW),
	}
	if stats.LastCompletion > 0 {
		model = append(model, "",
			labelValue("最近调用", fmt.Sprintf("%s · %d token · %s",
				stats.LastCallAt.Format("15:04:05"), stats.LastCompletion, formatDuration(stats.LastDuration)), keyW))
	}

	var body []string
	if twoCol {
		left := fitLines(env, h)
		right := fitLines(model, h)
		separator := ruleStyle.Render(" │ ")
		body = make([]string, 0, h)
		for i := 0; i < h; i++ {
			body = append(body, fitWidth(left[i], colW)+separator+right[i])
		}
	} else {
		body = append(body, env...)
		body = append(body, "")
		body = append(body, model...)
	}
	return f.render(body)
}

// estimateMark 存在估算调用时的标注
func estimateMark(stats ModelCallStats) string {
	if stats.EstimatedCalls > 0 {
		return dimStyle.Render(fmt.Sprintf("（含 %d 次估算）", stats.EstimatedCalls))
	}
	return ""
}
