package console

// ==== 月华对话链管理屏 ====
//
// 展示 Go 对话链状态并支持手动重建；
// 同时承载桌面操纵智能体的唯一人工入口（输入要求启动 / 随时中断）。
// 桌面操纵直接操作真实桌面键鼠，出于安全考虑不对月华 LTPX 工具链暴露。

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
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
	// inputMode 是否正在输入桌面操纵任务要求
	inputMode bool
	// input 桌面操纵任务要求输入框
	input textinput.Model
}

// engineKeys 引擎相关键位（运行状态屏底部提示）
func engineKeys(s engineState) []keyHint {
	if s.inputMode {
		return []keyHint{kh("Enter", "启动桌面操纵"), kh("Esc", "取消输入")}
	}
	if s.busy {
		return []keyHint{kh("r", "重建进行中…"), kh("i", "输入桌面操纵要求"), kh("x", "中断/强制结束"), kh("Esc", "返回")}
	}
	if status, wired := getDesktopOperatorStatus(); wired && status.Running && status.Stopping {
		return []keyHint{kh("r", "重建对话链"), kh("i", "输入桌面操纵要求"), kh("x", "强制结束"), kh("Esc", "返回")}
	}
	return []keyHint{kh("r", "重建对话链"), kh("i", "输入桌面操纵要求"), kh("x", "中断桌面操纵"), kh("Esc", "返回")}
}

// newDesktopOperatorInput 创建桌面操纵任务要求输入框
func newDesktopOperatorInput() textinput.Model {
	input := textinput.New()
	input.Placeholder = "例如：打开记事本，输入今天的待办并保存"
	input.CharLimit = 500
	input.Focus()
	return input
}

// updateEngine 引擎屏事件
func (m appModel) updateEngine(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.engine.inputMode {
			return m.updateEngineInput(msg)
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		case "i", "I":
			m.engine.input = newDesktopOperatorInput()
			m.engine.inputMode = true
			m.engine.msg = ""
			return m, textinput.Blink
		case "x", "X":
			status, wired := getDesktopOperatorStatus()
			if !wired {
				m.engine.msg = "宿主未接入桌面操纵控制钩子"
				m.engine.err = true
				return m, nil
			}
			if !status.Running {
				m.engine.msg = "桌面操纵当前未运行"
				m.engine.err = false
				return m, nil
			}
			stopDesktopOperator()
			if status.Stopping {
				m.engine.msg = "已强制结束桌面操纵"
				m.engine.err = false
				m.setNotice("桌面操纵已强制结束", false)
			} else {
				m.engine.msg = "已请求中断；若 4 秒内未退出，再按 x 可强制结束"
				m.engine.err = false
				m.setNotice("桌面操纵中断中", false)
			}
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

// updateEngineInput 桌面操纵任务要求输入态事件
func (m appModel) updateEngineInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.engine.inputMode = false
		m.engine.input.Blur()
		return m, nil
	case "enter":
		instruction := strings.TrimSpace(m.engine.input.Value())
		if instruction == "" {
			m.engine.msg = "请先输入要桌面操纵执行的桌面任务"
			m.engine.err = true
			return m, nil
		}
		if err := startDesktopOperator(instruction); err != nil {
			m.engine.msg = "桌面操纵启动失败: " + err.Error()
			m.engine.err = true
			m.setNotice("桌面操纵启动失败", true)
			return m, nil
		}
		m.engine.inputMode = false
		m.engine.input.Blur()
		m.engine.input.SetValue("")
		m.engine.msg = "桌面操纵已启动，按 x 可随时中断"
		m.engine.err = false
		m.setNotice("桌面操纵已启动", false)
		return m, nil
	}
	var cmd tea.Cmd
	m.engine.input, cmd = m.engine.input.Update(msg)
	return m, cmd
}

// reloadEngineCmd 生成“重建对话链”命令
func reloadEngineCmd(reason string) tea.Cmd {
	return func() tea.Msg {
		return engineDoneMsg{err: reloadMoonEngine(reason)}
	}
}

// viewEngineChainState 对话链状态区块（未接入钩子时给出明确结论，而不是一片空白）
func viewEngineChainState(w int) []string {
	const labelW = 12

	status, wired := getEngineStatus()
	if !wired {
		return []string{
			"  " + warnStyle.Render("宿主未接入对话链状态钩子（当前构建不支持重建）"),
			"  " + dimStyle.Render("若界面出现「对话链停摆」，只能重启程序恢复。"),
		}
	}

	lines := make([]string, 0, 12)
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
	return lines
}

// viewDesktopOperatorState 桌面操纵运行状态区块（未接入钩子时明确提示）
func (m appModel) viewDesktopOperatorState(w int) []string {
	const labelW = 12
	status, wired := getDesktopOperatorStatus()
	if !wired {
		return []string{
			"  " + warnStyle.Render("宿主未接入桌面操纵控制钩子（当前构建无法在此启动）"),
			"  " + dimStyle.Render("桌面操纵仅允许从本屏人工启动，不对月华工具链暴露。"),
		}
	}

	lines := make([]string, 0, 6)
	stateText := okStyle.Render("空闲")
	if status.Stopping {
		stateText = warnStyle.Render(m.spinner.View() + " 停止中…")
	} else if status.Running {
		stateText = selectStyle.Render(m.spinner.View() + " 运行中")
	}
	lines = append(lines, "  "+kvRow("状态", stateText, labelW))
	addWrap := func(label, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
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
	addWrap("本轮要求", status.Instruction)
	if status.Running {
		addWrap("已运行", formatDuration(time.Since(status.StartedAt)))
	}
	if status.Running && status.Stopping {
		addWrap("提示", "已请求中断，等待任务退出；再按 x 可立即强制结束")
	}
	if !status.Running {
		if status.Error != "" {
			addWrap("最近结果", errStyle.Render(status.Error))
		} else if status.Result != "" {
			addWrap("最近结果", status.Result)
		}
	}
	return lines
}
