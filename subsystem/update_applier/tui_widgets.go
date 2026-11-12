package main

// ==== 组件与排版原语 ====

import (
	"fmt"
	"strings"
)

// paint 按选中态着色
func paint(s string, selected bool) string {
	if selected {
		return selRowStyle.Render(s)
	}
	return s
}

// newRow 创建定宽行组装器
func newRow(width int, selected bool) *uiRowBuilder {
	return &uiRowBuilder{width: width, selected: selected, sep: 2}
}

// L 追加左侧片段
func (r *uiRowBuilder) L(segs ...string) *uiRowBuilder {
	r.left = append(r.left, segs...)
	return r
}

// R 追加右侧片段
func (r *uiRowBuilder) R(segs ...string) *uiRowBuilder {
	r.right = append(r.right, segs...)
	return r
}

// String 组装为恰好 width 列的一行
func (r *uiRowBuilder) String() string {
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
		if leftBudget < 1 {
			r.right, rightSum, leftBudget = nil, 0, r.width
		}
	}

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
	}

	if rightSum > 0 {
		gap := maxInt(1, r.width-leftWrote-rightSum)
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
	if used < r.width {
		write(strings.Repeat(" ", r.width-used))
	}
	return b.String()
}

// sectionRule 区块分隔标题
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

// keyHints 渲染键位提示串
func keyHints(hints []uiKeyHint) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		if h.Key == "" {
			continue
		}
		parts = append(parts, keyStyle.Render(h.Key)+" "+hintStyle.Render(h.Label))
	}
	return strings.Join(parts, hintStyle.Render(" · "))
}

// kvRow 定宽键值行
func kvRow(key, value string, keyW int) string {
	return dimStyle.Render(fitWidth(key, keyW)) + labelStyle.Render(value)
}

// helpRow 帮助页的一行键位说明：键位定宽 + 说明随后
func helpRow(keyW, w int, h uiKeyHint) string {
	return newRow(w, false).
		L("  ", keyStyle.Render(fitWidth(h.Key, keyW)), "  ", labelStyle.Render(h.Label)).
		String()
}

// helpColumn 帮助页的一栏：区块标题 + 若干键位行；h<=0 表示不限制行数
func helpColumn(section string, list []uiKeyHint, w, h int) []string {
	if w <= 0 {
		return nil
	}
	keyW := clampInt(w/3, 8, 16)
	out := []string{sectionRule(section, w), ""}
	if len(list) == 0 {
		out = append(out, "  "+faintStyle.Render("（本屏无额外键位）"))
	}
	for _, k := range list {
		if k.Key == "" {
			continue
		}
		out = append(out, helpRow(keyW, w, k))
	}
	if h <= 0 {
		return out
	}
	return fitLines(out, h)
}

// windowLineList 行窗口：确保 cursor 行完整可见，返回恰好 height 行
func windowLineList(lines []string, cursor, height int) []string {
	if height <= 0 {
		return nil
	}
	total := len(lines)
	if total <= height {
		return fitLines(lines, height)
	}
	cursor = clampInt(cursor, 0, total-1)
	top := cursor - height/2
	if top < 0 {
		top = 0
	}
	if cursor >= top+height {
		top = cursor - height + 1
	}
	top = clampInt(top, 0, total-height)
	return fitLines(lines[top:top+height], height)
}

// progressBar 进度条
func progressBar(percent, width int) string {
	percent = clampInt(percent, 0, 100)
	barW := clampInt(width-8, 8, 60)
	filled := percent * barW / 100
	bar := valueStyle.Render(strings.Repeat("█", filled)) + faintStyle.Render(strings.Repeat("░", barW-filled))
	return bar + " " + valueStyle.Render(fmt.Sprintf("%3d%%", percent))
}

// stepsBar 流水线步骤条
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

// emptyState 空态占位
func emptyState(title, hint, next string, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	inner := maxInt(8, w-4)
	lines := []string{"", "  " + selectStyle.Render("✦ "+truncTo(title, inner))}
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
	top := clampInt(h/4, 0, maxInt(0, h-len(lines)))
	out := make([]string, 0, h)
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	out = append(out, lines...)
	return fitLines(out, h)
}
