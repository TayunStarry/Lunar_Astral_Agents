package websearch

import (
	"strings"
	"testing"
)

// ── preprocessBingQuery ──

func TestPreprocessBingQuery_AlreadyQuoted(t *testing.T) {
	in := `hello "world"`
	if got := preprocessBingQuery(in); got != in {
		t.Errorf("已含双引号应原样返回，got=%q want=%q", got, in)
	}
}

func TestPreprocessBingQuery_LeftDoubleQuote(t *testing.T) {
	in := "\u201chello\u201d"
	if got := preprocessBingQuery(in); got != in {
		t.Errorf("已含中文左引号应原样返回，got=%q want=%q", got, in)
	}
}

func TestPreprocessBingQuery_SpecialSeparator(t *testing.T) {
	cases := []string{"钛宇·星光阁", "C++-tutorial", "a/b", "x|y", "a•b", "a—b", "a～b", "a~b", "a、b"}
	for _, c := range cases {
		got := preprocessBingQuery(c)
		want := "\"" + c + "\""
		if got != want {
			t.Errorf("含特殊分隔符应加引号，input=%q got=%q want=%q", c, got, want)
		}
	}
}

func TestPreprocessBingQuery_NoChinese(t *testing.T) {
	in := "hello world"
	if got := preprocessBingQuery(in); got != in {
		t.Errorf("无中文应原样返回，got=%q want=%q", got, in)
	}
}

func TestPreprocessBingQuery_ShortChinese(t *testing.T) {
	in := "你好世" // 3 个汉字 <= 3
	if got := preprocessBingQuery(in); got != in {
		t.Errorf("中文<=3应原样返回，got=%q want=%q", got, in)
	}
}

func TestPreprocessBingQuery_MediumChinese(t *testing.T) {
	in := "你好世界" // 4 个汉字
	got := preprocessBingQuery(in)
	// 中等长度中文查询不加引号，让 Bing 自然分词
	// 加引号会触发精确匹配，对含修饰词的查询（如"超自然最近更新"）有害
	if got != in {
		t.Errorf("中等长度中文应原样返回，got=%q want=%q", got, in)
	}
}

func TestPreprocessBingQuery_LongChinese(t *testing.T) {
	in := "你好世界你好世界你好世" // 11 个汉字 > 10
	if got := preprocessBingQuery(in); got != in {
		t.Errorf("中文>10应原样返回，got=%q want=%q", got, in)
	}
}

// ── truncateText ──

func TestTruncateText_NoTruncation(t *testing.T) {
	if got := truncateText("hello", 10); got != "hello" {
		t.Errorf("got=%q want=%q", got, "hello")
	}
}

func TestTruncateText_ExactLength(t *testing.T) {
	if got := truncateText("abc", 3); got != "abc" {
		t.Errorf("刚好等于上限不应截断，got=%q want=%q", got, "abc")
	}
}

func TestTruncateText_Truncated(t *testing.T) {
	if got := truncateText("hello world", 5); got != "hello..." {
		t.Errorf("got=%q want=%q", got, "hello...")
	}
}

func TestTruncateText_RuneBased(t *testing.T) {
	if got := truncateText("你好世界朋友", 4); got != "你好世界..." {
		t.Errorf("应按rune截断，got=%q want=%q", got, "你好世界...")
	}
}

func TestTruncateText_Empty(t *testing.T) {
	if got := truncateText("", 5); got != "" {
		t.Errorf("空字符串应返回空，got=%q", got)
	}
}

// ── extractTextContent ──

