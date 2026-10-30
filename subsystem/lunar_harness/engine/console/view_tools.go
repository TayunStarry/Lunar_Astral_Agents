package console

// ==== Go 智能体工具列表屏 ====
//
// 扫描宿主汇总的 Go 智能体工具目录，按「智能体 → 工具 → 说明」展示。
// 目录内容来自 protocol/agentinfo：各智能体包描述自己的工具，app 层汇合后注入，
// 控制台不复制任何工具定义。

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// toolsState 工具列表屏状态
type toolsState struct {
	agents []agentinfo.Agent
	index  int
	pl     listPane
	note   string
}

// toolEntry 展平后的可选条目
type toolEntry struct {
	agentIndex int
	toolIndex  int
	tool       agentinfo.Tool
}

// toolsEnter 进入工具列表屏：重新扫描一次目录
func (m *appModel) toolsEnter() tea.Cmd {
	m.tools.pl = listPane{}
	m.tools.index = 0
	m.tools.agents = getToolCatalog()
	m.tools.note = ""
	if len(m.tools.agents) == 0 {
		m.tools.note = "宿主未接入工具目录扫描钩子，或当前构建没有可展示的 Go 智能体"
	}
	return nil
}

// toolEntries 展平工具条目；无工具的智能体用一条说明占位，保证仍然可见。
func (s toolsState) toolEntries() []toolEntry {
	out := make([]toolEntry, 0)
	for agentIndex, agent := range s.agents {
		if len(agent.Tools) == 0 {
			note := agent.Note
			if note == "" {
				note = "无 function calling 工具"
			}
			out = append(out, toolEntry{agentIndex: agentIndex, toolIndex: -1, tool: agentinfo.Tool{Name: "（无工具）", Description: note}})
			continue
		}
		for toolIndex, tool := range agent.Tools {
			out = append(out, toolEntry{agentIndex: agentIndex, toolIndex: toolIndex, tool: tool})
		}
	}
	return out
}

// toolCount 工具总数（不含占位条目）
func (s toolsState) toolCount() int {
	count := 0
	for _, agent := range s.agents {
		count += len(agent.Tools)
	}
	return count
}

// toolsKeys 工具列表屏键位
func toolsKeys() []keyHint {
	return []keyHint{
		kh("↑/↓", "选择"),
		kh("Esc", "返回"),
	}
}

// updateTools 工具列表屏事件
func (m appModel) updateTools(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	entries := m.tools.toolEntries()
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.screen = screenMenu
		return m, nil
	case "up", "k":
		m.tools.index = listMove(m.tools.index, -1, len(entries), true)
	case "down", "j":
		m.tools.index = listMove(m.tools.index, 1, len(entries), true)
	case "home", "g":
		m.tools.index = 0
	case "end", "G":
		if len(entries) > 0 {
			m.tools.index = len(entries) - 1
		}
	}
	return m, nil
}

// toolsContextLine 标题栏右侧摘要
func toolsContextLine(s toolsState) string {
	if len(s.agents) == 0 {
		return "未扫描到工具目录"
	}
	return fmt.Sprintf("%d 个智能体 · %d 个工具", len(s.agents), s.toolCount())
}

// viewTools 工具列表屏渲染
func (m appModel) viewTools() string {
	f := m.frameFor("工具列表", toolsContextLine(m.tools), toolsKeys())
	w, h := f.bodySize()

	// 不再重复页面来源与扫描方式：标题栏已给出「几个智能体 / 几个工具」，
	// 每个智能体区块也有统一的首行简述，顶部再写一遍只是冗余。
	head := make([]string, 0, 2)
	if m.tools.note != "" {
		head = append(head, "  "+warnStyle.Render(m.tools.note), "")
	}

	entries := m.tools.toolEntries()
	if len(entries) == 0 {
		return f.render(fitLines(head, h))
	}

	listW := w
	rowW := maxInt(8, listW-listGutter)
	blocks := make([][]string, len(entries))
	lastAgent := -1
	for i, entry := range entries {
		agent := m.tools.agents[entry.agentIndex]
		var block []string
		if entry.agentIndex != lastAgent {
			title := agent.Name + " · " + agent.Source
			block = append(block, sectionRule(title, rowW))
			if agent.Note != "" {
				for _, line := range wrapPlain(agent.Note, maxInt(8, rowW-6)) {
					block = append(block, "  "+dimStyle.Render(line))
				}
			}
			lastAgent = entry.agentIndex
		}

		selected := i == m.tools.index
		cursor := "  "
		if selected {
			cursor = cursorStyle.Render("▸ ")
		}
		name := labelStyle.Render(entry.tool.Name)
		if selected {
			name = selectStyle.Render(entry.tool.Name)
		}
		// 工具行只写工具名：所属智能体已由上方区块标题标明，逐行重复只是噪音。
		block = append(block, newRow(rowW, selected).
			L(cursor, name).
			String())
		for _, line := range wrapPlain(entry.tool.Description, maxInt(8, rowW-6)) {
			block = append(block, "     "+dimStyle.Render(line))
		}
		blocks[i] = block
	}

	lines, _ := m.tools.pl.layoutView(blocks, m.tools.index, listW, maxInt(4, h-len(head)-2))
	body := append(append([]string{}, head...), lines...)
	return f.render(fitLines(body, h))
}
