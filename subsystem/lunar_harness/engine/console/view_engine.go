package console

// ==== 运行状态屏的桌面操纵区块 ====
//
// 「运行状态」屏承载桌面操纵智能体的唯一人工入口（输入要求启动 / 随时中断）。
// 桌面操纵直接操作真实桌面键鼠，出于安全考虑不对月华 LTPX 工具链暴露。
//
// 对话链状态重建（自动/手动）已整体移除：实测无法真正恢复正常运行，
// 这里只保留桌面操纵入口。

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// engineState 运行状态屏的本地状态
type engineState struct {
	// msg 最近一次人工操作结果
	msg string
	// err msg 是否代表失败
	err bool
	// inputMode 是否正在输入桌面操纵任务要求
	inputMode bool
	// input 桌面操纵任务要求输入框
	input textinput.Model
}

// engineKeys 运行状态屏键位
func engineKeys(s engineState) []keyHint {
	if s.inputMode {
		return []keyHint{kh("Enter", "启动桌面操纵"), kh("Esc", "取消输入")}
	}
	if status, wired := getDesktopOperatorStatus(); wired && status.Running && status.Stopping {
		return []keyHint{kh("i", "输入桌面操纵要求"), kh("x", "强制结束"), kh("Esc", "返回")}
	}
	return []keyHint{kh("i", "输入桌面操纵要求"), kh("x", "中断桌面操纵"), kh("Esc", "返回")}
}

// newDesktopOperatorInput 创建桌面操纵任务要求输入框
func newDesktopOperatorInput() textinput.Model {
	input := textinput.New()
	input.Placeholder = "例如：打开记事本，输入今天的待办并保存"
	input.CharLimit = 500
	input.Focus()
	return input
}

// updateEngine 运行状态屏事件（桌面操纵入口）
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
		}
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