func TestExtractTextContent_SkipTags(t *testing.T) {
	html := `<html><body>
<script>var x=1;</script>
<nav>nav text</nav>
<header>header text</header>
<article><p>Main content</p></article>
<footer>footer text</footer>
</body></html>`
	got := extractTextContent(html)
	if !strings.Contains(got, "Main content") {
		t.Errorf("应包含 article 内文本，got=%q", got)
	}
	if strings.Contains(got, "nav text") {
		t.Errorf("不应包含 nav 文本，got=%q", got)
	}
	if strings.Contains(got, "header text") {
		t.Errorf("不应包含 header 文本，got=%q", got)
	}
	if strings.Contains(got, "footer text") {
		t.Errorf("不应包含 footer 文本，got=%q", got)
	}
	if strings.Contains(got, "var x") {
		t.Errorf("不应包含 script 文本，got=%q", got)
	}
}

func TestExtractTextContent_MainResetsSkip(t *testing.T) {
	html := `<html><body>
<nav>nav text</nav>
<main><p>main content</p></main>
</body></html>`
	got := extractTextContent(html)
	if !strings.Contains(got, "main content") {
		t.Errorf("main 标签应重置 skip，got=%q", got)
	}
	if strings.Contains(got, "nav text") {
		t.Errorf("不应包含 nav 文本，got=%q", got)
	}
}

func TestExtractTextContent_Empty(t *testing.T) {
	if got := extractTextContent(""); got != "" {
		t.Errorf("空 HTML 应返回空字符串，got=%q", got)
	}
}

// ── extractBingResults ──

func TestExtractBingResults_Basic(t *testing.T) {
	htmlStr := `<html><body><ul>
<li class="b_algo"><h2><a href="https://example.com/1">Title 1</a></h2><div class="b_caption"><p>Snippet 1</p></div></li>
<li class="b_algo"><h2><a href="https://example.com/2">Title 2</a></h2><div class="b_caption"><p>Snippet 2</p></div></li>
</ul></body></html>`

	results := extractBingResults(htmlStr, 10)
	if len(results) != 2 {
		t.Fatalf("应提取2条结果，got=%d", len(results))
	}
	if results[0].Title != "Title 1" {
		t.Errorf("results[0].Title=%q want=%q", results[0].Title, "Title 1")
	}
	if results[0].URL != "https://example.com/1" {
		t.Errorf("results[0].URL=%q want=%q", results[0].URL, "https://example.com/1")
	}
	if !strings.Contains(results[0].Snippet, "Snippet 1") {
		t.Errorf("results[0].Snippet=%q 应包含 %q", results[0].Snippet, "Snippet 1")
	}
}

func TestExtractBingResults_RelativeURL(t *testing.T) {
	htmlStr := `<li class="b_algo"><h2><a href="/relative/path">Title</a></h2><div class="b_caption"><p>Snippet</p></div></li>`
	results := extractBingResults(htmlStr, 10)
	if len(results) != 1 {
		t.Fatalf("应提取1条结果，got=%d", len(results))
	}
	want := "https://cn.bing.com/relative/path"
	if results[0].URL != want {
		t.Errorf("相对URL应补全，got=%q want=%q", results[0].URL, want)
	}
}

func TestExtractBingResults_Limit(t *testing.T) {
	htmlStr := `<ul>
<li class="b_algo"><h2><a href="https://a.com">A</a></h2><div class="b_caption"><p>S</p></div></li>
<li class="b_algo"><h2><a href="https://b.com">B</a></h2><div class="b_caption"><p>S</p></div></li>
<li class="b_algo"><h2><a href="https://c.com">C</a></h2><div class="b_caption"><p>S</p></div></li>
</ul>`
	results := extractBingResults(htmlStr, 2)
	if len(results) != 2 {
		t.Fatalf("limit=2 应只返回2条，got=%d", len(results))
	}
}

func TestExtractBingResults_Empty(t *testing.T) {
	results := extractBingResults("", 10)
	if len(results) != 0 {
		t.Errorf("空HTML应返回0条结果，got=%d", len(results))
	}
}

// ── extractBaiduResults ──

