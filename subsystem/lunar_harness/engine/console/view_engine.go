package console

// ==== 月华对话链管理屏 ====
//
// 展示 Go 对话链状态并支持手动重建。

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// engineDoneMsg 手动重建完成
type engineDoneMsg struct{ err error }

// engineState 引擎屏的本地状态
type engineState struct {
	// busy 手动重建进行中
	busy bool
	// msg 最近一次手动操作结果
	msg string
	// err msg 是否代表失败
	err bool
}

// engineKeys 引擎屏键位
func engineKeys(s engineState) []keyHint {
	if s.busy {
		return []keyHint{kh("r", "重建进行中…"), kh("Esc", "返回")}
	}
	return []keyHint{kh("r", "重建对话链"), kh("Esc", "返回")}
}

// updateEngine 引擎屏事件
func (m appModel) updateEngine(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		case "R", "r":
			if m.engine.busy {
				return m, nil
			}
			m.engine.busy = true
			m.engine.msg = ""
			m.setBusy("重建月华对话链")
			return m, reloadEngineCmd("手动重建（控制台）")
		}
	case engineDoneMsg:
		m.engine.busy = false
		if msg.err != nil {
			m.engine.msg = "重建失败: " + msg.err.Error()
			m.engine.err = true
			m.setNotice("月华对话链重建失败", true)
		} else {
			m.engine.msg = "重建完成，对话链已恢复"
			m.engine.err = false
			m.setNotice("月华对话链已重建", false)
		}
		return m, nil
	}
	return m, nil
}

// reloadEngineCmd 生成“重建对话链”命令
func reloadEngineCmd(reason string) tea.Cmd {
	return func() tea.Msg {
		return engineDoneMsg{err: reloadMoonEngine(reason)}
	}
}

// viewEngine 对话链屏渲染：状态 + 手动重建
func (m appModel) viewEngine() string {
	f := m.frameFor("月华对话链 管理", "Go native", engineKeys(m.engine))
	w, h := f.bodySize()

	status, wired := getEngineStatus()
	lines := append([]string{}, sectionRule("对话链状态", w), "")
	if !wired {
		lines = append(lines,
			"  "+warnStyle.Render("宿主未接入对话链状态钩子（当前构建不支持重建）"),
			"  "+dimStyle.Render("若界面出现「对话链停摆」，只能重启程序恢复。"),
		)
		return f.render(fitLines(lines, h))
	}

	runtimeText := okStyle.Render("✓ 就绪")
	if !status.RuntimeReady {
		runtimeText = errStyle.Render("✗ 未就绪")
	}
	agentText := okStyle.Render("✓ 已加载")
	if !status.AgentReady {
		agentText = errStyle.Render("✗ 未加载")
	}

	bootText := "-"
	if !status.BootedAt.IsZero() {
		bootText = fmt.Sprintf("%s（已运行 %s）", status.BootedAt.Format("15:04:05"), formatDuration(time.Since(status.BootedAt)))
	}
	reloadText := "从未重载"
	if !status.LastReloadAt.IsZero() {
		reloadText = fmt.Sprintf("%s（%s）", status.LastReloadAt.Format("01-02 15:04:05"), status.LastReloadReason)
	}
	failText := fmt.Sprintf("连续 %d 次", status.ConsecutiveFailures)
	if status.ConsecutiveFailures > 0 {
		failText = warnStyle.Render(failText)
	} else {
		failText = okStyle.Render(failText)
	}
	autoText := okStyle.Render(fmt.Sprintf("开启（连续 %d 次失败触发）", status.AutoReloadThreshold))
	if !status.AutoReload {
		autoText = faintStyle.Render("已关闭（仅手动）")
	}

	const labelW = 12
	addKV := func(label, value string) {
		lines = append(lines, "  "+kvRow(label, value, labelW))
	}
	addWrap := func(label, text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		first := true
		for _, one := range wrapPlain(text, maxInt(20, w-labelW-6)) {
			if first {
				lines = append(lines, "  "+kvRow(label, one, labelW))
				first = false
				continue
			}
			lines = append(lines, "  "+strings.Repeat(" ", labelW+2)+dimStyle.Render(one))
		}
	}

	addKV("对话链", runtimeText)
	addKV("智能体", agentText)
	addKV("启动时间", bootText)
	addKV("重建次数", fmt.Sprintf("%d 次", status.ReloadCount))
	addKV("最近重建", reloadText)
	addWrap("重建结果", status.LastReloadResult)
	addKV("连续失败", failText)
	if status.LastFailure != "" {
		when := ""
		if !status.LastFailureAt.IsZero() {
			when = status.LastFailureAt.Format("15:04:05") + " · "
		}
		addWrap("最近失败", when+status.LastFailure)
	}
	addKV("自动重建", autoText)

	lines = append(lines, "", sectionRule("手动重建", w), "")
	hint := []string{
		"按 " + keyStyle.Render("r") + " 清空并重建 Go 对话链状态（上下文会重置），",
		"模型服务若已停止会一并拉起。重建期间对话链暂停；模型服务若已停止会一并拉起。",
	}
	for _, one := range hint {
		for _, seg := range wrapPlain(one, maxInt(20, w-4)) {
			lines = append(lines, "  "+dimStyle.Render(seg))
		}
	}

	if m.engine.busy {
		lines = append(lines, "", "  "+selectStyle.Render(m.spinner.View()+" 正在重建…"))
	} else if m.engine.msg != "" {
		lines = append(lines, "")
		if m.engine.err {
			lines = append(lines, "  "+errStyle.Render("✗ "+m.engine.msg))
		} else {
			lines = append(lines, "  "+okStyle.Render("✓ "+m.engine.msg))
		}
	}

	f.status = dimStyle.Render("Go 对话链")
	return f.render(lines)
}
