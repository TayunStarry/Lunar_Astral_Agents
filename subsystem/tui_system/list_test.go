package TuiSystem

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// makeBlocks 构造 n 个单行条目
func makeBlocks(n int) [][]string {
	out := make([][]string, n)
	for i := range out {
		out[i] = []string{fmt.Sprintf("item-%02d", i+1)}
	}
	return out
}

// TestListPaneKeepsCursorVisible 窗口化列表的核心契约：
// 无论选中第几项，窗口行数恒为给定高度、列宽恒为给定宽度，且选中项必在窗口内。
func TestListPaneKeepsCursorVisible(t *testing.T) {
	const (
		w, h = 24, 8
		n    = 50
	)
	blocks := makeBlocks(n)
	var p listPane
	for cursor := 0; cursor < n; cursor++ {
		lines, info := p.layoutView(blocks, cursor, w, h)
		if len(lines) != h {
			t.Fatalf("选中 %d 时窗口 %d 行，应为 %d 行", cursor, len(lines), h)
		}
		for i, l := range lines {
			if got := ansi.StringWidth(l); got != w {
				t.Fatalf("选中 %d 时第 %d 行列宽 %d，应为 %d", cursor, i+1, got, w)
			}
		}
		want := fmt.Sprintf("item-%02d", cursor+1)
		if !strings.Contains(strings.Join(lines, "\n"), want) {
			t.Fatalf("选中项 %s 不在窗口内：%v", want, lines)
		}
		if info.Cur != cursor || info.Items != n {
			t.Fatalf("位置信息不符：cur=%d items=%d", info.Cur, info.Items)
		}
	}
}

// TestListPaneTallBlockKeepsTitleVisible 高于窗口的条目对齐「块顶」：
// 宁可截断尾部，也要保留标题——长描述条目不能把标题滚没。
func TestListPaneTallBlockKeepsTitleVisible(t *testing.T) {
	blocks := [][]string{
		{"短条目"},
		{"长条目-标题", "行2", "行3", "行4", "行5", "行6", "行7", "行8", "行9", "行10"},
	}
	var p listPane
	lines, _ := p.layoutView(blocks, 1, 20, 4)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "长条目-标题") {
		t.Fatalf("高于窗口的条目应保留标题，实际窗口：%v", lines)
	}
}

// TestListPaneEmptyAndDegenerate 空列表与非法尺寸不得 panic，且行数/列宽符合约定
func TestListPaneEmptyAndDegenerate(t *testing.T) {
	var p listPane
	if lines, info := p.layoutView(nil, 0, 20, 5); len(lines) != 5 || info.Items != 0 {
		t.Fatalf("空列表应返回 5 行空窗口，实际 %d 行", len(lines))
	}
	if lines, _ := p.layoutView(makeBlocks(3), 0, 0, 3); len(lines) != 3 {
		t.Fatalf("宽度为 0 时仍应返回约定行数（零宽空行），实际 %d 行", len(lines))
	}
	if lines, _ := p.layoutView(makeBlocks(3), 0, 20, 0); lines != nil {
		t.Fatalf("高度为 0 时应返回 nil")
	}
	// 越界选中项收敛
	lines, info := p.layoutView(makeBlocks(3), 99, 20, 3)
	if info.Cur != 2 {
		t.Fatalf("越界选中项应收敛到末项，实际 %d", info.Cur)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "item-03") {
		t.Fatalf("收敛后的选中项应在窗口内")
	}
}

// TestListPaneScrollbarAppears 超屏时最右侧出现滚动条列（宽度仍守恒）
func TestListPaneScrollbarAppears(t *testing.T) {
	var p listPane
	lines, info := p.layoutView(makeBlocks(40), 0, 30, 6)
	if info.Total <= 6 {
		t.Fatalf("测试前置：内容应超屏")
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "█") {
		t.Fatalf("超屏时应有滚动条滑块：%v", lines)
	}
	for _, l := range lines {
		if got := ansi.StringWidth(l); got != 30 {
			t.Fatalf("含滚动条时行宽应仍为 30，实际 %d", got)
		}
	}
}

// TestListMoveAndNavKey 键位语义：单步可循环、翻页与首尾不循环
func TestListMoveAndNavKey(t *testing.T) {
	if got := listMove(0, -1, 5, true); got != 4 {
		t.Fatalf("循环上移应到末项，实际 %d", got)
	}
	if got := listMove(4, 1, 5, true); got != 0 {
		t.Fatalf("循环下移应到首项，实际 %d", got)
	}
	if got := listMove(0, -1, 5, false); got != 0 {
		t.Fatalf("非循环上移应止于首项，实际 %d", got)
	}
	if got := listMove(4, 1, 5, false); got != 4 {
		t.Fatalf("非循环下移应止于末项，实际 %d", got)
	}

	cases := []struct {
		cur, n, page int
		key          string
		want         int
	}{
		{3, 10, 4, "down", 4},
		{3, 10, 4, "j", 4},
		{3, 10, 4, "up", 2},
		{3, 10, 4, "pgdown", 7},
		{3, 10, 4, "pgup", 0},
		{3, 10, 4, "home", 0},
		{3, 10, 4, "g", 0},
		{3, 10, 4, "end", 9},
		{3, 10, 4, "G", 9},
	}
	for _, c := range cases {
		got, ok := listNavKey(c.cur, c.key, c.n, c.page, true)
		if !ok || got != c.want {
			t.Fatalf("listNavKey(%d, %q) = %d(ok=%v)，期望 %d", c.cur, c.key, got, ok, c.want)
		}
	}
	if _, ok := listNavKey(0, "x", 10, 4, true); ok {
		t.Fatalf("未知键位不应被消费")
	}
	if got := navAlias("left"); got != "up" {
		t.Fatalf("left 应归一为 up，实际 %q", got)
	}
	if got := navAlias("tab"); got != "down" {
		t.Fatalf("tab 应归一为 down，实际 %q", got)
	}
}

