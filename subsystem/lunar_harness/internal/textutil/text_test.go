package textutil

// text_test.go 锁定全仓唯一的截断/首非空语义，防止各包再复制一份后悄悄漂移。

import (
	"strings"
	"testing"
)

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{"未超长返回原文", "短文本", 10, "短文本"},
		{"超长保留 limit 字并加省略号", "一二三四五", 3, "一二三…"},
		{"按字符而非字节截断", "月华看视频", 2, "月华…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TruncateRunes(tc.text, tc.limit); got != tc.want {
				t.Fatalf("TruncateRunes(%q,%d)=%q，期望 %q", tc.text, tc.limit, got, tc.want)
			}
		})
	}
}

func TestHardCutRunesWithNotice(t *testing.T) {
	if got := HardCutRunesWithNotice("  一二三四五  ", 3); got != "一二…（已截断）" {
		t.Fatalf("HardCutRunesWithNotice 超长时应去空白并加标记，实际 %q", got)
	}
	if got := HardCutRunesWithNotice("  短  ", 10); got != "短" {
		t.Fatalf("HardCutRunesWithNotice 未超长时应只去首尾空白，实际 %q", got)
	}
}

func TestTruncateRunesTrimmed(t *testing.T) {
	if got := TruncateRunesTrimmed("  一二三四五  ", 3); got != "一二三…" {
		t.Fatalf("TruncateRunesTrimmed 超长时应去空白并加省略号，实际 %q", got)
	}
	if got := TruncateRunesTrimmed("  短  ", 10); got != "短" {
		t.Fatalf("TruncateRunesTrimmed 未超长时应只去首尾空白，实际 %q", got)
	}
	if got := TruncateRunesTrimmed("  短  ", 0); got != "短" {
		t.Fatalf("TruncateRunesTrimmed 的 limit<=0 应只去首尾空白，实际 %q", got)
	}
}

func TestClipRunes(t *testing.T) {
	if got := ClipRunes("一二三四五", 3); got != "一二三" {
		t.Fatalf("ClipRunes 应硬切且不追加标记，实际 %q", got)
	}
	if got := ClipRunes("  一二  ", 10); got != "  一二  " {
		t.Fatalf("ClipRunes 未超长时应保留原空白，实际 %q", got)
	}
	if got := ClipRunes("一二", 0); got != "一二" {
		t.Fatalf("ClipRunes 的 limit<=0 应原样返回，实际 %q", got)
	}
}

func TestCutRunes(t *testing.T) {
	// 总长度受 limit 约束（省略号占一位），与 agentbase.CutRunes 语义一致。
	if got := CutRunes("一二三四五", 3); got != "一二…" {
		t.Fatalf("CutRunes 应为总长 3，实际 %q", got)
	}
	if got := CutRunes("abc", 0); got != "…" {
		t.Fatalf("CutRunes 的 limit<1 应按 1 处理，实际 %q", got)
	}
}

func TestTruncateAtSentence(t *testing.T) {
	text := "第一句讲的是本地模型选型。第二句开始展开显存与量化之间的关系，并提到了很多细节；第三句讲别的。"
	got := TruncateAtSentence(text, 40)
	if !strings.HasSuffix(got, "。") && !strings.HasSuffix(got, "；") {
		t.Fatalf("应在句读处切断，实际 %q", got)
	}
	if len([]rune(got)) > 40 {
		t.Fatalf("不应超过上限，实际 %d 字: %q", len([]rune(got)), got)
	}
	if got := TruncateAtSentence(strings.Repeat("字", 80), 30); len([]rune(got)) != 31 || !strings.HasSuffix(got, "…") {
		t.Fatalf("无句读时应硬切并补省略号，实际 %q", got)
	}
	if got := TruncateAtSentence("短句。", 30); got != "短句。" {
		t.Fatalf("未超长不应改动，实际 %q", got)
	}
}

func TestTruncateAtSentenceMinFill(t *testing.T) {
	text := strings.Repeat("字", 30) + "，" + strings.Repeat("字", 100)
	if got := TruncateAtSentence(text, 100); len([]rune(got)) != 32 {
		t.Fatalf("默认实现应停在逗号后，实际 %d 字: %q", len([]rune(got)), got)
	}
	if got := TruncateAtSentenceMinFill(text, 100); len([]rune(got)) != 101 || !strings.HasSuffix(got, "…") {
		t.Fatalf("句读过早时应硬切到 limit 并补省略号，实际 %d 字: %q", len([]rune(got)), got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := FirstNonEmpty("  ", "", "月华"); got != "月华" {
		t.Fatalf("FirstNonEmpty 应跳过空白项，实际 %q", got)
	}
	if got := FirstNonEmpty(); got != "" {
		t.Fatalf("FirstNonEmpty 无入参应返回空串，实际 %q", got)
	}
}

func TestBVID(t *testing.T) {
	if got := ExtractBVID("看 https://www.bilibili.com/video/BV1xx411c7mD 这个"); got != "BV1xx411c7mD" {
		t.Fatalf("ExtractBVID 提取错误：%q", got)
	}
	if !IsBVID("BV1xx411c7mD") || IsBVID("前缀BV1xx411c7mD") {
		t.Fatalf("IsBVID 整串匹配语义错误")
	}
	if got := ReplaceBVID("BV1xx411c7mD 和 BV1yy411c7mE", func(string) string { return "[视频]" }); got != "[视频] 和 [视频]" {
		t.Fatalf("ReplaceBVID 批量替换错误：%q", got)
	}
}
