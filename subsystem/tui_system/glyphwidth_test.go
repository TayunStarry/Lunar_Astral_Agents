package TuiSystem

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// cjkConsoleWidths 复刻实测结果：conhost + 新宋体（CP 65001）下这些字形占 2 列，
// 而 x/ansi 的宽度表记为 1 列。
//
// 实测工具：.scratch/widthprobe（在经典 conhost 窗口内逐个字形读光标位移）。
var cjkConsoleWidths = map[rune]int{
	'◆': 2, '■': 2, '▶': 2, '·': 2, '→': 2,
}

// applyCJKConsole 模拟中文控制台字体的校准结果
func applyCJKConsole(t *testing.T) {
	t.Helper()
	ResetMeasuredGlyphWidths()
	t.Cleanup(ResetMeasuredGlyphWidths)
	for r, w := range cjkConsoleWidths {
		SetMeasuredGlyphWidth(r, w)
	}
}

// consoleWidth 按「终端实测宽度」计算一行的列宽：
// 内置宽度表 + 被终端渲染得更宽的字形所多出的列数。
func consoleWidth(line string) int {
	visible := ansi.Strip(line)
	total := ansi.StringWidth(visible)
	for _, r := range visible {
		if w, ok := cjkConsoleWidths[r]; ok {
			total += w - 1
		}
	}
	return total
}

// TestScreensFitCJKConsole 中文控制台字体下每屏都不许折行。
//
// 这是用户报障的直接回归用例（一打开就滚动 + 显示混乱）：只要有一行在终端上的实际宽度
// 超过窗口宽度，终端就会折行、整屏高度翻倍，而备用屏幕放不下便每帧向上滚。
//
// 修复前（布局度量按内置宽度表，把这些字形算作 1 列）本用例必然失败：
// 实测 140 列的窗口会渲染出 142 列的行。
func TestScreensFitCJKConsole(t *testing.T) {
	applyCJKConsole(t)
	seedAgentTurns(12)
	for _, tc := range screenCases() {
		for _, size := range termSizes {
			m := tc.build(size.w, size.h)
			view := m.View()
			for i, line := range strings.Split(view, "\n") {
				if got := consoleWidth(line); got > size.w {
					t.Fatalf("%s@%s(%d×%d) 第 %d 行在中文控制台字体下占 %d 列 > %d 列（会折行 → 整屏滚动）\n行内容: %q",
						tc.name, size.name, size.w, size.h, i+1, got, size.w, line)
				}
			}
		}
	}
}

// TestDispWidthFollowsCalibration 校准后排版度量必须与终端一致
func TestDispWidthFollowsCalibration(t *testing.T) {
	ResetMeasuredGlyphWidths()
	if got := dispWidth("◆"); got != 1 {
		t.Fatalf("未校准时 ◆ 应按内置表计 1 列，实际 %d", got)
	}
	applyCJKConsole(t)
	cases := []struct {
		in   string
		want int
	}{
		{"◆", 2},
		{"a◆b", 4},
		{"·", 2},
		{"中", 2},                // 东亚宽字形本来就按 2 列
		{"中◆", 4},               // 2 + 2
		{"\x1b[36m◆\x1b[0m", 2}, // ANSI 序列不占列
	}
	for _, c := range cases {
		if got := dispWidth(c.in); got != c.want {
			t.Errorf("dispWidth(%q) = %d，期望 %d", c.in, got, c.want)
		}
	}
}

// TestTruncToTerminalWidth 截断结果在终端上的宽度不得超过限额
// （内含校准字形时必须走终端宽度感知的截断，ansi.Truncate 会算错）
func TestTruncToTerminalWidth(t *testing.T) {
	applyCJKConsole(t)
	long := strings.Repeat("◆更新 ", 20)
	for _, w := range []int{1, 2, 5, 11, 40} {
		got := truncTo(long, w)
		if cw := consoleWidth(got); cw > w {
			t.Errorf("truncTo(w=%d) 结果在终端上占 %d 列: %q", w, cw, got)
		}
	}
}

