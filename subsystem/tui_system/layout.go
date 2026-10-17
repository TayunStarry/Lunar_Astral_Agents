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

// globalKeys 全屏通用键位。
//
// 翻页/首尾这类「处处可用」的键放在这里而不是各屏底栏：底栏宽度有限（窄终端要与
// 右侧状态争宽），全局键写进帮助页才既完整又不挤掉当前屏真正的操作提示。
var globalKeys = []keyHint{
	kh("↑/↓ k/j", "移动选择"),
	kh("PgUp/PgDn", "翻页（日志 / 列表）"),
	kh("Home/End g/G", "跳到首 / 尾"),
	kh("Enter", "确认 / 进入"),
	kh("Esc", "返回主菜单"),
	kh("[ / ]", "按侧栏顺序切上一屏 / 下一屏"),
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
//
// 宽屏时内容区左侧再嵌一列侧边导航（navW>0）：它只吃内容区的宽度，
// 因此各屏仍按 bodySize 排版即可自动适配，无需各自处理侧栏。
type frame struct {
	width, height int

	brand   string // 标题栏左：品牌
	crumbs  string // 标题栏右：面包屑（所属分组）
	title   string // 标题栏右：当前屏名
	context string // 标题栏右：上下文（仓库 / 筛选 / 模式）
	extra   string // 标题栏右：运行指示（任务 / 时长 / 时钟）

	keys   []keyHint
	status string // 键位栏右：位置 / 进度 / 状态

	navW  int                              // 侧栏列数（0 = 不显示）
	navFn func(width, height int) []string // 侧栏内容按需生成（仅整屏渲染时调用一次）
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

// navBlockWidth 侧栏实际占用的列数（含其右侧的分隔竖线）；未显示时为 0
func (f frame) navBlockWidth() int {
	if f.navW <= 0 {
		return 0
	}
	return f.navW + 1
}

// bodySize 内容区尺寸：框线占左右各 1 列、上下各 1 行；宽屏时再让出侧栏
func (f frame) bodySize() (int, int) {
	w, h := f.renderWidth()-2-f.navBlockWidth(), f.height-2
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

	// 侧栏只在整屏渲染时生成一次（syncViewMetrics 之类的热路径只取 bodySize，不付这份成本）
	var nav []string
	if f.navW > 0 && f.navFn != nil {
		nav = f.navFn(f.navW, maxInt(1, f.height-2))
	}
	border := ruleStyle.Render("│")
	sep := ruleStyle.Render("│")

	lines := make([]string, 0, f.height)
	lines = append(lines, f.topRule())
	for i, line := range fitLines(body, h) {
		if f.navW > 0 {
			var navLine string
			if i < len(nav) {
				navLine = nav[i]
			}
			lines = append(lines, border+fitWidth(navLine, f.navW)+sep+fitWidth(line, w)+border)
			continue
		}
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

// topRule 标题栏：╭─ ✦ 品牌 ───── 分组 › 屏幕名 · 上下文 ─ 运行指示 ─╮
func (f frame) topRule() string {
	left := titleStyle.Render("✦ " + f.brand)
	right := ""
	if f.crumbs != "" {
		right = crumbStyle.Render(f.crumbs) + dimStyle.Render(" › ")
	}
	right += selectStyle.Render(f.title)
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

	// minGap 两侧都有内容时至少保留的填充宽度。
	// 不能是 0：两侧文字顶在一起时框线会「断开」，读起来像两行拼错了（实测观感问题）。
	const minGap = 3
	lw, rw := dispWidth(lseg), dispWidth(rseg)
	if lw+rw+minGap > inner {
		// 右侧信息先限定在一半宽度内（避免状态把键位整体挤没）
		maxRight := maxInt(8, inner/2)
		if rw > maxRight {
			rseg = " " + truncTo(right, maxInt(4, maxRight-2)) + " "
			rw = dispWidth(rseg)
		}
		// 左侧键位让位
		if lw+rw+minGap > inner {
			if keep := inner - rw - minGap; keep >= 4 {
				lseg = truncTo(lseg, keep)
				lw = dispWidth(lseg)
			} else {
				lseg, lw = "", 0
			}
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

// frameFor 构建当前屏的框体骨架（标题栏右侧统一承载面包屑、屏幕名、上下文与运行指示）
func (m appModel) frameFor(title, context string, keys []keyHint) frame {
	f := frame{
		width:   m.width,
		height:  m.height,
		brand:   "星月智能 · 控制台",
		crumbs:  m.screenGroup(),
		title:   title,
		context: context,
		extra:   m.headerStatus(),
		keys:    m.footerKeys(keys),
	}
	if m.navVisible() {
		w := navWidth()
		f.navW = w
		f.navFn = func(width, height int) []string { return m.navLines(width, height) }
	}
	return f
}

// footerKeys 底栏键位：补上全局的「? 帮助」，让任何屏都能就地发现帮助页。
// 各屏自报的键位在前（更贴近当前操作），全局键位在后。
func (m appModel) footerKeys(keys []keyHint) []keyHint {
	for _, k := range keys {
		if k.key == "?" {
			return keys
		}
	}
	out := make([]keyHint, 0, len(keys)+1)
	out = append(out, keys...)
	return append(out, kh("?", "帮助"))
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

// helpRow 帮助页的一行键位说明：键位定宽 + 说明随后
func helpRow(keyW, w int, h keyHint) string {
	return newRow(w, false).
		L("  ", keyStyle.Render(fitWidth(h.key, keyW)), "  ", labelStyle.Render(h.label)).
		String()
}

// helpColumn 帮助页的一栏：区块标题 + 若干键位行；h<=0 表示不限制行数
func helpColumn(section string, list []keyHint, w, h int) []string {
	if w <= 0 {
		return nil
	}
	keyW := clampInt(w/3, 8, 16)
	out := []string{sectionRule(section, w), ""}
	if len(list) == 0 {
		out = append(out, "  "+faintStyle.Render("（本屏无额外键位）"))
	}
	for _, k := range list {
		if k.key == "" {
			continue
		}
		out = append(out, helpRow(keyW, w, k))
	}
	if h <= 0 {
		return out
	}
	return fitLines(out, h)
}

// viewHelp 键位帮助页：宽屏双栏（全局键位 / 当前屏键位 + 导航提示），窄屏上下排列。
// 键位与底栏同源（screenMeta），不另维护一份，避免「文档写的键」与「实际生效的键」失配。
func (m appModel) viewHelp() string {
	name, ctx, keys := m.screenMeta()
	f := m.frameFor("键位帮助", name, []keyHint{kh("?/Esc", "关闭")})
	w, h := f.bodySize()

	sub := name
	if ctx != "" {
		sub += " · " + ctx
	}

	// 全局栏尾部补一条「侧栏是什么」的说明：它是新界面里唯一的非常规元件
	global := append([]keyHint{}, globalKeys...)
	global = append(global, kh("侧栏", "宽屏时常驻，标出当前所在屏"))

	var body []string
	if w >= 76 {
		leftW := (w - 3) / 2
		rightW := w - leftW - 3
		left := helpColumn("全局键位", global, leftW, h)
		right := helpColumn("当前屏 · "+sub, keys, rightW, h)
		sep := ruleStyle.Render(" │ ")
		body = make([]string, 0, h)
		for i := 0; i < h; i++ {
			l, r := "", ""
			if i < len(left) {
				l = left[i]
			}
			if i < len(right) {
				r = right[i]
			}
			body = append(body, fitWidth(l, leftW)+sep+fitWidth(r, rightW))
		}
	} else {
		body = append(body, helpColumn("全局键位", global, w, 0)...)
		body = append(body, "")
		body = append(body, helpColumn("当前屏 · "+sub, keys, w, 0)...)
		body = append(body, "", "  "+dimStyle.Render("按 ? 或 Esc 返回"))
	}

	f.status = dimStyle.Render("帮助")
	return f.render(body)
}
