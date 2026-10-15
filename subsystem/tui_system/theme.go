package TuiSystem

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// ==== 调色板 ====
//
// 统一色阶：品牌紫承担标题与选中态，青绿承担数值与成功，橙/红承担警告与错误，
// 灰阶承担结构线（边框、分隔）与次要信息。各屏共用同一色板，
// 避免「每个屏各自取色」造成的观感漂移。

var (
	colAccent     = lipgloss.Color("213") // 品牌紫：标题、选中光标
	colAccentSoft = lipgloss.Color("219") // 浅紫：键位提示
	colText       = lipgloss.Color("252") // 正文
	colValue      = lipgloss.Color("86")  // 数值/强调（青）
	colDim        = lipgloss.Color("245") // 次要信息
	colFaint      = lipgloss.Color("240") // 更次要（占位、空态）
	colRule       = lipgloss.Color("238") // 结构线
	colOK         = lipgloss.Color("82")
	colWarn       = lipgloss.Color("214")
	colErr        = lipgloss.Color("204")
	colSelBg      = lipgloss.Color("237") // 选中行底色
	colSelFg      = lipgloss.Color("231") // 选中行前景
)

// ==== 基础样式 ====

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	labelStyle   = lipgloss.NewStyle().Foreground(colText)
	valueStyle   = lipgloss.NewStyle().Bold(true).Foreground(colValue)
	dimStyle     = lipgloss.NewStyle().Foreground(colDim)
	faintStyle   = lipgloss.NewStyle().Foreground(colFaint)
	selectStyle  = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	okStyle      = lipgloss.NewStyle().Bold(true).Foreground(colOK)
	warnStyle    = lipgloss.NewStyle().Foreground(colWarn)
	errStyle     = lipgloss.NewStyle().Bold(true).Foreground(colErr)
	cursorStyle  = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	ruleStyle    = lipgloss.NewStyle().Foreground(colRule)
	keyStyle     = lipgloss.NewStyle().Bold(true).Foreground(colAccentSoft)
	hintStyle    = lipgloss.NewStyle().Foreground(colDim)
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(colValue)
	selRowStyle  = lipgloss.NewStyle().Background(colSelBg).Foreground(colSelFg)

	// 选中行内的次级文字：底色之上仍保持可读的灰
	selDimStyle = lipgloss.NewStyle().Background(colSelBg).Foreground(lipgloss.Color("250"))
)

// ==== 日志专用样式 ====

var (
	logInfoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))  // 青色：常规
	logDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245")) // 灰色：stdout 原始行
	logProxyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))  // 蓝色：任务回显
	logWarnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // 橙色：警告
	logErrStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("204")) // 红色：错误
	thinkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("243")) // 思考流（暗色）
	roleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("176"))
)

// renderLogLine 单条日志渲染：按级别/来源着色
func renderLogLine(line LogLine) string {
	switch {
	case line.Level == "ERROR" || line.Level == "FATAL":
		return logErrStyle.Render(line.Text)
	case line.Level == "WARN":
		return logWarnStyle.Render(line.Text)
	case line.Level == "RAW":
		return logDimStyle.Render(line.Text)
	case line.Source == "task":
		return logProxyStyle.Render(line.Text)
	default:
		return logInfoStyle.Render(line.Text)
	}
}

// ==== 显示宽度与拼装原语 ====
//
// 终端排版必须按「显示列宽」而非字节或 rune 数计算：中文与 emoji 占 2 列，
// ANSI 转义序列占 0 列。以下原语统一走 x/ansi（grapheme 与宽字符感知），
// 是「中文标签对不齐、长行把框线挤破」这类排版问题的根治手段。

// dispWidth 字符串显示列宽（忽略 ANSI 转义序列）。
//
// 按**终端实测**宽度计算：中文控制台字体（新宋体等）会把 ◆ ■ ▶ · → 这类
// 「东亚歧义宽度」字形渲染成 2 列，若仍按内置宽度表（1 列）排版，整行就会超出
// 终端宽度而折行——用户实测的「一打开就滚动 + 显示混乱」正是这个成因。
// 未校准时（非 Windows 或取不到控制台）退化为内置宽度表，行为与以前一致。
func dispWidth(s string) int {
	base := ansi.StringWidth(s)
	if base == 0 || !hasWideGlyphs(s) {
		return base
	}
	return base + extraWidthOf(s)
}

