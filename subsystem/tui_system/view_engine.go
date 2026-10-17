package TuiSystem

// ==== 月华引擎（goja lunar_loop）管理屏 ====
//
// 这是「思维链停摆」的现场急救台：
//   - 上半屏给出引擎的真实状态（运行时、脚本、重载次数、连续失败）——
//     此前这类事故在界面上**完全没有痕迹**，用户只能看到月华不说话；
//   - 下半屏是手动重载（R）：换一块干净的 goja 运行时，尽力把对话上下文一并带回。
//
// 自动重载由宿主侧判定（连续失败达阈值），本屏只负责把它发生过这件事显示出来。

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// engineDoneMsg 手动重载完成
type engineDoneMsg struct{ err error }

// engineState 引擎屏的本地状态（跨重载保留最近一次手动操作结果）
type engineState struct {
	// busy 手动重载进行中
	busy bool
	// msg 最近一次手动重载的结果
	msg string
	// err msg 是否代表失败
	err bool
}

// engineKeys 引擎屏键位
func engineKeys(s engineState) []keyHint {
	if s.busy {
		return []keyHint{kh("R", "重载进行中…"), kh("Esc", "返回")}
	}
	return []keyHint{kh("R", "重载引擎"), kh("Esc", "返回")}
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
		case "R":
			if m.engine.busy {
				return m, nil
			}
			m.engine.busy = true
			m.engine.msg = ""
			m.setBusy("重载月华引擎")
			return m, reloadEngineCmd("手动重载（控制台）")
		}
	case engineDoneMsg:
		m.engine.busy = false
		if msg.err != nil {
			m.engine.msg = "重载失败: " + msg.err.Error()
			m.engine.err = true
			m.setNotice("月华引擎重载失败", true)
		} else {
			m.engine.msg = "重载完成，思考链已恢复"
			m.engine.err = false
			m.setNotice("月华引擎已重载", false)
		}
		return m, nil
	}
	return m, nil
}

// reloadEngineCmd 生成"重载引擎"命令（阻塞到重载完成，避免 TUI 先跑到一半的状态）
func reloadEngineCmd(reason string) tea.Cmd {
	return func() tea.Msg {
		return engineDoneMsg{err: reloadMoonEngine(reason)}
	}
}

// viewEngine 引擎屏渲染：状态表 + 手动重载说明
func (m appModel) viewEngine() string {
	f := m.frameFor("月华引擎 管理", "goja lunar_loop", engineKeys(m.engine))
	w, h := f.bodySize()

	status, wired := getEngineStatus()
	lines := append([]string{}, sectionRule("引擎状态", w), "")
	if !wired {
		lines = append(lines,
			"  "+warnStyle.Render("宿主未接入引擎自愈钩子（当前构建不支持重载）"),
			"  "+dimStyle.Render("若界面出现「思维链停摆」，只能重启程序恢复。"),
		)
		return f.render(fitLines(lines, h))
	}

	// 就绪徽标：运行时与脚本分开看——只"进程活着"不算能思考
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
		bootText = fmt.Sprintf("%s（已运行 %s）",
			status.BootedAt.Format("15:04:05"), formatDuration(time.Since(status.BootedAt)))
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

	addKV("运行时", runtimeText)
	addKV("智能体脚本", agentText)
	addKV("启动时间", bootText)
	addKV("重载次数", fmt.Sprintf("%d 次", status.ReloadCount))
	addKV("最近重载", reloadText)
	addWrap("重载结果", status.LastReloadResult)
	addKV("连续失败", failText)
	if status.LastFailure != "" {
		when := ""
		if !status.LastFailureAt.IsZero() {
			when = status.LastFailureAt.Format("15:04:05") + " · "
		}
		addWrap("最近失败", when+status.LastFailure)
	}
	addKV("自动重载", autoText)

	lines = append(lines, "", sectionRule("手动重载", w), "")
	hint := []string{
		"按 " + keyStyle.Render("R") + " 换一块干净的 goja 运行时：尽力带回对话上下文（图片会省略），",
		"模型服务若已停止会一并拉起。重载期间思考链暂停，通常数秒；需重启模型服务时更久。",
		"取舍：优先让思维链继续跑，其次才是保住上下文。",
	}
	for _, one := range hint {
		for _, seg := range wrapPlain(one, maxInt(20, w-4)) {
			lines = append(lines, "  "+dimStyle.Render(seg))
		}
	}

	if m.engine.busy {
		lines = append(lines, "", "  "+selectStyle.Render(m.spinner.View()+" 正在重载…"))
	} else if m.engine.msg != "" {
		lines = append(lines, "")
		if m.engine.err {
			lines = append(lines, "  "+errStyle.Render("✗ "+m.engine.msg))
		} else {
			lines = append(lines, "  "+okStyle.Render("✓ "+m.engine.msg))
		}
	}

	f.status = dimStyle.Render("goja 运行时")
	return f.render(lines)
}
