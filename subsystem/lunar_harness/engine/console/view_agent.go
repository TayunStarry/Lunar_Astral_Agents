package console

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

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

// viewAgent 智能体实况屏渲染：轮次分段（思考暗色 / 输出常色 / 工具调用高亮）。
//
// 宽屏把「实况流」与「状态」分成两栏：左栏只放推理过程，右栏单独汇总轮次、
// 进行中数量、工具调用与视角；窄屏不强行分栏，状态退回底栏，避免正文被挤成窄条。
func (m appModel) viewAgent() string {
	f := m.frameFor("推理实况", "", agentKeys(m.agent))
	w, h := f.bodySize()

	streamW, railW := agentStreamWidth(w)
	turns := GetTurns()
	inner := maxInt(20, streamW-4)
	turnGap, sectionGap := agentGapsFor(h)
	lines := agentTurnLines(turns, inner, m.agent.expand, turnGap, sectionGap)
	m.agent.total = len(lines)

	win, offset, info := windowLines(lines, m.agent.offset, maxInt(1, h), m.agent.follow)
	m.agent.offset = offset

	body := win
	if railW > 0 {
		rail := m.agentStatusRail(railW, turns, info)
		sep := ruleStyle.Render(" │ ")
		body = make([]string, h)
		for i := 0; i < h; i++ {
			l, r := "", ""
			if i < len(win) {
				l = fitWidth(win[i], streamW)
			} else {
				l = fitWidth("", streamW)
			}
			if i < len(rail) {
				r = fitWidth(rail[i], railW)
			}
			body[i] = l + sep + r
		}
	}

	// 分栏时状态已经在右栏，底栏不再重复；单栏时状态必须落在底栏
	if railW == 0 {
		f.status = agentStatusText(turns, info.Total, m.agent.follow)
	}
	return f.render(body)
}

// 实况屏分栏阈值：正文达到此宽度才让出右侧状态栏。
// 数值按「侧栏 + 外框 + 左右两栏都还能读」反推，低于它时单栏更实用。
const (
	agentRailFrom = 84
	agentRailMin  = 20
	agentRailMax  = 24
)

// agentStreamWidth 构造实况屏分栏：返回正文宽度与右侧状态栏宽度（railW=0 表示不分栏）。
// 分栏时中间固定让出 3 列竖分隔，调用方据此保证左右两栏恰好铺满正文宽度。
func agentStreamWidth(bodyW int) (streamW, railW int) {
	if bodyW < agentRailFrom {
		return bodyW, 0
	}
	railW = clampInt(bodyW/4, agentRailMin, agentRailMax)
	streamW = bodyW - railW - 3
	if streamW < 30 {
		return bodyW, 0
	}
	return streamW, railW
}

// agentStatusRail 右侧状态栏：把「轮次 / 用时 / 步骤 / 工具 / 行数 / 视角 / 跟随」
// 从实况流里拿出来单独成栏，流内容只保留推理过程本身。
func (m appModel) agentStatusRail(width int, turns []*AgentTurn, info scrollInfo) []string {
	tools := 0
	current := "—"
	for _, turn := range turns {
		current = turn.Role
		tools += len(turn.Tools)
	}
	timing := turnTimingText(turns, time.Now())
	step := turnStep(turns)

	mode := "摘要"
	if m.agent.expand {
		mode = "全文"
	}
	const labelW = 7
	follow := okStyle.Render("● 跟随")
	if !m.agent.follow {
		follow = warnStyle.Render("‖ 暂停")
	}

	return []string{
		sectionRule("状态", width),
		"",
		kvRow("轮次", fmt.Sprintf("%d", len(turns)), labelW),
		kvRow("用时", timing, labelW),
		kvRow("步骤", step, labelW),
		kvRow("工具", fmt.Sprintf("%d 次", tools), labelW),
		kvRow("总行数", fmt.Sprintf("%d", info.Total), labelW),
		kvRow("视角", mode, labelW),
		dimStyle.Render(fitWidth("当前", labelW)) + valueStyle.Render(current),
		dimStyle.Render(fitWidth("滚动", labelW)) + follow,
	}
}

// turnTimingText 轮次用时：有进行中轮次时给「本轮」已用时长，
// 否则给最近一轮的「上轮」耗时（两者必须可区分，避免把历史当实时）。
func turnTimingText(turns []*AgentTurn, now time.Time) string {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Active {
			return "本轮 " + elapsedText(now.Sub(turns[i].StartedAt))
		}
	}
	if n := len(turns); n > 0 {
		t := turns[n-1]
		end := t.FinishedAt
		if end.IsZero() {
			end = now
		}
		return "上轮 " + elapsedText(end.Sub(t.StartedAt))
	}
	return "—"
}