// truncTo 裁剪到 w 列（超出部分以 … 结尾）；w<=0 返回空串
func truncTo(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if dispWidth(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	if hasWideGlyphs(s) {
		// 含校准字形时不能交给 ansi.Truncate：它按内置宽度表算列数，会截多或截少
		return consoleTruncate(s, w)
	}
	return ansi.Truncate(s, w, "…")
}

// fitWidth 把一行补齐或裁剪到恰好 w 列
func fitWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	d := dispWidth(s)
	switch {
	case d == w:
		return s
	case d > w:
		return truncTo(s, w)
	default:
		return s + strings.Repeat(" ", w-d)
	}
}

// paint 按选中态着色（未选中时原样返回，保留片段自身样式）
func paint(s string, selected bool) string {
	if selected {
		return selRowStyle.Render(s)
	}
	return s
}

// rowBuilder 定宽行组装器：左侧片段靠左、右侧片段靠右，中间自动填充，
// 结果恰好 width 列。selected 时底色分别包裹每个片段——若把整行一次性
// 渲染底色，片段内层样式的复位序列会在中途清掉底色，是整行高亮「半截变色」
// 的常见成因；逐片段包裹可彻底规避。
type rowBuilder struct {
	width    int
	selected bool
	left     []string
	right    []string
	sep      int // 左右之间的最小间隔列数
}

// newRow 创建定宽行组装器
func newRow(width int, selected bool) *rowBuilder {
	return &rowBuilder{width: width, selected: selected, sep: 2}
}

// L 追加左侧（靠左）片段
func (r *rowBuilder) L(segs ...string) *rowBuilder {
	r.left = append(r.left, segs...)
	return r
}

// R 追加右侧（靠右）片段
func (r *rowBuilder) R(segs ...string) *rowBuilder {
	r.right = append(r.right, segs...)
	return r
}

// String 组装为恰好 width 列的一行
func (r *rowBuilder) String() string {
	if r.width <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	write := func(s string) {
		if s == "" {
			return
		}
		b.WriteString(paint(s, r.selected))
		used += dispWidth(s)
	}

	rightSum := 0
	for _, s := range r.right {
		rightSum += dispWidth(s)
	}
	leftBudget := r.width
	if len(r.right) > 0 {
		leftBudget = r.width - rightSum - r.sep
		if leftBudget < 1 { // 右侧过长：整体让位给左侧主体
			r.right, rightSum, leftBudget = nil, 0, r.width
		}
	}

	// 左侧：末片段吃掉剩余预算
	leftWrote := 0
	for i, s := range r.left {
		avail := leftBudget - leftWrote
		if avail <= 0 {
			break
		}
		if i == len(r.left)-1 {
			s = fitWidth(s, avail)
		} else {
			s = truncTo(s, avail)
		}
		write(s)
		leftWrote += dispWidth(s)
	}
	if leftWrote < leftBudget && rightSum == 0 {
		write(strings.Repeat(" ", leftBudget-leftWrote))
		leftWrote = leftBudget
	}

	// 右侧：中间填充后依次写入，越界即截断
	if rightSum > 0 {
		gap := r.width - leftWrote - rightSum
		if gap < 1 {
			gap = 1
		}
		write(strings.Repeat(" ", gap))
		for _, s := range r.right {
			remain := r.width - used
			if remain <= 0 {
				break
			}
			if dispWidth(s) > remain {
				s = truncTo(s, remain)
			}
			write(s)
		}
	}

	// 兜底补齐（正常路径下恒为 0）
	if used < r.width {
		write(strings.Repeat(" ", r.width-used))
	}
	return b.String()
}

// listRow 单行列表项：光标 + 主体 + 右侧点缀（空串则省略），铺满 width 列
func listRow(width int, selected bool, cursor, primary, right string) string {
	r := newRow(width, selected).L(cursor, primary)
	if right != "" {
		r.R(right)
	}
	return r.String()
}

// wrapPlain 按显示宽度硬折行（保留原有换行；返回至少一行）
func wrapPlain(s string, w int) []string {
	if w <= 1 {
		w = 2
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "")
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		wrapped := ansi.Hardwrap(para, w, false)
		out = append(out, strings.Split(wrapped, "\n")...)
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// wrapText 折行并可限定最多 maxLines 行（超出时保留尾部并加省略号前缀）
func wrapText(s string, w, maxLines int) []string {
	lines := wrapPlain(s, w)
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
		lines[0] = "…" + lines[0]
	}
	return lines
}