func TestExtractBaiduResults_Basic(t *testing.T) {
	htmlStr := `<html><body>
<div class="result"><h3><a href="http://example.com/1">Title 1</a></h3><div class="c-abstract">Snippet 1</div></div>
<div class="c-container"><h3><a href="http://example.com/2">Title 2</a></h3><div class="c-abstract">Snippet 2</div></div>
</body></html>`
	results := extractBaiduResults(htmlStr, 10)
	if len(results) != 2 {
		t.Fatalf("应提取2条结果，got=%d", len(results))
	}
	if results[0].Title != "Title 1" {
		t.Errorf("results[0].Title=%q want=%q", results[0].Title, "Title 1")
	}
	if results[0].URL != "http://example.com/1" {
		t.Errorf("results[0].URL=%q want=%q", results[0].URL, "http://example.com/1")
	}
}

func TestExtractBaiduResults_RelativeURL(t *testing.T) {
	htmlStr := `<div class="result"><h3><a href="/path">Title</a></h3><div class="c-abstract">Snippet</div></div>`
	results := extractBaiduResults(htmlStr, 10)
	if len(results) != 1 {
		t.Fatalf("应提取1条结果，got=%d", len(results))
	}
	want := "https://www.baidu.com/path"
	if results[0].URL != want {
		t.Errorf("相对URL应补全，got=%q want=%q", results[0].URL, want)
	}
}

// ── extractSogouResults ──

func TestExtractSogouResults_Basic(t *testing.T) {
	htmlStr := `<html><body>
<div class="rb"><h3 class="pt"><a href="http://example.com/1">Title 1</a></h3><div class="str-text">Snippet 1</div></div>
<div class="vrwrap"><h3 class="vrTitle"><a href="http://example.com/2">Title 2</a></h3><div class="str-text">Snippet 2</div></div>
</body></html>`
	results := extractSogouResults(htmlStr, 10)
	if len(results) != 2 {
		t.Fatalf("应提取2条结果，got=%d", len(results))
	}
	if results[0].Title != "Title 1" {
		t.Errorf("results[0].Title=%q want=%q", results[0].Title, "Title 1")
	}
}

func TestExtractSogouResults_RelativeURL(t *testing.T) {
	htmlStr := `<div class="rb"><h3><a href="/path">Title</a></h3><div class="str-text">Snippet</div></div>`
	results := extractSogouResults(htmlStr, 10)
	if len(results) != 1 {
		t.Fatalf("应提取1条结果，got=%d", len(results))
	}
	want := "https://www.sogou.com/path"
	if results[0].URL != want {
		t.Errorf("相对URL应补全，got=%q want=%q", results[0].URL, want)
	}
}

// ── extractDDGResults ──

func TestExtractDDGResults_Basic(t *testing.T) {
	htmlStr := `<html><body><table>
<tr class="result-snippet"><td><a href="//example.com/1">Title 1</a>Snippet 1</td></tr>
<tr class="result-snippet"><td><a href="//example.com/2">Title 2</a>Snippet 2</td></tr>
</table></body></html>`
	results := extractDDGResults(htmlStr, 10)
	if len(results) != 2 {
		t.Fatalf("应提取2条结果，got=%d", len(results))
	}
	if results[0].Title != "Title 1" {
		t.Errorf("results[0].Title=%q want=%q", results[0].Title, "Title 1")
	}
	want := "https://example.com/1"
	if results[0].URL != want {
		t.Errorf("DDG URL应补全 https: 前缀，got=%q want=%q", results[0].URL, want)
	}
}

func TestExtractDDGResults_Limit(t *testing.T) {
	htmlStr := `<table>
<tr class="result-snippet"><td><a href="//a.com">A</a>S</td></tr>
<tr class="result-snippet"><td><a href="//b.com">B</a>S</td></tr>
<tr class="result-snippet"><td><a href="//c.com">C</a>S</td></tr>
</table>`
	results := extractDDGResults(htmlStr, 2)
	if len(results) != 2 {
		t.Fatalf("limit=2 应只返回2条，got=%d", len(results))
	}
}
