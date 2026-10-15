package TuiSystem

import (
	"fmt"
	"strings"
	"time"
)

// keyHint 一条键位提示（键 + 说明）。同一份数据同时驱动底栏与帮助页，
// 避免「文档里写的键」与「界面提示的键」两处维护、日久失配。
type keyHint struct {
	key   string
	label string
}

// kh 构造键位提示
func kh(key, label string) keyHint { return keyHint{key: key, label: label} }

// globalKeys 全屏通用键位
var globalKeys = []keyHint{
	kh("↑/↓ k/j", "移动选择"),
	kh("Enter", "确认 / 进入"),
	kh("Esc", "返回上级"),
	kh("?", "键位帮助"),
	kh("Ctrl+C", "退出程序"),
}

// 框体尺寸下限：低于此尺寸改用极简提示，避免渲染出破碎的框线与错位的文字
const (
	frameMinWidth  = 44
	frameMinHeight = 10
	// noticeTTL 底部/标题栏结果提示的保留时长
	noticeTTL = 8 * time.Second
)

// frame 屏幕骨架：统一「标题栏 / 定高内容区 / 键位栏」三段式布局。
//
// 内容区的行数与列宽被强制约束在终端边界之内（裁剪 + 补齐到恰好），
// 因此任何屏幕都不可能把内容顶出终端——「列表超出终端高度且无法滚动」
// 这类缺陷由此在框架层被根除，而不是靠每个屏各自小心翼翼。
type frame struct {
	width, height int

	brand   string // 标题栏左：品牌
	title   string // 标题栏右：当前屏名
	context string // 标题栏右：上下文（仓库 / 筛选 / 模式）
	extra   string // 标题栏右：运行指示（任务 / 时长 / 时钟）

	keys   []keyHint
	status string // 键位栏右：位置 / 进度 / 状态
}

// renderWidth 实际用于排版的列数：比终端宽度少 1 列，末列留空。
//
// 为什么让出最后一列（实测所得）：经典 conhost 上「正好写满整行」的行会在换行时丢掉最后一个
// 字符——实测中文控制台字体下两条框线的右角 ╮ / ╯ 直接消失，因为紧跟着的 CRLF 把它吞了
// （Windows Terminal 无此问题，conhost 的末列延迟换行怪癖）。让出 1 列即可免疫，
// 视觉上无感；同时也避开了「写到最后一格触发换行」这一族老问题。
func (f frame) renderWidth() int {
	w := f.width - 1
	if w < 1 {
		w = 1
	}
	return w
}

// bodySize 内容区尺寸：框线占左右各 1 列、上下各 1 行
func (f frame) bodySize() (int, int) {
	w, h := f.renderWidth()-2, f.height-2
	if w < 1 {
		w = 1
	}
	if h < 0 {
		h = 0
	}
	return w, h
}

// bodyPage 当前屏内容区高度（作为列表翻页步长）
func (m appModel) bodyPage() int {
	_, h := m.frameFor("", "", nil).bodySize()
	return maxInt(1, h-2)
}

// tooSmall 终端尺寸不足以承载框体
func (f frame) tooSmall() bool {
	return f.width < frameMinWidth || f.height < frameMinHeight
}

// render 渲染整屏：内容区被裁剪/补齐为恰好 bodySize 行，整屏高度恒等于终端高度。
//
// 行宽由 dispWidth（终端实测宽度）保证，见 theme.go / glyphwidth.go：
// 中文控制台字体下部分字形占 2 列，度量若按 1 列算，整行会超出终端而折行，
// 整屏高度翻倍 → 表现为「一打开就滚动 + 显示混乱」。
func (f frame) render(body []string) string {
	if f.tooSmall() {
		return f.renderTiny(body)
	}
	w, h := f.bodySize()

	lines := make([]string, 0, f.height)
	lines = append(lines, f.topRule())
	border := ruleStyle.Render("│")
	for _, line := range fitLines(body, h) {
		lines = append(lines, border+fitWidth(line, w)+border)
	}
	lines = append(lines, f.bottomRule())
	return strings.Join(lines, "\n")
}

// renderTiny 终端过小时的降级渲染：只保证不越界，并提示放大窗口
func (f frame) renderTiny(body []string) string {
	notice := errStyle.Render(fmt.Sprintf("终端过小（%d×%d）", f.width, f.height)) +
		dimStyle.Render(fmt.Sprintf("  需 ≥%d×%d", frameMinWidth, frameMinHeight))
	lines := append([]string{notice}, body...)
	h := f.height
	if h < 1 {
		h = 1
	}
	out := fitLines(lines, h)
	// 降级路径同样必须裁剪到终端宽度：过小判定不是放弃边界的理由
	for i, l := range out {
		out[i] = truncTo(l, f.renderWidth())
	}
	return strings.Join(out, "\n")
}

// topRule 标题栏：╭─ ✦ 品牌 ───────────── 屏幕名 · 上下文 ─ 运行指示 ─╮
func (f frame) topRule() string {
	left := titleStyle.Render("✦ " + f.brand)
	right := selectStyle.Render(f.title)
	if f.context != "" {
		right += dimStyle.Render(" · ") + valueStyle.Render(f.context)
	}
	if f.extra != "" {
		right += dimStyle.Render("  ") + f.extra
	}
	return composeRule("╭", "╮", left, right, "─", f.renderWidth())
}

