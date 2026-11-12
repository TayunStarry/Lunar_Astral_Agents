package main

// ==== 框体骨架 ====
//
// 与主控制台同源：标题栏（╭─ ✦ 品牌 ─ 分组 › 屏名 · 上下文 ─ 运行指示 ─╮）、
// 定高内容区、键位栏（╰─ 键位 ──────────── 状态 ─╯）。内容区严格裁剪/补齐，
// 任何屏幕都不会把内容顶出终端。

import (
	"fmt"
	"strings"
	"time"
)

// ==== 框体与导航 ====

// frameFor 构建当前屏框体：标题栏分组、底栏键位、宽屏侧栏与主控制台同构
func (m *uiModel) frameFor(title, context string, keys []uiKeyHint) uiFrame {
	f := uiFrame{
		width:   m.width,
		height:  m.height,
		brand:   uiBrand,
		crumbs:  screenGroups[m.screen],
		title:   title,
		context: context,
		extra:   m.headerStatus(),
		keys:    m.footerKeys(keys),
	}
	if m.navVisible() {
		f.navW = navWidth()
		f.navFn = m.navLines
	}
	return f
}

// footerKeys 底栏键位：把「? 帮助」放在最前，窄终端截断时帮助入口仍可见
func (m *uiModel) footerKeys(keys []uiKeyHint) []uiKeyHint {
	for _, k := range keys {
		if k.Key == "?" {
			return keys
		}
	}
	out := make([]uiKeyHint, 0, len(keys)+1)
	out = append(out, uiKeyHint{Key: "?", Label: "帮助"})
	return append(out, keys...)
}

// navVisible 侧栏显示条件：与主控制台一致——帮助页与总览（菜单等价屏）不显示，
// 终端过窄或过矮时收起，避免把内容区挤到不可用。
func (m *uiModel) navVisible() bool {
	if m.showHelp || m.screen == screenOverview {
		return false
	}
	if m.width < uiNavShowFrom || m.height < uiNavMinHeight {
		return false
	}
	return true
}

// navWidth 侧栏宽度：按最长屏名与快捷键标记计算，收敛在 [uiNavWidthMin, uiNavWidthMax]
func navWidth() int {
	labelW, keyW := 0, 0
	for i := 0; i < screenCount; i++ {
		labelW = maxInt(labelW, dispWidth(screenNames[i]))
		keyW = maxInt(keyW, dispWidth("["+screenKeys[i]+"]"))
	}
	return clampInt(2+labelW+2+keyW+2, uiNavWidthMin, uiNavWidthMax)
}

// navRows 侧栏行表：组首插入分组标题（顺序 = 屏顺序）
func navRows() []uiNavRow {
	rows := make([]uiNavRow, 0, screenCount+3)
	for i := 0; i < screenCount; i++ {
		if i == 0 || screenGroups[i-1] != screenGroups[i] {
			rows = append(rows, uiNavRow{idx: -1, group: screenGroups[i]})
		}
		rows = append(rows, uiNavRow{idx: i})
	}
	return rows
}

// navLines 渲染恰好 h 行的侧栏内容（五个屏 + 分组标题，无需滚动）
func (m *uiModel) navLines(w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	out := []string{newRow(w, false).
		L(" ", faintStyle.Render(truncTo("导航", maxInt(2, w-2)))).
		String()}
	for _, row := range navRows() {
		if len(out) >= h {
			break
		}
		out = append(out, m.navRowLine(w, row))
	}
	return fitLines(out, h)
}

// navRowLine 单行侧栏：分组标题 / 普通条目 / 当前屏（整行反白）
func (m *uiModel) navRowLine(w int, row uiNavRow) string {
	if row.idx < 0 {
		return newRow(w, false).
			L(" ", navGroupStyle.Render(truncTo(row.group, maxInt(4, w-2)))).
			String()
	}
	active := row.idx == m.screen
	cursor := "  "
	label := navItemStyle.Render(screenNames[row.idx])
	key := navKeyStyle.Render("[" + screenKeys[row.idx] + "]")
	if active {
		cursor = cursorStyle.Render("▸ ")
		label = screenNames[row.idx]
		key = selDimStyle.Render("[" + screenKeys[row.idx] + "]")
	}
	return newRow(w, active).L(cursor, label).R(key).String()
}

