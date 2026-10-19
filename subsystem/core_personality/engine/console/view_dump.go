package console

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// ==== 数据导出屏 ====
//
// 用途：把运行期数据落到磁盘，供离线排查（模型看到了什么、它怎么想的、
// 上下文怎么涨的、日志里到底哪条先发生）。三类数据、两种格式：
//   - 原始日志 → `.log`（JSON Lines：一行一条 JSON，可直接 grep / jq / tail）
//   - 核心智能实况 → `.json`（模型调用轮次：思考全文 / 输出全文 / 工具序列 / 时长）
//   - 智能体上下文 → `.json`（月华主上下文，或任选一个子智能体）
//
// 三者的数据来源互不重叠，因此导出项也分开：
//   - 实况轮次在 TUI 的轮次缓冲里（本模块直接可见）；
//   - 智能体上下文只在月华 goja 运行时内（经宿主钩子向运行时索取）。
//
// 设计取舍：
//   - **所有导出项都是列表里可选的一行**，回车即导出。不把动作挂到底栏快捷键上：
//     底部键位栏在与右侧状态争宽时会被截断（见 layout.go composeRule 的取舍顺序
//     「状态不可丢、键位可压缩」），动作一多就会被截掉，用户看不到也按不到。
//   - 数字/字母热键只是**快捷键**，条目的显示键位来自自身 hotkey 字段而非行号，
//     因此今后插入新条目不会让既有热键发生位移。
//   - 导出是**一次性动作**而非持续录制：出问题时按一下、拿到文件，不引入后台写盘任务；
//   - 清单固定声明：子智能体集合是编译期已知的，动态发现只会引入
//     "列表里突然多出一个不可用目标"的不确定性；
//   - 只回显「文件 + 条数 + 大小」，不把内容打进 TUI：上下文可达数 MB，
//     塞进终端既看不清也拖慢渲染，要看内容请直接打开文件。

// dumpKind 导出项的取数通道
type dumpKind int

const (
	// dumpKindLogs 原始日志（TUI 日志环形缓冲）
	dumpKindLogs dumpKind = iota
	// dumpKindTranscript 核心智能实况（TUI 轮次缓冲）
	dumpKindTranscript
	// dumpKindContext 智能体上下文（月华 goja 运行时）
	dumpKindContext
)

// dumpOption 数据导出屏的一个可选项
type dumpOption struct {
	kind    dumpKind
	key     string // dumpKindContext 时为传给宿主的 agent 名；其余为空
	hotkey  string // 数字/字母快捷键（显示于行右）
	group   string // 分组标题（渲染时在该组首条之前插入）
	title   string // 显示名
	desc    string // 说明（导出的是哪份数据）
	toolTip string // 进行中提示
}

// dumpOptionList 可导出的数据清单：顺序即显示顺序，按「日志 / 实况 / 上下文」分组。
//
// 热键分配刻意让上下文那组继续占用 1-7（与既有习惯一致），
// 两个新的运行数据用 l / t——它们是「日志」与「实况」的助记。
var dumpOptionList = []dumpOption{
	{kind: dumpKindLogs, hotkey: "l", group: "运行数据", title: "原始日志",
		desc:    "全量 stdout + 结构化日志，导出为 .log（JSON Lines，可 grep / jq / tail）",
		toolTip: "正在导出原始日志…"},
	{kind: dumpKindTranscript, hotkey: "t", group: "运行数据", title: "核心智能 实况",
		desc:    "模型调用轮次：思考全文 / 输出全文 / 工具调用序列 / 每轮耗时，导出为 .json",
		toolTip: "正在导出核心智能实况…"},

	{kind: dumpKindContext, key: "lunar", hotkey: "1", group: "智能体上下文", title: "月华 主上下文",
		desc: "未读消息队列 + 长期记忆摘要（进入对话者前的输入）"},
	{kind: dumpKindContext, key: "dialogue", hotkey: "2", title: "对话者",
		desc: "与用户交互的主智能体：最近 48 条上下文（含图片标签降级结果）"},
	{kind: dumpKindContext, key: "painter", hotkey: "3", title: "绘制者",
		desc: "绘图子智能体：任务描述与绘画工具返回"},
	{kind: dumpKindContext, key: "musician", hotkey: "4", title: "演奏者",
		desc: "音乐子智能体：任务描述与乐理工具返回"},
	{kind: dumpKindContext, key: "perceiver", hotkey: "5", title: "感知者",
		desc: "视频 / 动态图理解子智能体"},
	{kind: dumpKindContext, key: "memorizer", hotkey: "6", title: "记忆者",
		desc: "长期记忆写入与检索摘要子智能体"},
	{kind: dumpKindContext, key: "description", hotkey: "7", title: "描述者",
		desc: "图片内容描述子智能体（降级占位与 describe_images 共用）"},
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
		key := msg.String()
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		case "up", "k":
			m.dump.index = listMove(m.dump.index, -1, len(dumpOptionList), true)
		case "down", "j":
			m.dump.index = listMove(m.dump.index, 1, len(dumpOptionList), true)
		case "home", "g":
			m.dump.index = 0
		case "end", "G":
			m.dump.index = len(dumpOptionList) - 1
		case "enter", "right":
			// 回车导出选中项：所有导出动作都走这一条路径（含日志与实况），
			// 不依赖用户记住各自动作的独立快捷键
			return m, m.dumpSelected()
		default:
			// 字母/数字热键：定位到该条目并立即导出（与主菜单"键即动作"的手感一致）
			if idx := dumpHotkeyIndex(key); idx >= 0 {
				m.dump.index = idx
				return m, m.dumpSelected()
			}
		}
	}
	return m, nil
}

