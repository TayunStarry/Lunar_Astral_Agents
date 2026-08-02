package websearch

import (
	"testing"
)

// TestIsNewsLink 验证新闻链接识别（不要求尾斜杠）
func TestIsNewsLink(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://www.chaoziran.com/news", true},       // 无尾斜杠（之前漏识别）
		{"https://www.chaoziran.com/news/", true},      // 带尾斜杠
		{"https://www.chaoziran.com/m/news/652", true}, // 移动版新闻详情
		{"https://www.chaoziran.com/page/privacy", false}, // 隐私政策页（不应识别为新闻）
		{"https://www.chaoziran.com/page/table_permission", false},
		{"https://example.com/article/123", true},
		{"https://example.com/announcement", true},
		{"https://example.com/home", false},
	}
	for _, c := range cases {
		got := isNewsLink(c.url)
		if got != c.want {
			t.Errorf("isNewsLink(%q) = %v, want %v", c.url, got, c.want)
		} else {
			t.Logf("✓ isNewsLink(%q) = %v", c.url, got)
		}
	}
}

// TestFilterNewsLinks 验证 chaoziran.com 首页链接能筛选出新闻页
func TestFilterNewsLinks(t *testing.T) {
	links := []string{
		"https://www.chaoziran.com/",
		"https://www.chaoziran.com/news",
		"https://www.chaoziran.com/page/privacy",
		"https://www.chaoziran.com/page/table_permission",
		"https://www.chaoziran.com/m/news/652",
		"https://cdn.chaoziran.com/style.css",
	}
	got := filterNewsLinks(links)
	t.Logf("筛选结果: %v", got)
	// 应包含 /news 和 /m/news/652，不包含 privacy
	if len(got) != 2 {
		t.Errorf("筛选数量 = %d, want 2 (news + m/news/652)", len(got))
	}
	for _, l := range got {
		if !isNewsLink(l) {
			t.Errorf("筛选结果含非新闻链接: %s", l)
		}
	}
}
