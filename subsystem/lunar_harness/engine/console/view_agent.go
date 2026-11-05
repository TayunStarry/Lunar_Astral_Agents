package console

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
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

// viewAgent 智能体实况屏渲染：轮次分段（思考暗色 / 输出常色 / 工具调用高亮）
func (m appModel) viewAgent() string {
	f := m.frameFor("推理实况", "", agentKeys(m.agent))
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