// TestWindowLinesFollowAndScroll 行窗口的跟随语义：跟随态贴底、暂停后维持位置、
// 滚回底部自动恢复跟随。
func TestWindowLinesFollowAndScroll(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%03d", i)
	}

	win, off, info := windowLines(lines, 0, 10, true)
	if off != 90 || info.Top != 90 {
		t.Fatalf("跟随态应贴底（offset=90），实际 %d", off)
	}
	if !strings.Contains(win[9], "line-099") {
		t.Fatalf("跟随态最后一行应为最新行，实际 %q", win[9])
	}

	next, follow, ok := scrollKey(off, "pgup", len(lines), 10, true)
	if !ok || follow {
		t.Fatalf("向上翻页应暂停跟随")
	}
	if next >= off {
		t.Fatalf("向上翻页应减小 offset：%d → %d", off, next)
	}

	// 翻回底部：恢复跟随
	next, follow, _ = scrollKey(next, "end", len(lines), 10, false)
	if !follow || next != 90 {
		t.Fatalf("End 应恢复跟随并贴底，实际 offset=%d follow=%v", next, follow)
	}

	// 内容不足一屏时 offset 恒为 0
	_, off, _ = windowLines(lines[:3], 5, 10, false)
	if off != 0 {
		t.Fatalf("内容不足一屏时 offset 应为 0，实际 %d", off)
	}
}

// TestWidthHelpersCJK 中文按显示宽度计算：补齐与裁剪均不得错列
func TestWidthHelpersCJK(t *testing.T) {
	if got := dispWidth("运行时间"); got != 8 {
		t.Fatalf("「运行时间」显示宽度应为 8，实际 %d", got)
	}
	if got := dispWidth(fitWidth("运行时间", 14)); got != 14 {
		t.Fatalf("中文补齐后宽度应为 14，实际 %d", got)
	}
	if got := dispWidth(truncTo("运行时间与资源占用总量", 9)); got > 9 {
		t.Fatalf("中文裁剪后宽度不得超限，实际 %d", got)
	}
	if got := dispWidth(sectionRule("运行状态", 30)); got != 30 {
		t.Fatalf("区块分隔线宽度应为 30，实际 %d", got)
	}
	if got := dispWidth(stepsBar(marketPublishSteps, 1, 40)); got != 40 {
		t.Fatalf("步骤条宽度应为 40，实际 %d", got)
	}
}

// TestRowBuilderExactWidth 行组装器：任意片段组合下均恰好 width 列，
// 且选中态下每个片段都带上底色（避免整行高亮半截变色）。
func TestRowBuilderExactWidth(t *testing.T) {
	cases := []struct {
		width    int
		selected bool
		left     []string
		right    []string
	}{
		{40, false, []string{"▸ ", "标题"}, []string{"标签"}},
		{40, true, []string{"▸ ", "标题"}, []string{"标签"}},
		{10, false, []string{"超长光标本行内容", "更长的主体"}, []string{"右侧点缀"}},
		{60, true, []string{"◆ 工具包 pkg-01"}, nil},
		{20, false, nil, []string{"仅有右侧"}},
		{5, true, []string{"ab"}, []string{"cd"}},
	}
	for _, c := range cases {
		r := newRow(c.width, c.selected).L(c.left...)
		if len(c.right) > 0 {
			r.R(c.right...)
		}
		got := r.String()
		if d := dispWidth(got); d != c.width {
			t.Fatalf("行宽 %d ≠ 期望 %d（left=%v right=%v selected=%v）", d, c.width, c.left, c.right, c.selected)
		}
		if c.selected {
			// 底色须分别包裹每个片段（含填充），否则内层样式复位会截断底色，
			// 表现为「整行高亮只亮了一半」
			need := len(c.left) + len(c.right)
			if n := strings.Count(got, "48;5;237"); n < need {
				t.Fatalf("选中行底色片段数 %d < 片段数 %d：%q", n, need, got)
			}
		}
	}
}

// TestPosTextAndHumanSize 位置文本与体积展示
func TestPosTextAndHumanSize(t *testing.T) {
	if got := posText(2, 17, "个包"); got != "3/17 个包" {
		t.Fatalf("位置文本不符：%q", got)
	}
	if got := posText(0, 0, "个包"); got != "" {
		t.Fatalf("空列表位置文本应为空，实际 %q", got)
	}
	if got := humanSize(0); got != "-" {
		t.Fatalf("0 字节应显示 -，实际 %q", got)
	}
	if got := humanSize(1536); got != "1.5 KB" {
		t.Fatalf("1536 字节应为 1.5 KB，实际 %q", got)
	}
	if got := humanSize(3 * 1024 * 1024); got != "3.0 MB" {
		t.Fatalf("3MB 应为 3.0 MB，实际 %q", got)
	}
}
