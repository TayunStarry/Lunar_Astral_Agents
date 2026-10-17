package TuiSystem

// ==== 组件与排版原语 ====
//
// 这里的每个函数都只做「按显示宽度拼装一行/一块文本」，不做业务判断；
// 屏幕上看到的一切框线、定宽行、进度条、空态都由它们生成，
// 因此宽度不变式（每行恒为 N 列）只需在这一层守住。

import (
	"fmt"
	"strings"
)

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

// sectionRule 区块分隔标题：── 标题 ────────────（宽度铺满 w 列）
// 标题为空时只画一条分隔线——空标题若照旧加「── 」前缀，会留下「──  ────」的空档。
func sectionRule(title string, w int) string {
	if w <= 0 {
		return ""
	}
	if strings.TrimSpace(title) == "" {
		return ruleStyle.Render(strings.Repeat("─", w))
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

// emptyState 空态占位：标题 + 一句说明 + 下一步提示。
//
// 为什么要统一成组件：空列表是全应用最容易「什么都不显示」的地方，
// 用户看到一片空白只能猜。各屏此前各写各的灰色一行字，措辞与位置都不一致。
// 收成一个组件后，任何屏的「没有数据」都明确回答三件事：这是什么、为什么空、接下来按什么。
func emptyState(title, hint, next string, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	inner := maxInt(8, w-4)
	lines := []string{
		"",
		"  " + selectStyle.Render("✦ "+truncTo(title, inner)),
	}
	if hint != "" {
		lines = append(lines, "")
		for _, l := range wrapPlain(hint, inner) {
			lines = append(lines, "    "+dimStyle.Render(l))
		}
	}
	if next != "" {
		lines = append(lines, "")
		for _, l := range wrapPlain(next, inner) {
			lines = append(lines, "    "+faintStyle.Render(l))
		}
	}
	// 垂直方向放在偏上三分之一处：既不像居中那样随窗口高度乱跳，也不至于贴顶
	top := clampInt(h/4, 0, maxInt(0, h-len(lines)))
	out := make([]string, 0, h)
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	out = append(out, lines...)
	return fitLines(out, h)
}
