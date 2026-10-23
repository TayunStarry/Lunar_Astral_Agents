package console

import (
	"encoding/json"
	"strings"
	"testing"
)

// 覆盖：即使有人绕过管线入口、把带控制序列的正文直接塞进缓冲，
// 导出侧消费 `ScrubLogText` 也必须能把转义码挡在文件之外。
//
// 为什么需要这条隔离测试：`TestExportedJSONLHasNoEscapeCodes` 与 server 包的
// 端到端测试都经由管线入口，而入口已做清洗 —— 于是"导出的兜底清洗被移除"
// 这类回归不会让它们失败（实测：把导出侧的 ScrubLogText 换回原始正文，端到端测试
// 依旧通过）。这里直接对缓冲里可能出现的脏正文做断言，独立守住兜底层。
func TestScrubLogTextGuardsExportBoundary(t *testing.T) {
	// 模拟"绕过入口"的脏正文：颜色码 + 回车刷新 + OSC 标题
	dirty := []string{
		"\x1b[1;32m[OK]\x1b[0m 已加载",
		"进度 1/3\r进度 2/3\r进度 3/3",
		"\x1b]0;标题\x07正文",
	}

	var b strings.Builder
	for _, d := range dirty {
		for _, text := range ScrubLogText(d) {
			enc, err := json.Marshal(map[string]any{"text": text})
			if err != nil {
				t.Fatalf("编码失败: %v", err)
			}
			b.Write(enc)
			b.WriteByte('\n')
		}
	}
	out := b.String()

	if strings.ContainsRune(out, 0x1b) {
		t.Fatalf("导出边界仍放行 ESC:\n%s", out)
	}
	if strings.ContainsRune(out, '\r') {
		t.Fatalf("导出边界仍放行回车:\n%s", out)
	}

	// 反向断言：可读内容必须保留（清洗过猛同样是缺陷）
	got := map[string]bool{}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("不是合法 JSON: %v（%s）", err, line)
		}
		text, _ := rec["text"].(string)
		got[text] = true
	}
	for _, want := range []string{"[OK] 已加载", "进度 1/3", "进度 2/3", "进度 3/3", "正文"} {
		if !got[want] {
			t.Fatalf("缺少剥净后的正文 %q；实际得到 %v", want, got)
		}
	}
}
