package console

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// ==== 数据导出屏 ====
//
// 用途：把运行期数据落到磁盘，供离线排查（模型看到了什么、它怎么想的、
// 上下文怎么涨的、日志里到底哪条先发生）。导出范围按「运行数据 / Go 智能体」重划：
//   - 原始日志 → `.log`（JSON Lines）
//   - 推理实况 → `.json`（模型调用轮次：思考全文 / 输出全文 / 工具序列 / 时长）
//   - 核心人格 → `.json`（对话者 + 绘制者 + 演奏者 + 截图者 + 主上下文合并）
//   - 各 Go 智能体 → `.json`（网络搜索 / 哔哩观影 / 桌面操作 / 通讯桥接 / 互助论坛）
//
// 设计取舍：
//   - **所有导出项都是列表里可选的一行**，回车即导出。不把动作挂到底栏快捷键上：
//     底部键位栏在与右侧状态争宽时会被截断（见 layout.go composeRule 的取舍顺序
//     「状态不可丢、键位可压缩」），动作一多就会被截掉，用户看不到也按不到。
//   - 数字/字母热键只是**快捷键**，条目的显示键位来自自身 hotkey 字段而非行号，
//     因此今后插入新条目不会让既有热键发生位移。
//   - 导出是**一次性动作**而非持续录制：出问题时按一下、拿到文件，不引入后台写盘任务；
//   - 清单固定声明：导出目标来自 engine/agentdump 注册表，控制台不复制智能体实现；
//   - 只回显「文件 + 条数 + 大小」，不把内容打进 TUI。

// dumpKind 导出项的取数通道
type dumpKind int

const (
	// dumpKindLogs 原始日志（TUI 日志环形缓冲）
	dumpKindLogs dumpKind = iota
	// dumpKindTranscript 核心智能实况（TUI 轮次缓冲）
	dumpKindTranscript
	// dumpKindContext 智能体上下文（agentdump 注册表）
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

// dumpOptionList 可导出的数据清单：按「运行数据 / Go 智能体」分组。
// 快捷键统一为两位数字编号（01-13）：第一键定分组，第二键定条目，既不与
// 「↑/↓ 选择」冲突，也不会因为新增智能体而让既有编号位移。
var dumpOptionList = []dumpOption{
	{kind: dumpKindLogs, hotkey: "01", group: "运行数据", title: "原始日志",
		desc:    "全量 stdout + 结构化日志，导出为 .log（JSON Lines，可 grep / jq / tail）",
		toolTip: "正在导出原始日志…"},
	{kind: dumpKindTranscript, hotkey: "02", title: "推理实况",
		desc:    "模型调用轮次：思考全文 / 输出全文 / 工具调用序列 / 每轮耗时，导出为 .json",
		toolTip: "正在导出推理实况…"},

	{kind: dumpKindContext, key: "core", hotkey: "03", group: "Go 智能体", title: "核心人格",
		desc: "对话者 + 绘制者 + 演奏者 + 截图者 + 月华主上下文的合并快照"},
	{kind: dumpKindContext, key: "file_editor", hotkey: "04", title: "文件编辑",
		desc: "文件编辑的系统提示词、工具清单与文档库现状（按次执行，不保留上下文）"},
	{kind: dumpKindContext, key: "creative", hotkey: "05", title: "创作",
		desc: "创作体系的规划日志：上一轮计划、备忘、参考资料、错误与收尾简述"},
	{kind: dumpKindContext, key: "responder", hotkey: "06", title: "通用应答",
		desc: "通用应答的队列与运行状态：待处理数、特许运行、忙碌标记"},
	{kind: dumpKindContext, key: "savior", hotkey: "07", title: "智能修复",
		desc: "智能修复的待处理故障队列、尝试次数与静默截止时间"},
	{kind: dumpKindContext, key: "web_search", hotkey: "08", title: "网络搜索",
		desc: "最近一次检索的检索词、引擎、页数与结构化报告"},
	{kind: dumpKindContext, key: "bilibili", hotkey: "09", title: "哔哩观影",
		desc: "哔哩观影选片配置、已看视频规模与日记/历史记忆"},
	{kind: dumpKindContext, key: "desktop", hotkey: "10", title: "桌面操作",
		desc: "最近一次桌面操纵任务的要求、起止时间、结果或中断原因"},
	{kind: dumpKindContext, key: "descriptor", hotkey: "11", title: "描述者",
		desc: "描述者的文件查询工具清单与运行说明（按次执行，不保留上下文）"},
	{kind: dumpKindContext, key: "dialogue_bridging", hotkey: "12", title: "通讯桥接",
		desc: "最近一次多目标投递计划与逐项执行报告"},
	{kind: dumpKindContext, key: "mutual_forum", hotkey: "13", title: "互助论坛",
		desc: "互助论坛智能体最近的需求/答复上下文（模块：Mutual Forum；不含内部凭证）"},
}

// dumpState 数据导出屏状态
type dumpState struct {
	index   int      // 目标选中项
	pl      listPane // 目标列表滚动窗口
	running bool     // 是否有导出在进行
	keyBuf  string   // 两位数字编号的输入缓冲（第一键后等待第二键）
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
	m.dump.keyBuf = ""
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
		case "backspace":
			m.dump.keyBuf = ""
			return m, nil
		case "up", "k":
			m.dump.keyBuf = ""
			m.dump.index = listMove(m.dump.index, -1, len(dumpOptionList), true)
		case "down", "j":
			m.dump.keyBuf = ""
			m.dump.index = listMove(m.dump.index, 1, len(dumpOptionList), true)
		case "home", "g":
			m.dump.keyBuf = ""
			m.dump.index = 0
		case "end", "G":
			m.dump.keyBuf = ""
			m.dump.index = len(dumpOptionList) - 1
		case "enter", "right":
			// 回车导出选中项：所有导出动作都走这一条路径（含日志与实况），
			// 不依赖用户记住各自动作的独立快捷键
			m.dump.keyBuf = ""
			return m, m.dumpSelected()
		default:
			// 两位数字编号：第一键进入缓冲，第二键命中即导出
			if key >= "0" && key <= "9" && len(key) == 1 {
				return m.typeDumpDigits(key)
			}
		}
	}
	return m, nil
}

