package TuiSystem

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// ==== 数据导出屏 ====
//
// 用途：把运行期数据落到磁盘，供离线排查（模型看到了什么、上下文怎么涨的、
// 日志里到底哪条先发生）。两类数据、两种格式：
//   - 原始日志 → `.log`（JSON Lines：一行一条 JSON，可直接 grep / jq / tail）
//   - 智能体上下文 → `.json`（月华主上下文，或任选一个子智能体）
//
// 设计取舍：
//   - 导出是**一次性动作**而非持续录制：出问题时按一下、拿到文件，不引入后台写盘任务；
//   - 目标清单固定声明（见 dumpTargets）：子智能体集合是编译期已知的，
//     动态发现只会引入"列表里突然多出一个不可用目标"的不确定性；
//   - 只回显「文件 + 条数 + 大小」，不把内容打进 TUI：上下文可达数 MB，
//     塞进终端既看不清也拖慢渲染，要看内容请直接打开文件。

// dumpTarget 一个可导出的上下文目标
type dumpTarget struct {
	key   string // 传给宿主的 agent 名
	title string // 显示名
	desc  string // 说明（导出的是哪份上下文）
}

// dumpTargetList 可导出的上下文目标清单（与月华侧 data-dump.ts 的注册表对应）
var dumpTargetList = []dumpTarget{
	{key: "lunar", title: "月华 主上下文", desc: "未读消息队列 + 长期记忆摘要（进入对话者前的输入）"},
	{key: "dialogue", title: "对话者", desc: "与用户交互的主智能体：最近 48 条上下文（含图片标签降级结果）"},
	{key: "painter", title: "绘制者", desc: "绘图子智能体：任务描述与绘画工具返回"},
	{key: "musician", title: "演奏者", desc: "音乐子智能体：任务描述与乐理工具返回"},
	{key: "perceiver", title: "感知者", desc: "视频 / 动态图理解子智能体"},
	{key: "memorizer", title: "记忆者", desc: "长期记忆写入与检索摘要子智能体"},
	{key: "description", title: "描述者", desc: "图片内容描述子智能体（降级占位与 describe_images 共用）"},
}

// dumpState 数据导出屏状态
type dumpState struct {
	index   int      // 目标选中项
	pl      listPane // 目标列表滚动窗口
	running bool     // 是否有导出在进行
	lastErr string   // 最近一次失败原因
	// last 最近一次成功回执（{显示名, 结果}）
	lastLabel  string
	lastResult DataDumpResult
}

// newDumpState 构建数据导出屏状态
func newDumpState() dumpState {
	return dumpState{}
}

// dumpEnter 进入数据导出屏
func (m *appModel) dumpEnter() tea.Cmd {
	m.dump.pl = listPane{}
	m.dump.running = false
	m.dump.lastErr = ""
	return nil
}

// dumpDoneMsg 导出完成
type dumpDoneMsg struct {
	label  string // 导出对象（显示名）
	result DataDumpResult
	err    error
}

// updateDump 数据导出屏事件
func (m appModel) updateDump(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case dumpDoneMsg:
		m.dump.running = false
		if msg.err != nil {
			m.dump.lastErr = msg.err.Error()
			m.setNotice("导出失败", true)
			return m, nil
		}
		m.dump.lastErr = ""
		m.dump.lastLabel = msg.label
		m.dump.lastResult = msg.result
		m.setNotice(fmt.Sprintf("已导出 %s（%d 条 / %s）", msg.result.Path, msg.result.Records, humanBytes(msg.result.Bytes)), false)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		case "up", "k":
			m.dump.index = listMove(m.dump.index, -1, len(dumpTargetList), true)
		case "down", "j":
			m.dump.index = listMove(m.dump.index, 1, len(dumpTargetList), true)
		case "home", "g":
			m.dump.index = 0
		case "end", "G":
			m.dump.index = len(dumpTargetList) - 1
		case "1", "2", "3", "4", "5", "6", "7":
			// 数字键直达并导出（与主菜单"数字即动作"的手感一致）
			idx := int(msg.String()[0] - '1')
			if idx >= 0 && idx < len(dumpTargetList) {
				m.dump.index = idx
				return m, m.dumpContext()
			}
		case "l":
			return m, m.dumpLogs()
		case "enter", "right":
			return m, m.dumpContext()
		}
	}
	return m, nil
}