// headerStatus 标题栏右侧运行指示：进行中的任务优先，其次结果提示，最后时长与时钟
func (m *uiModel) headerStatus() string {
	if m.installStageBusy() {
		return selectStyle.Render("● " + installStageLabel(m.install.Stage))
	}
	if m.publish.Busy {
		return selectStyle.Render("● 打包上传中")
	}
	if m.notice != "" && time.Since(m.noticeAt) < uiNoticeTTL {
		if m.noticeErr {
			return errStyle.Render("✗ " + m.notice)
		}
		return okStyle.Render("✓ " + m.notice)
	}
	return dimStyle.Render(time.Since(m.startAt).Round(time.Second).String() + " · " + time.Now().Format("15:04"))
}

// renderWidth 实际排版列数：让出最后一列，规避 conhost 末列换行吞字
func (f uiFrame) renderWidth() int {
	return maxInt(1, f.width-1)
}

// navBlockWidth 侧栏占用列数（含分隔竖线）
func (f uiFrame) navBlockWidth() int {
	if f.navW <= 0 {
		return 0
	}
	return f.navW + 1
}

// bodySize 内容区尺寸
func (f uiFrame) bodySize() (int, int) {
	w := f.renderWidth() - 2 - f.navBlockWidth()
	h := f.height - 2
	return maxInt(1, w), maxInt(0, h)
}

// tooSmall 终端不足以承载框体
func (f uiFrame) tooSmall() bool {
	return f.width < uiFrameMinWidth || f.height < uiFrameMinHeight
}

// render 渲染整屏
func (f uiFrame) render(body []string) string {
	w, h := f.bodySize()
	if f.tooSmall() {
		lines := append([]string{errStyle.Render(fmt.Sprintf("终端过小（%d×%d）", f.width, f.height)) +
			dimStyle.Render(fmt.Sprintf("  需 ≥%d×%d", uiFrameMinWidth, uiFrameMinHeight))}, body...)
		out := fitLines(lines, maxInt(1, f.height))
		for i, l := range out {
			out[i] = truncTo(l, f.renderWidth())
		}
		return strings.Join(out, "\n")
	}

	var nav []string
	if f.navW > 0 && f.navFn != nil {
		nav = f.navFn(f.navW, maxInt(1, h))
	}
	border := ruleStyle.Render("│")

	lines := make([]string, 0, f.height)
	lines = append(lines, f.topRule())
	for i, line := range fitLines(body, h) {
		if f.navW > 0 {
			var navLine string
			if i < len(nav) {
				navLine = nav[i]
			}
			lines = append(lines, border+fitWidth(navLine, f.navW)+border+fitWidth(line, w)+border)
			continue
		}
		lines = append(lines, border+fitWidth(line, w)+border)
	}
	lines = append(lines, f.bottomRule())
	return strings.Join(lines, "\n")
}

// topRule 标题栏
func (f uiFrame) topRule() string {
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

// bottomRule 键位栏
func (f uiFrame) bottomRule() string {
	return composeRule("╰", "╯", keyHints(f.keys), f.status, "─", f.renderWidth())
}

// composeRule 组装一条水平框线；空间不足时优先保留右侧状态信息
func composeRule(cornerL, cornerR, left, right, fill string, width int) string {
	if width < 2 {
		return truncTo(left, width)
	}
	inner := width - 2
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

	const minGap = 3
	lw, rw := dispWidth(lseg), dispWidth(rseg)
	if lw+rw+minGap > inner {
		maxRight := maxInt(8, inner/2)
		if rw > maxRight {
			rseg = " " + truncTo(right, maxInt(4, maxRight-2)) + " "
			rw = dispWidth(rseg)
		}
		if lw+rw+minGap > inner {
			if keep := inner - rw - minGap; keep >= 4 {
				lseg = truncTo(lseg, keep)
				lw = dispWidth(lseg)
			} else {
				lseg, lw = "", 0
			}
		}
		if lw+rw > inner {
			rseg, lw, lseg = truncTo(rseg, inner), 0, ""
			rw = dispWidth(rseg)
		}
	}

	gap := maxInt(0, inner-lw-rw)
	return ruleStyle.Render(cornerL) + lseg +
		ruleStyle.Render(strings.Repeat(fill, gap)) + rseg +
		ruleStyle.Render(cornerR)
}