// typeDumpDigits 处理两位数字编号输入。
// 第一键只进入缓冲（不立即导出，避免 01 与 1 号动作歧义），第二键命中后导出。
func (m appModel) typeDumpDigits(key string) (tea.Model, tea.Cmd) {
	candidate := m.dump.keyBuf + key
	if idx := dumpHotkeyIndex(candidate); idx >= 0 {
		m.dump.keyBuf = ""
		m.dump.index = idx
		return m, m.dumpSelected()
	}
	if dumpHotkeyPrefixExists(candidate) {
		m.dump.keyBuf = candidate
		return m, nil
	}
	// 第二键不匹配：把它当作新的第一键（允许连续改选）
	if dumpHotkeyPrefixExists(key) {
		m.dump.keyBuf = key
		return m, nil
	}
	m.dump.keyBuf = ""
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

// dumpHotkeyPrefixExists 判断输入是否为某个编号的前缀。
func dumpHotkeyPrefixExists(prefix string) bool {
	if prefix == "" {
		return false
	}
	for _, opt := range dumpOptionList {
		if opt.hotkey != "" && len(opt.hotkey) > len(prefix) && opt.hotkey[:len(prefix)] == prefix {
			return true
		}
	}
	return false
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
			"  "+dimStyle.Render("核心人格需要月华运行时就绪；其余智能体需先运行过对应功能才有内容可导"))
	case m.dump.lastLabel != "":
		body = append(body,
			"  "+okStyle.Render("✓ 已写出 "+m.dump.lastResult.Path),
			"  "+dimStyle.Render(fmt.Sprintf("对象 %s · 记录 %d 条 · %s",
				m.dump.lastLabel, m.dump.lastResult.Records, humanBytes(m.dump.lastResult.Bytes))))
	default:
		hint := "↑/↓ 选择后回车导出；也可输入行右侧的两位数字编号"
		if m.dump.keyBuf != "" {
			hint = "已输入编号 " + m.dump.keyBuf + "_，再按一位数字完成导出"
		}
		body = append(body, "  "+dimStyle.Render(hint))
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
