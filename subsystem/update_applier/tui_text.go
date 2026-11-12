package main

// ==== 文本度量与折行原语 ====
//
// 终端排版按「显示列宽」计算：中文与框线字形可能占 2 列，ANSI 转义占 0 列。
// 与主控制台同源：含手机控制台字体下「东亚歧义宽度」字形的实测校准（见 tui_glyph_*.go）。

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
)

// probeGlyphs 需要实测的字形（只登记界面实际会用到的装饰字符）
var probeGlyphs = []rune{
	'─', '│', '╭', '╮', '╰', '╯', '├', '┤',
	'█', '▓', '▒', '░',
	'◆', '◇', '●', '○', '■', '□', '▪', '▫', '▶', '▸', '◀', '◂',
	'✦', '✓', '✗',
	'·', '•', '…', '—', '–', '×', '≥', '≤', '→', '←', '↑', '↓',
	'①', '②', '③', '④', '⑤', '⑥', '⑦', '⑧', '⑨',
}

var (
	glyphMu    sync.RWMutex
	wideGlyphs = map[rune]int{} // 字形 → 比内置宽度表多占的列数
)

// setMeasuredGlyphWidth 记录实测宽度（只记差值）
func setMeasuredGlyphWidth(r rune, w int) {
	if w < 1 || w > 4 {
		return
	}
	glyphMu.Lock()
	defer glyphMu.Unlock()
	extra := w - ansi.StringWidth(string(r))
	if extra <= 0 {
		delete(wideGlyphs, r)
		return
	}
	wideGlyphs[r] = extra
}

// hasWideGlyphs 字符串是否含已校准的宽字形
func hasWideGlyphs(s string) bool {
	glyphMu.RLock()
	defer glyphMu.RUnlock()
	if len(wideGlyphs) == 0 {
		return false
	}
	for r := range wideGlyphs {
		if strings.ContainsRune(s, r) {
			return true
		}
	}
	return false
}

// wideGlyphExtra 字形比内置宽度表多占的列数
func wideGlyphExtra(r rune) int {
	glyphMu.RLock()
	defer glyphMu.RUnlock()
	return wideGlyphs[r]
}

// dispWidth 字符串显示列宽（忽略 ANSI）
func dispWidth(s string) int {
	base := ansi.StringWidth(s)
	if base == 0 || !hasWideGlyphs(s) {
		return base
	}
	extra := 0
	for _, r := range s {
		extra += wideGlyphExtra(r)
	}
	return base + extra
}

// truncTo 裁剪到 w 列（超出以 … 结尾）
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
		return truncWide(s, w)
	}
	return ansi.Truncate(s, w, "…")
}

// truncWide 按实测宽度截断
func truncWide(s string, w int) string {
	ellipsis := "…"
	budget := w - 1
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := ansi.StringWidth(string(r)) + wideGlyphExtra(r)
		if used+rw > budget {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	b.WriteString(ellipsis)
	return b.String()
}

// fitWidth 补齐或裁剪到恰好 w 列
func fitWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	d := dispWidth(s)
	switch {
	case d == w:
		return s
	case d > w:
		cut := truncTo(s, w)
		if cd := dispWidth(cut); cd < w {
			cut += strings.Repeat(" ", w-cd)
		}
		return cut
	default:
		return s + strings.Repeat(" ", w-d)
	}
}

// wrapPlain 按显示宽度硬折行
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
		out = append(out, strings.Split(ansi.Hardwrap(para, w, false), "\n")...)
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// fitLines 裁剪或补齐到恰好 h 行
func fitLines(lines []string, h int) []string {
	if h <= 0 {
		return nil
	}
	out := make([]string, 0, h)
	out = append(out, lines...)
	if len(out) > h {
		out = out[:h]
	}
	for len(out) < h {
		out = append(out, "")
	}
	return out
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

// humanSize 字节数人性化
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
