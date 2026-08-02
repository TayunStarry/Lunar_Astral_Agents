package websearch

import (
	"strings"
	"testing"
)

// ── formatResults ──

func TestFormatResults_Basic(t *testing.T) {
	results := []SearchResult{
		{Title: "标题1", Snippet: "摘要1"},
		{Title: "标题2", Snippet: "摘要2"},
	}
	got := formatResults(results)
	if !strings.Contains(got, "「标题1」") {
		t.Errorf("应包含标题1，got=%q", got)
	}
	if !strings.Contains(got, "：摘要1") {
		t.Errorf("应包含摘要1，got=%q", got)
	}
	if !strings.Contains(got, "「标题2」") {
		t.Errorf("应包含标题2，got=%q", got)
	}
}

func TestFormatResults_EmptySnippet(t *testing.T) {
	results := []SearchResult{
		{Title: "标题", Snippet: ""},
	}
	got := formatResults(results)
	if !strings.Contains(got, "「标题」") {
		t.Errorf("应包含标题，got=%q", got)
	}
	if strings.Contains(got, "：") {
		t.Errorf("空摘要不应有冒号，got=%q", got)
	}
}

func TestFormatResults_Empty(t *testing.T) {
	got := formatResults(nil)
	if got != "" {
		t.Errorf("空结果应返回空字符串，got=%q", got)
	}
}

// ── formatResultsTruncated ──

func TestFormatResultsTruncated_NoTruncation(t *testing.T) {
	results := []SearchResult{
		{Title: "标题", Snippet: "短摘要"},
	}
	got := formatResultsTruncated(results, 100)
	if !strings.Contains(got, "「标题」：短摘要") {
		t.Errorf("短摘要不应截断，got=%q", got)
	}
}

func TestFormatResultsTruncated_Truncated(t *testing.T) {
	longSnippet := strings.Repeat("a", 200)
	results := []SearchResult{
		{Title: "标题", Snippet: longSnippet},
	}
	got := formatResultsTruncated(results, 50)
	if strings.Contains(got, strings.Repeat("a", 200)) {
		t.Errorf("长摘要应被截断，got=%q", got)
	}
	if !strings.Contains(got, "...") {
		t.Errorf("截断后应包含省略号，got=%q", got)
	}
	// 验证截断后的前缀
	if !strings.Contains(got, strings.Repeat("a", 50)) {
		t.Errorf("应保留前50个字符，got=%q", got)
	}
}

func TestFormatResultsTruncated_RuneBased(t *testing.T) {
	longSnippet := strings.Repeat("你", 200) // 200个中文rune
	results := []SearchResult{
		{Title: "标题", Snippet: longSnippet},
	}
	got := formatResultsTruncated(results, 50)
	// 应按rune截断，保留50个"你"
	if !strings.Contains(got, strings.Repeat("你", 50)+"...") {
		t.Errorf("应按rune截断保留50个字符+省略号，got 长度=%d", len(got))
	}
}

// ── formatWebpageResultsFallback ──

func TestFormatWebpageResultsFallback_Basic(t *testing.T) {
	got := formatWebpageResultsFallback("测试查询", []string{"内容1", "内容2"})
	if !strings.Contains(got, `"测试查询"`) {
		t.Errorf("应包含查询词，got=%q", got)
	}
	if !strings.Contains(got, "内容1") {
		t.Errorf("应包含内容1，got=%q", got)
	}
	if !strings.Contains(got, "内容2") {
		t.Errorf("应包含内容2，got=%q", got)
	}
}

func TestFormatWebpageResultsFallback_Truncation(t *testing.T) {
	// 构造超过 webpageMaxFallbackChars(4000) 的内容
	longContent := strings.Repeat("a", 5000)
	got := formatWebpageResultsFallback("q", []string{longContent})
	if !strings.Contains(got, "[内容已截断]") {
		t.Errorf("超长内容应被截断并标注，got 长度=%d", len(got))
	}
}

func TestFormatWebpageResultsFallback_NoTruncation(t *testing.T) {
	shortContent := "短内容"
	got := formatWebpageResultsFallback("q", []string{shortContent})
	if strings.Contains(got, "[内容已截断]") {
		t.Errorf("短内容不应截断，got=%q", got)
	}
}