// TestFitWidthTerminalWidth 补齐/裁剪后恒为限额列数（终端口径）
func TestFitWidthTerminalWidth(t *testing.T) {
	applyCJKConsole(t)
	for _, in := range []string{"", "短", "◆ 更新包 · 拉取", strings.Repeat("◆x", 30)} {
		for _, w := range []int{3, 10, 24, 60} {
			got := fitWidth(in, w)
			if cw := consoleWidth(got); cw != w {
				t.Errorf("fitWidth(%q, %d) 终端宽度 = %d，期望 %d（%q）", in, w, cw, w, got)
			}
		}
	}
}

// TestAllUIGlyphsAreProbed 界面上出现的每个非 ASCII 字形都必须登记进探测表。
//
// 为什么必须守住：漏登记的字形在中文控制台上可能是 2 列，而排版按 1 列算，
// 于是整屏折行、滚动——这正是用户报的故障。新增装饰字符时必须一并登记。
func TestAllUIGlyphsAreProbed(t *testing.T) {
	seedAgentTurns(12)
	seen := map[rune]string{}
	for _, tc := range screenCases() {
		for _, size := range termSizes {
			m := tc.build(size.w, size.h)
			for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
				for _, r := range line {
					if r < 0x80 {
						continue // ASCII：任何终端都是 1 列
					}
					// 东亚宽/全角字形（中日韩、全角标点）终端一致按 2 列处理，无需探测
					if isEastAsianWide(r) {
						continue
					}
					if !isProbedGlyph(r) {
						seen[r] = tc.name
					}
				}
			}
		}
	}
	if len(seen) > 0 {
		for r, screen := range seen {
			t.Errorf("屏幕 %s 使用了未登记的字形 %q (U+%04X)：中文控制台字体下可能占 2 列，"+
				"须加入 probeGlyphsList", screen, string(r), r)
		}
	}
}

// TestEscapeSequenceLen 转义序列长度解析（宽度计算必须整段跳过）
func TestEscapeSequenceLen(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"\x1b[0m", 4},
		{"\x1b[38;5;208m", 11},
		{"\x1b[1;36mX", 7},
		{"\x1b]8;;http://x\x07", 14},
		{"\x1bM", 2},
	}
	for _, c := range cases {
		if got := escapeSequenceLen(c.in); got != c.want {
			t.Errorf("escapeSequenceLen(%q) = %d，期望 %d", c.in, got, c.want)
		}
	}
}

// TestRuneTermWidth ASCII 恒为 1 列，校准字形附加额外列
func TestRuneTermWidth(t *testing.T) {
	applyCJKConsole(t)
	cases := []struct {
		r    rune
		want int
	}{
		{'x', 1}, {' ', 1}, {'中', 2}, {'◆', 2}, {'·', 2}, {'✦', 1},
	}
	for _, c := range cases {
		if got := runeTermWidth(c.r); got != c.want {
			t.Errorf("runeTermWidth(%q) = %d，期望 %d", string(c.r), got, c.want)
		}
	}
}

// isEastAsianWide 判断是否是终端一致按 2 列渲染的东亚宽/全角字形。
// 判定口径：内置宽度表算 2 列，且不属于「歧义宽度」风险区（探针表覆盖的范围）。
func isEastAsianWide(r rune) bool {
	if r < 0x1100 {
		return false
	}
	if ansi.StringWidth(string(r)) != 2 {
		return false
	}
	switch {
	case r >= 0x2190 && r <= 0x21FF: // 箭头
		return false
	case r >= 0x2460 && r <= 0x24FF: // 带圈数字
		return false
	case r >= 0x25A0 && r <= 0x25FF: // 几何图形
		return false
	case r >= 0x2600 && r <= 0x27BF: // 杂项符号与装饰
		return false
	case r >= 0x2B00 && r <= 0x2BFF: // 杂项符号与箭头
		return false
	case r == 0x00B7 || r == 0x2022 || r == 0x2025 || r == 0x2026 || r == 0x2014 || r == 0x00D7:
		return false
	}
	_, size := utf8.DecodeRuneInString(string(r))
	return size > 1
}
