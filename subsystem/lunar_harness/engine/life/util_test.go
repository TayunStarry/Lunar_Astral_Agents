package life

import (
	"strings"
	"testing"
)

// TestTruncateAtSentence 记录截断必须在句读处收口，不能把一句话切半。
func TestTruncateAtSentence(t *testing.T) {
	text := "第一句讲的是本地模型选型。第二句开始展开显存与量化之间的关系，并提到了很多细节；第三句讲别的。"
	got := truncateAtSentence(text, 40)
	if !strings.HasSuffix(got, "。") && !strings.HasSuffix(got, "；") {
		t.Fatalf("应在句读处切断，实际 %q", got)
	}
	if len([]rune(got)) > 40 {
		t.Fatalf("不应超过上限，实际 %d 字: %q", len([]rune(got)), got)
	}
	if strings.HasSuffix(got, "半") {
		t.Fatalf("不应停在半个词上: %q", got)
	}

	// 没有任何句读时退化为硬切并补省略号
	plain := strings.Repeat("字", 80)
	if got := truncateAtSentence(plain, 30); !strings.HasSuffix(got, "…") {
		t.Fatalf("无句读时应补省略号，实际 %q", got)
	}

	// 未超长时原样返回
	if got := truncateAtSentence("短句。", 30); got != "短句。" {
		t.Fatalf("未超长不应改动，实际 %q", got)
	}
}
