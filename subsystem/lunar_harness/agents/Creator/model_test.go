package Creator

// model_test.go 生活链简述的截断规则：只允许在标点处收尾，绝不做半句硬截断。

import (
	"strings"
	"testing"
)

func TestClipAtSentenceCutsOnPunctuation(t *testing.T) {
	text := strings.Repeat("月华正在整理这一轮的创作记录，", 10) + "最终收尾。"
	got := clipAtSentence(text, 60)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("超长文本应在标点处截断并带省略号: %q", got)
	}
	runes := []rune(got)
	if len(runes) < 2 {
		t.Fatalf("截断结果过短: %q", got)
	}
	if prev := string(runes[len(runes)-2]); !strings.ContainsAny(prev, "。！？!?；;，,、：:") {
		t.Fatalf("省略号前必须是标点，实得 %q（%q）", prev, got)
	}
}

func TestClipAtSentenceKeepsTextWithoutPunctuation(t *testing.T) {
	text := strings.Repeat("无标点长句", 40)
	if got := clipAtSentence(text, 30); got != text {
		t.Fatalf("没有标点时必须保留完整内容，不应硬截断: got=%q", got)
	}
}

func TestClipAtSentenceKeepsShortText(t *testing.T) {
	text := "短句。"
	if got := clipAtSentence(text, 30); got != text {
		t.Fatalf("未超限的文本不应被修改: %q", got)
	}
}