// turnStep 当前步骤：取最近一轮的真实阶段（由流式事件派生）。
func turnStep(turns []*AgentTurn) string {
	if len(turns) == 0 {
		return "空闲"
	}
	if phase := turns[len(turns)-1].Phase; phase != "" {
		return phase
	}
	return "—"
}

// agentStatusText 底栏状态：窄屏（不分栏）时必须用一行讲清「看到哪、在做什么、是否跟随」。
// 这里是分栏状态栏的紧凑退化形态：去掉标签占位与「当前角色」，只保留滚动定位、
// 用时、步骤与跟随；「? 帮助」由底栏键位保证在最前，不会被状态文本挤掉。
func agentStatusText(turns []*AgentTurn, lines int, follow bool) string {
	state := okStyle.Render("● 跟随")
	if !follow {
		state = warnStyle.Render("‖ 暂停")
	}
	compact := fmt.Sprintf("%d轮·%d行·%s %s", len(turns), lines, turnTimingText(turns, time.Now()), turnStep(turns))
	return dimStyle.Render(compact) + "  " + state
}

// agentGapsFor 实况屏留白：轮次之间保留较明显的间隔，轮内（思考 / 工具 / 输出）
// 各留 1 行；内容区越矮越先收起轮内留白，保证窄小终端不会只剩空行。
func agentGapsFor(bodyH int) (turnGap, sectionGap int) {
	switch {
	case bodyH >= 18:
		return 2, 1
	case bodyH >= 12:
		return 1, 1
	case bodyH >= 6:
		return 1, 0
	default:
		return 0, 0
	}
}

// agentLines 构建实况内容行（历史轮次 + 进行中轮次）
func (m appModel) agentLines(width int, expand bool, turnGap, sectionGap int) []string {
	return agentTurnLines(GetTurns(), width, expand, turnGap, sectionGap)
}

// agentTurnLines 按给定轮次构建实况内容行（渲染与状态栏共用同一份快照）
func agentTurnLines(turns []*AgentTurn, width int, expand bool, turnGap, sectionGap int) []string {
	var lines []string
	if len(turns) > 0 {
		lines = append(lines, "") // 与标题框线之间留一行呼吸空间
	}
	for i, turn := range turns {
		if i > 0 {
			for n := 0; n < turnGap; n++ {
				lines = append(lines, "")
			}
		}
		lines = append(lines, renderTurnLines(turn, width, expand, sectionGap)...)
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
// sectionGap 控制轮内各段之间的空行数（0 = 紧凑，1 = 留一行）。
func renderTurnLines(turn *AgentTurn, width int, expand bool, sectionGap int) []string {
	var lines []string

	status := warnStyle.Render("… 进行中")
	if !turn.Active {
		status = okStyle.Render("✓ 完成") + dimStyle.Render(" "+turn.FinishedAt.Format("15:04:05"))
	}
	head := newRow(width, false).
		L(roleStyle.Render("◆ "+turn.Role), "  ", dimStyle.Render(turn.StartedAt.Format("15:04:05"))).
		R(status).
		String()
	lines = append(lines, head)

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
		lines = appendSectionGap(lines, sectionGap)
		lines = append(lines, thinkLabel(hidden, len(full), expand, width))
		for _, l := range shown {
			lines = append(lines, thinkStyle.Render("  │ "+l))
		}
	}
	// 工具调用
	if len(turn.Tools) > 0 {
		lines = appendSectionGap(lines, sectionGap)
		for _, tool := range turn.Tools {
			for i, l := range wrapPlain(tool, maxInt(8, width-6)) {
				if i == 0 {
					lines = append(lines, valueStyle.Render("  » "+l))
				} else {
					lines = append(lines, valueStyle.Render("    "+l))
				}
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
		lines = appendSectionGap(lines, sectionGap)
		lines = append(lines, outputLabel(hidden, len(full), expand, width))
		for _, l := range shown {
			lines = append(lines, labelStyle.Render("  │ "+l))
		}
	}
	return lines
}

// appendSectionGap 在段与段之间补空行（轮首不加，避免头部上方凭空多一行）
func appendSectionGap(lines []string, gap int) []string {
	for i := 0; i < gap; i++ {
		lines = append(lines, "")
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
