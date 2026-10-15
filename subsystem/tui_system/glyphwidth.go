package TuiSystem

// ==== 终端字形宽度校准 ====
//
// 背景（实测故障）：一批「东亚歧义宽度」字形在中文控制台字体（新宋体等）下实际占 **2 列**，
// 而 x/ansi 的宽度表记 1 列。界面按「恰好铺满终端宽度」排版，于是每行都比终端宽 →
// 自动折行 → 整屏高度翻倍 → 备用屏幕放不下就每帧向上滚。
// 实测（conhost + 新宋体，CP 65001）：◆ ■ ▶ · → 均为 2 列，框线字形为 1 列。
//
// 因此排版度量必须与终端一致：启动时在真实控制台上实测这些字形（calibrateGlyphWidths），
// 之后 dispWidth / truncTo（见 theme.go）都按实测宽度计算，行宽才真正等于终端列数。
//
// 注意：只做「字形替换」是不够的——行宽在排版阶段就已算出，度量错了，替换只能维持
// 那个已经错误的宽度（早期尝试正是栽在这里：整行仍比终端宽，照样折行）。

import (
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// probeGlyphsList 需要实测的字形集合：非 ASCII、且宽度可能随控制台字体而异的字形。
//
// 新增界面装饰字符时必须登记在此，否则 TestAllUIGlyphsAreProbed 会失败——
// 漏登记的字形在中文控制台上会把整屏顶到折行。
var probeGlyphsList = []rune{
	// 框线（Unicode 里属歧义宽度，部分 CJK 字体按 2 列渲染）
	'─', '│', '╭', '╮', '╰', '╯', '├', '┤', '┬', '┴', '┼', '━', '┃',
	// 进度/强调块与柱状图
	'█', '▓', '▒', '░', '▀', '▄', '▌', '▐',
	// 列表与状态图标
	'◆', '◇', '●', '○', '■', '□', '▪', '▫', '▶', '▸', '►', '◀', '◂',
	'✦', '✧', '★', '☆', '✓', '✔', '✗', '✘', '⚠', '⚙', '⏸', '💭',
	// 分隔与箭头
	'·', '•', '‥', '…', '—', '–', '×', '≠', '≥', '≤', '↕',
	'→', '←', '↑', '↓',
	// 带圈数字（发布说明里用①②③标注步骤）
	'①', '②', '③', '④', '⑤', '⑥', '⑦', '⑧', '⑨', '⑩',
	// 加载动画（bubbles spinner.Dot 的各帧）
	'⣾', '⣽', '⣻', '⢿', '⡿', '⣟', '⣯', '⣷',
}

// probeGlyphs 返回去重后的探测字形（保持登记顺序，便于日志阅读）
func probeGlyphs() []rune {
	seen := make(map[rune]bool, len(probeGlyphsList))
	out := make([]rune, 0, len(probeGlyphsList))
	for _, r := range probeGlyphsList {
		if seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// isProbedGlyph 该字形是否已登记
func isProbedGlyph(r rune) bool {
	for _, p := range probeGlyphsList {
		if p == r {
			return true
		}
	}
	return false
}

var (
	glyphWidthMu sync.RWMutex
	// wideGlyphs 只记录「实测比宽度表更宽」的字形（通常只有几个），
	// 排版度量每次渲染都要查它，因此保持精简。
	wideGlyphs = map[rune]int{}
)

// SetMeasuredGlyphWidth 记录某字形的实测占位宽度（只保留 >1 的字形）
func SetMeasuredGlyphWidth(r rune, w int) {
	if w < 1 || w > 4 {
		return
	}
	glyphWidthMu.Lock()
	defer glyphWidthMu.Unlock()
	if w == 1 {
		delete(wideGlyphs, r)
		return
	}
	wideGlyphs[r] = w
}

// ResetMeasuredGlyphWidths 清空校准结果（测试用）
func ResetMeasuredGlyphWidths() {
	glyphWidthMu.Lock()
	defer glyphWidthMu.Unlock()
	wideGlyphs = map[rune]int{}
}

// measuredGlyphWidth 取实测宽度；未校准时返回 (0, false)
func measuredGlyphWidth(r rune) (int, bool) {
	glyphWidthMu.RLock()
	defer glyphWidthMu.RUnlock()
	w, ok := wideGlyphs[r]
	return w, ok
}

// wideGlyphExtra 该字形比宽度表多占的列数（0 表示一致）
func wideGlyphExtra(r rune) int {
	glyphWidthMu.RLock()
	defer glyphWidthMu.RUnlock()
	if w, ok := wideGlyphs[r]; ok {
		return w - 1
	}
	return 0
}

// hasWideGlyphs 快速判断字符串里是否含需要按额外宽度计算的字形
func hasWideGlyphs(s string) bool {
	glyphWidthMu.RLock()
	defer glyphWidthMu.RUnlock()
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

// runeTermWidth 单个字形在终端上的列宽
func runeTermWidth(r rune) int {
	if r < 0x80 {
		return 1 // ASCII：任何控制台字体都是 1 列
	}
	return ansi.StringWidth(string(r)) + wideGlyphExtra(r)
}

// extraWidthOf 字符串里「校准字形多占的列数」总和（跳过 ANSI 转义序列）
func extraWidthOf(s string) int {
	extra := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i += escapeSequenceLen(s[i:])
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		extra += wideGlyphExtra(r)
		i += size
	}
	return extra
}

// consoleTruncate 终端宽度感知的截断：按实测列宽累积，超出即止，末尾补省略号。
// 仅在字符串含校准字形时使用（其余情况仍走 ansi.Truncate，它对 emoji 组合更精确）。
func consoleTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	ellipsis := "…"
	ew := runeTermWidth('…')
	if ew < 1 {
		ew = 1
	}
	if w <= ew {
		return ""
	}
	budget := w - ew

	var b strings.Builder
	b.Grow(len(s))
	used := 0
	i := 0
	for i < len(s) {
		if s[i] == 0x1b {
			n := escapeSequenceLen(s[i:])
			b.WriteString(s[i : i+n])
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runeTermWidth(r)
		if used+rw > budget {
			break
		}
		b.WriteRune(r)
		used += rw
		i += size
	}
	b.WriteString(ellipsis)
	// 截断可能切断样式区间：补一个复位，避免颜色溢出到后续内容
	if strings.IndexByte(s, 0x1b) >= 0 {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// escapeSequenceLen 返回从 ESC 起的转义序列字节长度。
// 覆盖 CSI（ESC [ … 终止字节）、OSC（ESC ] … BEL/ST）与其余双字节序列。
func escapeSequenceLen(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	switch s[1] {
	case '[': // CSI：参数/中间字节后跟 0x40-0x7E 的终止字节
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
		return len(s)
	case ']': // OSC：直到 BEL 或 ST（ESC \）
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
		return len(s)
	default:
		return 2
	}
}
