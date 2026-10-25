package BilibiliViewer

import "testing"

// TestPickSearchKeywords 钉住「随机挑几个参考词去搜索」：去空、去重、数量受控。
func TestPickSearchKeywords(t *testing.T) {
	keywords := []string{"AI", " ", "Qwen", "AI", "DeepSeek", "GLM", "Kimi"}

	got := pickSearchKeywords(keywords, searchKeywordTries)
	if len(got) != searchKeywordTries {
		t.Fatalf("应挑出 %d 个词，实际 %d：%v", searchKeywordTries, len(got), got)
	}
	seen := map[string]bool{}
	allowed := map[string]bool{"AI": true, "Qwen": true, "DeepSeek": true, "GLM": true, "Kimi": true}
	for _, word := range got {
		if seen[word] {
			t.Fatalf("不应出现重复词: %v", got)
		}
		if !allowed[word] {
			t.Fatalf("出现了配置外的词: %q", word)
		}
		seen[word] = true
	}

	// 词数不足时全量返回，不报错
	if all := pickSearchKeywords([]string{"AI", "Qwen"}, 5); len(all) != 2 {
		t.Fatalf("词数不足时应返回全部，实际 %v", all)
	}
	if none := pickSearchKeywords(nil, 3); len(none) != 0 {
		t.Fatalf("无参考词时应返回空，实际 %v", none)
	}
}

// TestDropSeenKeepsUnknownBvid 钉住已看去重只按 BV 号匹配，链接解析不出 BV 时保留候选。
func TestDropSeenKeepsUnknownBvid(t *testing.T) {
	links := []videoLink{
		{Href: "https://www.bilibili.com/video/BV1xx411c7mD", Title: "看过的"},
		{Href: "https://www.bilibili.com/video/BV1yy411c7mE", Title: "没看过的"},
		{Href: "https://www.bilibili.com/other", Title: "非视频链接"},
	}
	out := dropSeen(links, map[string]bool{"BV1xx411c7mD": true})
	if len(out) != 2 {
		t.Fatalf("应剔除 1 条已看，实际剩 %d 条", len(out))
	}
	if out[0].Title != "没看过的" || out[1].Title != "非视频链接" {
		t.Fatalf("剔除结果不对：%v", out)
	}
}