// dumpHotkeyIndex 热键 → 条目下标（未命中返回 -1）。
// 依 hotkey 字段查找而非按下标推算，故插入新条目不会造成热键位移。
func dumpHotkeyIndex(key string) int {
	if key == "" {
		return -1
	}
	for i, opt := range dumpOptionList {
		if opt.hotkey != "" && opt.hotkey == key {
			return i
		}
	}
	return -1
}

// dumpSelected 导出当前选中的条目（按取数通道分发）
func (m *appModel) dumpSelected() tea.Cmd {
	if m.dump.running || m.dump.index < 0 || m.dump.index >= len(dumpOptionList) {
		return nil
	}
	opt := dumpOptionList[m.dump.index]
	toolTip := opt.toolTip
	if toolTip == "" {
		toolTip = "正在导出 " + opt.title + "…"
	}
	m.dump.running = true
	m.setNotice(toolTip, false)

	label := opt.title
	switch opt.kind {
	case dumpKindLogs:
		return func() tea.Msg {
			res, err := DumpLogs()
			return dumpDoneMsg{label: label, result: res, err: err}
		}
	case dumpKindTranscript:
		return func() tea.Msg {
			res, err := DumpTranscript()
			return dumpDoneMsg{label: label, result: res, err: err}
		}
	default:
		target := opt.key
		return func() tea.Msg {
			res, err := DumpContext(target)
			return dumpDoneMsg{label: label, result: res, err: err}
		}
	}
}

// dumpKeys 数据导出屏键位（底栏与帮助页同源）。
//
// 刻意只留三条：底栏与右侧状态争宽时会被截断（layout.go composeRule：状态不可丢、
// 键位可压缩），把每个导出动作都列上去会互相挤掉。动作本身已在列表里可选，
// 热键也标在各自行上，底栏无需重复枚举。
func dumpKeys(s dumpState) []keyHint {
	if s.running {
		return []keyHint{kh("…", "导出进行中")}
	}
	return []keyHint{
		kh("↑/↓", "选择"),
		kh("Enter", "导出"),
		kh("Esc", "返回"),
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

	// 实况轮次的现有条数：让用户在选中"核心智能 实况"之前就知道有没有东西可导
	store := GetTurnStoreInfo()
	head := []string{
		dimStyle.Render("  导出运行期数据到 local_data/logs/dump/（日志为 .log / JSONL，其余为 .json）"),
		dimStyle.Render(fmt.Sprintf("  实况现有 %d 轮（上限 %d，累计 %d 轮，已淘汰 %d）",
			store.HistoryLen+boolToInt(store.HasActive), store.HistoryCap, store.Created, store.Dropped)),
		"",
	}

	// 列表：选中项高亮；按 group 插分组标题；条数可能超出高度，故走统一滚动窗口
	listW := w
	rowW := maxInt(8, listW-listGutter)
	blocks := make([][]string, len(dumpOptionList))
	for i, opt := range dumpOptionList {
		var b []string
		if i == 0 || dumpOptionList[i-1].group != opt.group { // 组首插入分组标题
			b = append(b, sectionRule(opt.group, rowW))
		}
		sel := i == m.dump.index
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		// 键位标签取自条目本身（不是行号）：插入新条目不会让既有热键位移
		tag := "[" + opt.hotkey + "]"
		keyTag := dimStyle.Render(tag)
		title := labelStyle.Render(opt.title)
		if sel {
			keyTag = keyStyle.Render(tag)
			title = selectStyle.Render(opt.title)
		}
		b = append(b, listRow(rowW, sel, cursor, title, keyTag))
		b = append(b, newRow(rowW, false).
			L("     ", dimStyle.Render(truncTo(opt.desc, maxInt(4, rowW-6)))).String())
		blocks[i] = b
	}
	lines, _ := m.dump.pl.layoutView(blocks, m.dump.index, listW, maxInt(4, h-7))
	body := append(append([]string{}, head...), lines...)
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
		body = append(body, "  "+dimStyle.Render("↑/↓ 选择后回车导出；也可直接按行右侧的键位"))
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

// boolToInt 布尔转 0/1（用于"是否有进行中轮次"的展示计数）
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