// dumpContext 导出当前选中的智能体上下文
func (m *appModel) dumpContext() tea.Cmd {
	if m.dump.running {
		return nil
	}
	target := dumpTargetList[m.dump.index]
	m.dump.running = true
	m.setNotice("正在导出 "+target.title+"…", false)
	return func() tea.Msg {
		res, err := DumpContext(target.key)
		return dumpDoneMsg{label: target.title, result: res, err: err}
	}
}

// dumpLogs 导出原始日志
func (m *appModel) dumpLogs() tea.Cmd {
	if m.dump.running {
		return nil
	}
	m.dump.running = true
	m.setNotice("正在导出原始日志…", false)
	return func() tea.Msg {
		res, err := DumpLogs()
		return dumpDoneMsg{label: "原始日志", result: res, err: err}
	}
}

// dumpKeys 数据导出屏键位（底栏与帮助页同源）
func dumpKeys(s dumpState) []keyHint {
	if s.running {
		return []keyHint{kh("…", "导出进行中")}
	}
	return []keyHint{
		kh("↑/↓", "选择目标"),
		kh("1-7", "直达并导出上下文"),
		kh("Enter", "导出选中上下文 (.json)"),
		kh("l", "导出原始日志 (.log)"),
		kh("Esc", "返回菜单"),
	}
}

// dumpContextLine 数据导出屏上下文提示（标题栏右侧）
func dumpContextLine(s dumpState) string {
	if s.running {
		return "导出中…"
	}
	if s.lastErr != "" {
		return "上次导出失败"
	}
	if s.lastLabel != "" {
		return "上次导出：" + s.lastLabel
	}
	return "导出文件落在 local_data/logs/dump/"
}

// viewDump 数据导出屏渲染
func (m appModel) viewDump() string {
	f := m.frameFor("数据导出", dumpContextLine(m.dump), dumpKeys(m.dump))
	w, h := f.bodySize()

	body := []string{
		dimStyle.Render("  两类数据：原始日志 → .log（JSON Lines，可 grep/jq）；智能体上下文 → .json"),
		"",
	}

	// 目标列表：选中项高亮；条目数固定但高度可能不足，故仍走统一滚动窗口
	listW := w
	rowW := maxInt(8, listW-listGutter)
	blocks := make([][]string, len(dumpTargetList))
	for i, t := range dumpTargetList {
		sel := i == m.dump.index
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		keyTag := dimStyle.Render(fmt.Sprintf("[%d]", i+1))
		title := labelStyle.Render(t.title)
		if sel {
			keyTag = keyStyle.Render(fmt.Sprintf("[%d]", i+1))
			title = selectStyle.Render(t.title)
		}
		blocks[i] = []string{
			listRow(rowW, sel, cursor, title, keyTag),
			newRow(rowW, false).L("     ", dimStyle.Render(truncTo(t.desc, maxInt(4, rowW-6)))).String(),
		}
	}
	lines, _ := m.dump.pl.layoutView(blocks, m.dump.index, listW, maxInt(4, h-6))
	body = append(body, lines...)
	body = append(body, "")

	switch {
	case m.dump.running:
		body = append(body, "  "+selectStyle.Render(m.spinner.View())+" "+valueStyle.Render("正在导出…"))
	case m.dump.lastErr != "":
		body = append(body, "  "+errStyle.Render("✗ 导出失败："+m.dump.lastErr),
			"  "+dimStyle.Render("智能体上下文导出依赖月华运行时；若提示未就绪，请等月华启动完成后再试"))
	case m.dump.lastLabel != "":
		body = append(body,
			"  "+okStyle.Render("✓ 已写出 "+m.dump.lastResult.Path),
			"  "+dimStyle.Render(fmt.Sprintf("对象 %s · 记录 %d 条 · %s",
				m.dump.lastLabel, m.dump.lastResult.Records, humanBytes(m.dump.lastResult.Bytes))))
	default:
		body = append(body, "  "+dimStyle.Render("↑/↓ 选择目标后回车导出；按 l 导出原始日志"))
	}

	return f.render(fitLines(body, h))
}

// humanBytes 字节数的人类可读写法
func humanBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