// bottomRule 键位栏：╰─ 键位提示 ───────────────── 位置/进度 ─╯
func (f frame) bottomRule() string {
	return composeRule("╰", "╯", keyHints(f.keys), f.status, "─", f.renderWidth())
}

// composeRule 组装一条水平框线：两角之间以 fill 填充，左右文本内嵌。
//
// 空间不足时的取舍顺序是刻意的：右侧承载位置/进度/错误等「状态信息」，
// 左侧是「键位提示」——状态信息不可丢，键位可压缩。
// 成品恒为 width 列：框线一旦错位，整个界面就会「歪」，
// 因此这里的宽度收敛优先于信息完整。
func composeRule(cornerL, cornerR, left, right, fill string, width int) string {
	if width < 2 {
		return truncTo(left, width)
	}
	inner := width - 2

	// 零宽文本（含「只有样式、没有内容」的渲染结果）视作空，避免框线出现空档
	if dispWidth(left) == 0 {
		left = ""
	}
	if dispWidth(right) == 0 {
		right = ""
	}

	lseg, rseg := "", ""
	if left != "" {
		lseg = "─ " + left + " "
	}
	if right != "" {
		rseg = " " + right + " ─"
	}

	lw, rw := dispWidth(lseg), dispWidth(rseg)
	if lw+rw > inner {
		// 右侧信息先限定在一半宽度内（避免状态把键位整体挤没）
		maxRight := maxInt(8, inner/2)
		if rw > maxRight {
			rseg = " " + truncTo(right, maxInt(4, maxRight-2)) + " "
			rw = dispWidth(rseg)
		}
		// 左侧键位让位
		if keep := inner - rw - 1; keep >= 4 {
			lseg = truncTo(lseg, keep)
			lw = dispWidth(lseg)
		} else {
			lseg, lw = "", 0
		}
		// 兜底：仍放不下则只保留右侧
		if lw+rw > inner {
			rseg, lw, lseg = truncTo(rseg, inner), 0, ""
			rw = dispWidth(rseg)
		}
	}

	gap := inner - lw - rw
	if gap < 0 {
		gap = 0
	}
	return ruleStyle.Render(cornerL) + lseg +
		ruleStyle.Render(strings.Repeat(fill, gap)) + rseg +
		ruleStyle.Render(cornerR)
}

// frameFor 构建当前屏的框体骨架（标题栏右侧统一承载屏幕名、上下文与运行指示）
func (m appModel) frameFor(title, context string, keys []keyHint) frame {
	return frame{
		width:   m.width,
		height:  m.height,
		brand:   "星月智能 · 控制台",
		title:   title,
		context: context,
		extra:   m.headerStatus(),
		keys:    keys,
	}
}

// headerStatus 标题栏右侧的运行指示：进行中的任务优先，
// 其次是刚结束任务的结果，都没有时展示运行时长与时钟。
func (m appModel) headerStatus() string {
	switch {
	case m.busy != "":
		return selectStyle.Render(m.spinner.View() + " " + m.busy)
	case m.notice != "" && time.Since(m.noticeAt) < noticeTTL:
		if m.noticeErr {
			return errStyle.Render("✗ " + m.notice)
		}
		return okStyle.Render("✓ " + m.notice)
	default:
		return dimStyle.Render(formatDuration(time.Since(startTime)) + " · " + time.Now().Format("15:04:05"))
	}
}

// setBusy 标记后台任务开始（标题栏显示 spinner 与任务名）
func (m *appModel) setBusy(label string) {
	m.busy = label
	m.notice = ""
}

// setNotice 标记后台任务结束（标题栏短暂显示结果，随后自动隐去）
func (m *appModel) setNotice(label string, isErr bool) {
	m.busy = ""
	m.notice = label
	m.noticeErr = isErr
	m.noticeAt = time.Now()
}

// ==== 帮助页 ====

// helpRow 帮助页的一行键位说明：键位定宽 16 列，说明随后
func helpRow(w int, h keyHint) string {
	return newRow(w, false).
		L("  ", keyStyle.Render(fitWidth(h.key, 16)), "  ", labelStyle.Render(h.label)).
		String()
}

// viewHelp 键位帮助页：全局键位 + 当前屏键位（与底栏同源，不另维护一份）
func (m appModel) viewHelp() string {
	name, ctx, keys := m.screenMeta()
	f := m.frameFor("键位帮助", name, []keyHint{kh("?", "关闭"), kh("Esc", "关闭")})
	w, _ := f.bodySize()

	sub := name
	if ctx != "" {
		sub += " · " + ctx
	}
	body := []string{
		sectionRule("全局键位", w),
		"",
	}
	for _, k := range globalKeys {
		body = append(body, helpRow(w, k))
	}
	body = append(body, "", sectionRule("当前屏 · "+sub, w), "")
	if len(keys) == 0 {
		body = append(body, dimStyle.Render("  本屏无需额外键位"))
	}
	for _, k := range keys {
		body = append(body, helpRow(w, k))
	}
	body = append(body, "", dimStyle.Render("  按 ? 或 Esc 返回"))

	f.status = dimStyle.Render("帮助")
	return f.render(body)
}
