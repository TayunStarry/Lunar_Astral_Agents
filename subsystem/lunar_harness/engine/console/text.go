package console

// ==== 文本度量与折行原语 ====
//
// 终端排版必须按「显示列宽」而非字节或 rune 数计算：中文与 emoji 占 2 列，
// ANSI 转义序列占 0 列。以下原语统一走 x/ansi（grapheme 与宽字符感知），
// 是「中文标签对不齐、长行把框线挤破」这类排版问题的根治手段。
//
// 本文件只放「与界面无关的纯文本运算」，样式与组件见 theme.go / widgets.go。

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

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
		// 截断后必须再补齐：截断遇到宽字符边界会停在不足 w 列处（不会自己补空格）。
		// 少了这步，该行右框线就比其它行左移一格——用户实测的「未对齐」正是这种形态。
		cut := truncTo(s, w)
		if cd := dispWidth(cut); cd < w {
			cut += strings.Repeat(" ", w-cd)
		}
		return cut
	default:
		return s + strings.Repeat(" ", w-d)
	}
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

// ==== 数值/文本换算 ====

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

// ==== 数值收敛 ====

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
