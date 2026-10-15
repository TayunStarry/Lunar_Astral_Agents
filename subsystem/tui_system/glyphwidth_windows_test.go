//go:build windows

package TuiSystem

import "testing"

// TestCalibrateGlyphWidthsWorks 校准路径必须真的产出结果。
//
// 这是本类故障的关键一环：校准失败会**静默退化**成内置宽度表（把中文控制台字体下占 2 列的
// 字形算作 1 列），于是每行比终端宽 1-2 格、框线右角被挤到下一行，严重时整屏折行滚动。
// 实测曾出现过「校准返回空」而无人察觉，故在此钉死。
func TestCalibrateGlyphWidthsWorks(t *testing.T) {
	f := consoleOut()
	if f == nil {
		t.Skip("当前环境没有控制台，跳过校准验证")
	}
	defer f.Close()

	measured := calibrateGlyphWidths(f)
	if len(measured) == 0 {
		t.Fatalf("校准未产出任何结果（会静默退回内置宽度表，中文控制台下界面将错位/折行）")
	}
	if want := len(probeGlyphs()) / 2; len(measured) < want {
		t.Fatalf("校准结果过少：%d 个（期望至少 %d 个，共 %d 个待测字形）",
			len(measured), want, len(probeGlyphs()))
	}
	for r, w := range measured {
		if w < 1 || w > 4 {
			t.Errorf("字形 %q 测得异常宽度 %d", string(r), w)
		}
	}
	// 框线字形在任何控制台字体下都应测得 1 或 2 列
	if w, ok := measured['─']; !ok || (w != 1 && w != 2) {
		t.Errorf("框线字形 ─ 未测得有效宽度（ok=%v w=%d）", ok, w)
	}
	t.Logf("校准成功：%d 个字形，其中宽度 >1 的有 %d 个", len(measured), countWide(measured))
}

// TestCalibrateGlyphWidthsRestoresScreen 校准不得破坏屏幕内容（探测区须原样恢复）
func TestCalibrateGlyphWidthsRestoresScreen(t *testing.T) {
	f := consoleOut()
	if f == nil {
		t.Skip("当前环境没有控制台，跳过")
	}
	defer f.Close()

	before := readConsoleRow(f, 0, 20)
	_ = calibrateGlyphWidths(f)
	after := readConsoleRow(f, 0, 20)
	if before != after {
		t.Fatalf("校准改动了屏幕内容：%q → %q", before, after)
	}
}

func countWide(m map[rune]int) int {
	n := 0
	for _, w := range m {
		if w > 1 {
			n++
		}
	}
	return n
}
