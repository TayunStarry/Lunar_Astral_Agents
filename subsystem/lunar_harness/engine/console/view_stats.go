package console

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

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
		case "3":
			return m.frontendRelaunch("library")
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
	f := m.frameFor("运行状态", "", engineKeys(m.engine))
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
	right = append(right, sectionRule(agentinfo.NameDesktopOperator+"智能体", rightW), "")
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
