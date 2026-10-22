package console

// ==== 月华引擎管理屏 ====
//
// 保留 goja 运行时的状态与手动重载；生活链已从 goja 层移除，
// 这里只提供一个“开始刷视频”的入口，实际浏览器操作由宿主 Go 模块完成。

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// engineDoneMsg 手动重载完成
type engineDoneMsg struct{ err error }

// biliLifeStartedMsg “开始刷视频”请求的受理结果
type biliLifeStartedMsg struct {
	detail string
	err    error
}

// engineState 引擎屏的本地状态
type engineState struct {
	// busy 手动重载进行中
	busy bool
	// msg 最近一次手动操作结果
	msg string
	// err msg 是否代表失败
	err bool
	// biliMsg 最近一次“开始刷视频”的受理结果
	biliMsg string
	// biliErr biliMsg 是否代表失败
	biliErr bool
}

// engineKeys 引擎屏键位
func engineKeys(s engineState) []keyHint {
	if s.busy {
		return []keyHint{kh("r", "重载进行中…"), kh("Esc", "返回")}
	}
	return []keyHint{kh("r", "重载引擎"), kh("b", "开始刷视频"), kh("Esc", "返回")}
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
			m.setBusy("重载月华引擎")
			return m, reloadEngineCmd("手动重载（控制台）")
		case "B", "b":
			m.engine.biliMsg = "正在启动浏览器…"
			m.engine.biliErr = false
			m.setNotice("B站生活：正在启动", false)
			return m, startBilibiliLifeCmd()
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
	case biliLifeStartedMsg:
		if msg.err != nil {
			m.engine.biliMsg = "启动失败: " + msg.err.Error()
			m.engine.biliErr = true
			m.setNotice("B站生活启动失败", true)
		} else {
			m.engine.biliMsg = msg.detail
			m.engine.biliErr = false
			m.setNotice("B站生活已启动", false)
		}
		return m, nil
	}
	return m, nil
}

// reloadEngineCmd 生成“重载引擎”命令
func reloadEngineCmd(reason string) tea.Cmd {
	return func() tea.Msg {
		return engineDoneMsg{err: reloadMoonEngine(reason)}
	}
}

// startBilibiliLifeCmd 生成“开始刷视频”命令。
// 宿主钩子只负责启动后台 goroutine 并立刻返回，不会阻塞 TUI。
func startBilibiliLifeCmd() tea.Cmd {
	return func() tea.Msg {
		detail, err := startBilibiliLife()
		return biliLifeStartedMsg{detail: detail, err: err}
	}
}

// viewEngine 引擎屏渲染：引擎状态 + 手动重载 + 开始刷视频
func (m appModel) viewEngine() string {
	f := m.frameFor("月华引擎 管理", "goja lunar_loop", engineKeys(m.engine))
	w, h := f.bodySize()

	status, wired := getEngineStatus()
	lines := append([]string{}, sectionRule("引擎状态", w), "")
	if !wired {
		lines = append(lines,
			"  "+warnStyle.Render("宿主未接入引擎自愈钩子（当前构建不支持重载）"),
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
		"按 " + keyStyle.Render("r") + " 换一块干净的 goja 运行时：尽力带回对话上下文（图片会省略），",
		"模型服务若已停止会一并拉起。重载期间对话链暂停，通常数秒；需重启模型服务时更久。",
	}
	for _, one := range hint {
		for _, seg := range wrapPlain(one, maxInt(20, w-4)) {
			lines = append(lines, "  "+dimStyle.Render(seg))
		}
	}

	lines = append(lines, "", sectionRule("B 站生活（真实浏览器）", w), "")
	lifeHints := []string{
		"按 " + keyStyle.Render("b") + " 开始一次刷视频：打开真实 B 站网页，未登录会停在登录页等你扫码。",
		"登录检测每 10 秒一次；60 秒没登录成功就关闭浏览器、放弃本次。",
		"登录后：选视频 → 停留详情页 → 下载并做 60 秒切片 → 逐片理解 → 读评论 → 写评论 → 单独发一条 @分享评论（@谁由 mention_targets 配置）→ 动态写日记 → 回复动态消息。",
		"全程只开一个 B 站窗口；一轮多个视频时按顺序逐个切换，不会同时开多个视频窗口。",
		"选片优先命中 bilibili_life.json 的 preference_keywords；涉政、男女对立、引战、录播/付费等词直接过滤。",
		"自动生活默认开启：每 interval_hours 判定一次，每天最多 daily_limit 次；这里按 b 强制触发不占次数。",
		"生活与对话互斥：新消息到达时，生活做完当前模型调用就暂停；对话完成且 10 秒无新消息后从断点续跑。",
		"生活链的模型输出会进「核心智能 实况」（主菜单 1），不用只盯日志干等。",
	}
	for _, one := range lifeHints {
		for _, seg := range wrapPlain(one, maxInt(20, w-4)) {
			lines = append(lines, "  "+dimStyle.Render(seg))
		}
	}
	if m.engine.biliMsg != "" {
		lines = append(lines, "")
		if m.engine.biliErr {
			lines = append(lines, "  "+errStyle.Render("✗ "+m.engine.biliMsg))
		} else {
			lines = append(lines, "  "+okStyle.Render("✓ "+m.engine.biliMsg))
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

	if m.engine.biliMsg != "" {
		if m.engine.biliErr {
			f.status = errStyle.Render("B站生活 ✗ " + m.engine.biliMsg)
		} else {
			f.status = okStyle.Render("B站生活 ✓ " + m.engine.biliMsg)
		}
	} else {
		f.status = dimStyle.Render("goja 运行时")
	}
	return f.render(lines)
}
