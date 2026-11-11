package multimodal

import "testing"

func TestNormalizeOutputSubdir(t *testing.T) {
	cases := map[string]string{
		"我的演绎":      "我的演绎",
		" 星月/项目 ":   "星月项目",
		"../escape": "escape",
		"a\\b:c":    "abc",
		"":          "",
		" . ":       "",
	}
	for input, want := range cases {
		if got := normalizeOutputSubdir(input); got != want {
			t.Fatalf("normalizeOutputSubdir(%q) = %q，期望 %q", input, got, want)
		}
	}
}
