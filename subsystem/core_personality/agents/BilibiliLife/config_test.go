package BilibiliLife

import "testing"

func TestFilterAndScoreBlocksPoliticsAndConfrontation(t *testing.T) {
	cfg := defaultConfig()
	links := []videoLink{
		{Href: "https://www.bilibili.com/video/BV1", Title: "中美关系最新进展"},
		{Href: "https://www.bilibili.com/video/BV2", Title: "男女对立又上热搜了"},
		{Href: "https://www.bilibili.com/video/BV3", Title: "AI 绘画入门：大肥鱼模型实测"},
		{Href: "https://www.bilibili.com/video/BV4", Title: "直播回放：昨天"},
	}
	out, blockedCount := filterAndScore(links, cfg)
	if blockedCount != 3 {
		t.Fatalf("应过滤 3 条涉政/对立/录播，实际 %d：%#v", blockedCount, out)
	}
	if len(out) != 1 || out[0].Href != "https://www.bilibili.com/video/BV3" {
		t.Fatalf("应只留下 AI 视频，实际 %#v", out)
	}
	if out[0].Score <= 0 {
		t.Fatalf("AI 参考词应加分，实际 score=%d", out[0].Score)
	}
}

func TestBlockedKeywordsAreNonEmpty(t *testing.T) {
	cfg := defaultConfig()
	for _, word := range []string{"政治", "男女对立", "引战"} {
		if _, ok := blockedBy("测试"+word+"内容", cfg.BlockedKeywords); !ok {
			t.Fatalf("内置安全过滤缺少 %q", word)
		}
	}
}
