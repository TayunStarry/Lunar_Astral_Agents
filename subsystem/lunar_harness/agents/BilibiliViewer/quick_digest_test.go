package BilibiliViewer

// quick_digest_test.go 直驱印象材料的回归测试：
// 评论去重键与材料排版（直驱模式需覆盖全部采集评论，速览只展示少量）。

import (
	"strings"
	"testing"
)

// TestCommentDedupeKey 空白折叠 + 忽略大小写，重复刷屏应归并为同一个键
func TestCommentDedupeKey(t *testing.T) {
	cases := [][2]string{
		{"  第一  ", "第一"},
		{"第一", "第一"},
		{"666", "666"},
		{"Hello World", "hello world"},
		{"Hello   World", "hello world"},
		{"", ""},
	}
	for _, c := range cases {
		if got := commentDedupeKey(c[0]); got != c[1] {
			t.Fatalf("评论 %q 的去重键应为 %q，实得 %q", c[0], c[1], got)
		}
	}
}

// TestQuickDigestMaterialRows 评论行数受 commentRows 控制，弹幕全部进入材料
func TestQuickDigestMaterialRows(t *testing.T) {
	info := videoInfo{Title: "测试视频", Owner: "UP主", Duration: 180}
	comments := make([]commentItem, 0, 10)
	for i := 0; i < 10; i++ {
		comments = append(comments, commentItem{User: "用户", Content: strings.Repeat("评", 1) + string(rune('A'+i))})
	}
	danmaku := []string{"前排", "这也太强了"}

	material := quickDigestMaterial(info, []string{"科普"}, comments, danmaku, 8)
	if !strings.Contains(material, comments[7].Content) {
		t.Fatalf("第 8 条评论应进入材料：%s", material)
	}
	if strings.Contains(material, comments[8].Content) {
		t.Fatalf("第 9 条评论应被 commentRows 截掉：%s", material)
	}
	for _, text := range danmaku {
		if !strings.Contains(material, text) {
			t.Fatalf("弹幕 %q 应进入材料：%s", text, material)
		}
	}
	// 直驱印象传 0 之外的大值时应覆盖全部评论
	if full := quickDigestMaterial(info, nil, comments, nil, 60); !strings.Contains(full, comments[9].Content) {
		t.Fatalf("直驱印象材料应包含全部评论：%s", full)
	}
}