// clipLines 只保留前 h 行
func clipLines(lines []string, h int) []string {
	if h <= 0 {
		return nil
	}
	if len(lines) <= h {
		return lines
	}
	return lines[:h]
}

// padLines 补齐到恰好 h 行（空行补位）
func padLines(lines []string, h int) []string {
	if h <= 0 {
		return nil
	}
	out := make([]string, 0, h)
	out = append(out, lines...)
	for len(out) < h {
		out = append(out, "")
	}
	return out[:h]
}

// fitLines 裁剪或补齐到恰好 h 行
func fitLines(lines []string, h int) []string {
	return padLines(clipLines(lines, h), h)
}

// ==== 组合组件 ====

// sectionRule 区块分隔标题：── 标题 ────────────（宽度铺满 w 列）
func sectionRule(title string, w int) string {
	if w <= 0 {
		return ""
	}
	head := "── " + title + " "
	hw := dispWidth(head)
	if hw >= w {
		return sectionStyle.Render(truncTo(title, w))
	}
	return sectionStyle.Render(head) + ruleStyle.Render(strings.Repeat("─", w-hw))
}

// keyHints 渲染键位提示串：`↑/↓ 选择 · Enter 进入 · Esc 返回`
func keyHints(hints []keyHint) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		if h.key == "" {
			continue
		}
		parts = append(parts, keyStyle.Render(h.key)+" "+hintStyle.Render(h.label))
	}
	return strings.Join(parts, hintStyle.Render(" · "))
}

// labelValue 渲染「标签 + 数值」定宽行（标签按显示宽度补齐，中文不再错位）
func labelValue(label, value string, labelW int) string {
	return dimStyle.Render(fitWidth(label, labelW)) + valueStyle.Render(value)
}

// kvRow 定宽键值行（键按显示宽度对齐）
func kvRow(key, value string, keyW int) string {
	return dimStyle.Render(fitWidth(key, keyW)) + labelStyle.Render(value)
}

// progressBar 渲染进度条：宽度自适应，百分比右对齐
func progressBar(percent int, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	barW := width - 8
	if barW < 8 {
		barW = 8
	}
	if barW > 60 {
		barW = 60
	}
	filled := percent * barW / 100
	filledStr := strings.Repeat("█", filled)
	emptyStr := strings.Repeat("░", barW-filled)
	bar := valueStyle.Render(filledStr) + faintStyle.Render(emptyStr)
	return bar + " " + valueStyle.Render(fmt.Sprintf("%3d%%", percent))
}

// stepsBar 向导步骤条：当前步高亮、已完成步转绿、未达步灰显，
// 让多步流程随时可见「我在哪一步、还剩几步」。
func stepsBar(steps []string, active, w int) string {
	marks := []string{"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"}
	r := newRow(w, false)
	for i, s := range steps {
		m := "○"
		if i < len(marks) {
			m = marks[i]
		}
		label := m + " " + s
		if i > 0 {
			r.L(dimStyle.Render("  ──  "))
		}
		switch {
		case i == active:
			r.L(selectStyle.Render(label))
		case i < active:
			r.L(okStyle.Render(label))
		default:
			r.L(faintStyle.Render(label))
		}
	}
	return r.String()
}

// withErrStrip 在内容顶部插入错误提示条（错误永不被列表挤到屏幕之外）
func withErrStrip(body []string, err error, w int) []string {
	if err == nil {
		return body
	}
	strip := errStyle.Render("✗ "+truncTo(err.Error(), maxInt(8, w-4))) + dimStyle.Render("   （详细过程见「原始日志」）")
	return append([]string{strip, ""}, body...)
}

// humanSize 字节数人性化（B / KB / MB / GB，保留 1 位小数）
func humanSize(n int64) string {
	switch {
	case n <= 0:
		return "-"
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.2f GB", float64(n)/(1024*1024*1024))
	}
}

// posText 列表位置文本：「3/17 个包」（总数 0 时返回空串）
func posText(cur, n int, unit string) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d %s", clampInt(cur, 0, n-1)+1, n, unit)
}

// ==== 杂项 ====

// formatDuration 时长格式化（秒级）
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	sec := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, sec)
	}
	return fmt.Sprintf("%dm%02ds", m, sec)
}

// firstLine 取文本首行
func firstLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// clampInt 数值收敛到 [lo, hi]
func clampInt(v, lo, hi int) int {
	if lo > hi {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// maxInt 取较大值
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// minInt 取较小值
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
