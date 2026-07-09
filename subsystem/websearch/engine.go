package websearch

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

// NewBingSearcher 创建必应搜索器
func NewBingSearcher(timeout time.Duration, userAgent string) *BingSearcher {
	return &BingSearcher{
		client: &http.Client{Timeout: timeout},
	}
}

func (b *BingSearcher) Name() string { return "Bing" }

func (b *BingSearcher) Search(query string, limit int) ([]SearchResult, error) {
	processedQuery := preprocessBingQuery(query)
	searchURL := fmt.Sprintf("https://www.bing.com/search?q=%s&mkt=zh-CN&setlang=zh-hans&ensearch=0",
		url.QueryEscape(processedQuery))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", defaultConfig.HTTP.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Bing 搜索失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Bing 搜索返回异常状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取搜索响应失败: %w", err)
	}

	return extractBingResults(string(body), limit), nil
}

// SearchRaw 直接搜索，跳过预处理（用于回退策略，避免引号精确匹配过窄）
func (b *BingSearcher) SearchRaw(query string, limit int) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://www.bing.com/search?q=%s&mkt=zh-CN&setlang=zh-hans&ensearch=0",
		url.QueryEscape(query))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", defaultConfig.HTTP.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Bing 搜索失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Bing 搜索返回异常状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取搜索响应失败: %w", err)
	}

	return extractBingResults(string(body), limit), nil
}

// preprocessBingQuery 预处理查询字符串，提升中文搜索质量
func preprocessBingQuery(query string) string {
	if strings.Contains(query, "\"") || strings.Contains(query, "\u201c") {
		return query
	}

	// 含特殊分隔符的查询（如"钛宇·星光阁"）：加引号做精确短语匹配
	// 不能拆成空格分词，否则Bing会把"钛宇"拆成"钛"+"宇"，匹配到无关内容
	hasSpecial := false
	for _, r := range query {
		if r == '·' || r == '-' || r == '/' || r == '|' || r == '•' ||
			r == '\u2014' || r == '～' || r == '~' || r == '、' {
			hasSpecial = true
			break
		}
	}
	if hasSpecial {
		// 保留原样加引号：Bing 对引号内的短语做精确匹配
		return "\"" + query + "\""
	}

	chineseCount := 0
	for _, r := range query {
		if unicode.Is(unicode.Han, r) {
			chineseCount++
		}
	}

	if chineseCount == 0 || chineseCount <= 3 || chineseCount > 10 {
		return query
	}

	return "\"" + query + "\""
}

func extractBingResults(htmlStr string, limit int) []SearchResult {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}

	var results []SearchResult
	var inAlgo bool
	var currentResult SearchResult
	var inH2 bool
	var inCaption bool
	var skipChildren bool

	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if len(results) >= limit {
			return
		}

		if n.Type == html.ElementNode {
			if n.Data == "li" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "b_algo") {
						inAlgo = true
						currentResult = SearchResult{}
						inH2 = false
						inCaption = false
						skipChildren = false
						break
					}
				}
			}
			if inAlgo && n.Data == "h2" {
				inH2 = true
			}
			if inAlgo && n.Data == "div" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "b_caption") {
						inCaption = true
					}
				}
			}
			if inAlgo && !inCaption && n.Data == "a" && currentResult.URL == "" {
				for _, attr := range n.Attr {
					if attr.Key == "href" && attr.Val != "" && !strings.HasPrefix(attr.Val, "javascript:") {
						currentResult.URL = attr.Val
						break
					}
				}
			}
		}

		if n.Type == html.TextNode && inAlgo {
			text := strings.TrimSpace(n.Data)
			if text == "" {
				goto next
			}
			if inH2 && currentResult.Title == "" {
				currentResult.Title = text
			}
			if inCaption {
				if currentResult.Snippet != "" {
					currentResult.Snippet += " "
				}
				currentResult.Snippet += text
			}
		}

	next:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if skipChildren {
				continue
			}
			if inAlgo && !inCaption && (n.Data == "span" || n.Data == "cite" || n.Data == "div") {
				for _, attr := range n.Attr {
					if attr.Key == "class" && (strings.Contains(attr.Val, "news_dt") || strings.Contains(attr.Val, "news_meta")) {
						skipChildren = true
						break
					}
				}
				if skipChildren {
					continue
				}
			}
			walk(c)
		}

		if n.Type == html.ElementNode {
			if inAlgo && n.Data == "h2" {
				inH2 = false
			}
			if inAlgo && n.Data == "li" {
				if currentResult.Title != "" || currentResult.Snippet != "" {
					if !strings.HasPrefix(currentResult.URL, "http") {
						currentResult.URL = "https://cn.bing.com" + currentResult.URL
					}
					results = append(results, currentResult)
				}
				inAlgo = false
				inCaption = false
			}
		}
	}

	walk(doc)
	return results
}

// ---- 百度搜索 ----

// NewBaiduSearcher 创建百度搜索器
func NewBaiduSearcher(timeout time.Duration, userAgent string) *BaiduSearcher {
	return &BaiduSearcher{
		client: &http.Client{Timeout: timeout},
	}
}

func (b *BaiduSearcher) Name() string { return "Baidu" }

func (b *BaiduSearcher) Search(query string, limit int) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://www.baidu.com/s?wd=%s&ie=utf-8&rn=%d",
		url.QueryEscape(query), limit)

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建百度请求失败: %w", err)
	}
	req.Header.Set("User-Agent", defaultConfig.HTTP.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求百度搜索失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("百度搜索返回异常状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取百度响应失败: %w", err)
	}

	return extractBaiduResults(string(body), limit), nil
}

func (b *BaiduSearcher) SearchRaw(query string, limit int) ([]SearchResult, error) {
	return b.Search(query, limit) // 百度无预处理，Search 即原始搜索
}

func extractBaiduResults(htmlStr string, limit int) []SearchResult {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}

	var results []SearchResult
	var inResult bool
	var inTitle bool
	var inAbstract bool
	var currentResult SearchResult
	var currentURL string

	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if len(results) >= limit {
			return
		}

		if n.Type == html.ElementNode {
			// 百度结果容器：div.result 或 div.c-container
			if n.Data == "div" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && (strings.Contains(attr.Val, "result") ||
						strings.Contains(attr.Val, "c-container")) {
						inResult = true
						currentResult = SearchResult{}
						currentURL = ""
						inTitle = false
						inAbstract = false
						break
					}
				}
			}
			// 标题：h3 内的 a 标签
			if inResult && n.Data == "h3" {
				inTitle = true
			}
			// 摘要：div.c-abstract 或 span.content-right_*
			if inResult && n.Data == "div" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "c-abstract") {
						inAbstract = true
					}
				}
			}
			if inResult && n.Data == "span" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "content-right") {
						inAbstract = true
					}
				}
			}
			// URL：a 标签的 href
			if inResult && n.Data == "a" && currentURL == "" {
				for _, attr := range n.Attr {
					if attr.Key == "href" && attr.Val != "" &&
						!strings.HasPrefix(attr.Val, "javascript:") &&
						!strings.HasPrefix(attr.Val, "#") {
						currentURL = attr.Val
						break
					}
				}
			}
		}

		if n.Type == html.TextNode && inResult {
			text := strings.TrimSpace(n.Data)
			if text == "" {
				goto baiduNext
			}
			if inTitle && currentResult.Title == "" {
				currentResult.Title = text
			}
			if inAbstract {
				if currentResult.Snippet != "" {
					currentResult.Snippet += " "
				}
				currentResult.Snippet += text
			}
		}

	baiduNext:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}

		if n.Type == html.ElementNode {
			if inResult && n.Data == "h3" {
				inTitle = false
			}
			if inResult && n.Data == "div" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && (strings.Contains(attr.Val, "result") ||
						strings.Contains(attr.Val, "c-container")) {
						if currentResult.Title != "" || currentResult.Snippet != "" {
							if !strings.HasPrefix(currentURL, "http") {
								currentURL = "https://www.baidu.com" + currentURL
							}
							currentResult.URL = currentURL
							results = append(results, currentResult)
						}
						inResult = false
						inAbstract = false
						break
					}
				}
			}
		}
	}

	walk(doc)
	return results
}

// ---- 搜狗搜索 ----

// NewSogouSearcher 创建搜狗搜索器
func NewSogouSearcher(timeout time.Duration, userAgent string) *SogouSearcher {
	return &SogouSearcher{
		client: &http.Client{Timeout: timeout},
	}
}

func (s *SogouSearcher) Name() string { return "Sogou" }

func (s *SogouSearcher) Search(query string, limit int) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://www.sogou.com/web?query=%s&ie=utf8&num=%d",
		url.QueryEscape(query), limit)

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建搜狗请求失败: %w", err)
	}
	req.Header.Set("User-Agent", defaultConfig.HTTP.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求搜狗搜索失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("搜狗搜索返回异常状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取搜狗响应失败: %w", err)
	}

	return extractSogouResults(string(body), limit), nil
}

func (s *SogouSearcher) SearchRaw(query string, limit int) ([]SearchResult, error) {
	return s.Search(query, limit) // 搜狗无预处理，Search 即原始搜索
}

func extractSogouResults(htmlStr string, limit int) []SearchResult {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}

	var results []SearchResult
	var inResult bool
	var inTitle bool
	var inAbstract bool
	var currentResult SearchResult
	var currentURL string

	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if len(results) >= limit {
			return
		}

		if n.Type == html.ElementNode {
			// 搜狗结果容器：div.rb 或 div.vrwrap
			if n.Data == "div" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && (attr.Val == "rb" || attr.Val == "vrwrap" ||
						strings.HasPrefix(attr.Val, "rb ") || strings.HasPrefix(attr.Val, "vrwrap ")) {
						inResult = true
						currentResult = SearchResult{}
						currentURL = ""
						inTitle = false
						inAbstract = false
						break
					}
				}
			}
			// 标题：h3.pt 或 h3.vrTitle
			if inResult && n.Data == "h3" {
				inTitle = true
			}
			// 摘要：div.space-txt 或 div.str-text 或 div.ft
			if inResult && n.Data == "div" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && (strings.Contains(attr.Val, "space-txt") ||
						strings.Contains(attr.Val, "str-text") || strings.Contains(attr.Val, "ft")) {
						inAbstract = true
					}
				}
			}
			// 摘要也可以出现在 p 标签中
			if inResult && n.Data == "p" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && (strings.Contains(attr.Val, "str_info") ||
						strings.Contains(attr.Val, "star-wiki")) {
						inAbstract = true
					}
				}
			}
			// URL：a 标签的 href
			if inResult && n.Data == "a" && currentURL == "" {
				for _, attr := range n.Attr {
					if attr.Key == "href" && attr.Val != "" &&
						!strings.HasPrefix(attr.Val, "javascript:") &&
						!strings.HasPrefix(attr.Val, "#") {
						currentURL = attr.Val
						break
					}
				}
			}
		}

		if n.Type == html.TextNode && inResult {
			text := strings.TrimSpace(n.Data)
			if text == "" {
				goto sogouNext
			}
			if inTitle && currentResult.Title == "" {
				currentResult.Title = text
			}
			if inAbstract {
				if currentResult.Snippet != "" {
					currentResult.Snippet += " "
				}
				currentResult.Snippet += text
			}
		}

	sogouNext:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}

		if n.Type == html.ElementNode {
			if inResult && n.Data == "h3" {
				inTitle = false
			}
			if inResult && n.Data == "div" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && (attr.Val == "rb" || attr.Val == "vrwrap" ||
						strings.HasPrefix(attr.Val, "rb ") || strings.HasPrefix(attr.Val, "vrwrap ")) {
						if currentResult.Title != "" || currentResult.Snippet != "" {
							if !strings.HasPrefix(currentURL, "http") {
								currentURL = "https://www.sogou.com" + currentURL
							}
							currentResult.URL = currentURL
							results = append(results, currentResult)
						}
						inResult = false
						inAbstract = false
						break
					}
				}
			}
		}
	}

	walk(doc)
	return results
}

// ---- DuckDuckGo ----

// NewDuckDuckGoSearcher 创建 DuckDuckGo 搜索器
func NewDuckDuckGoSearcher(timeout time.Duration, userAgent string) *DuckDuckGoSearcher {
	return &DuckDuckGoSearcher{
		client: &http.Client{Timeout: timeout},
	}
}

func (d *DuckDuckGoSearcher) Name() string { return "DuckDuckGo" }

func (d *DuckDuckGoSearcher) SearchRaw(query string, limit int) ([]SearchResult, error) {
	return d.Search(query, limit) // DDG 无预处理，Search 即原始搜索
}

func (d *DuckDuckGoSearcher) Search(query string, limit int) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://lite.duckduckgo.com/lite/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 DuckDuckGo 请求失败: %w", err)
	}
	req.Header.Set("User-Agent", defaultConfig.HTTP.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 DuckDuckGo 搜索失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DuckDuckGo 搜索返回异常状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 DuckDuckGo 响应失败: %w", err)
	}

	return extractDDGResults(string(body), limit), nil
}

func extractDDGResults(htmlStr string, limit int) []SearchResult {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}

	var results []SearchResult
	var inRow bool
	var inLink bool
	var currentResult SearchResult
	var currentURL string

	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if len(results) >= limit {
			return
		}

		if n.Type == html.ElementNode {
			if n.Data == "tr" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "result-snippet") {
						inRow = true
						currentResult = SearchResult{}
						currentURL = ""
						walk(n.FirstChild)
						if currentResult.Title != "" || currentResult.Snippet != "" {
							if !strings.HasPrefix(currentURL, "http") {
								currentURL = "https:" + currentURL
							}
							currentResult.URL = currentURL
							results = append(results, currentResult)
						}
						inRow = false
						return
					}
				}
			}
			if inRow && n.Data == "a" && currentResult.Title == "" {
				inLink = true
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						currentURL = attr.Val
						break
					}
				}
			}
		}

		if n.Type == html.TextNode && inRow {
			text := strings.TrimSpace(n.Data)
			if text == "" {
				goto ddgNext
			}
			if inLink && currentResult.Title == "" {
				currentResult.Title = text
			} else if !inLink {
				if currentResult.Snippet != "" {
					currentResult.Snippet += " "
				}
				currentResult.Snippet += text
			}
		}

	ddgNext:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}

		if n.Type == html.ElementNode {
			if inRow && n.Data == "a" {
				inLink = false
			}
		}
	}

	walk(doc)
	return results
}

// ---- 通用工具 ----

// extractTextContent 从 HTML 中提取正文文本
func extractTextContent(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return ""
	}

	var sb strings.Builder
	skipTags := map[string]bool{
		"script": true, "style": true, "noscript": true,
		"nav": true, "footer": true, "header": true, "iframe": true,
	}

	var walk func(n *html.Node, inSkip bool)
	walk = func(n *html.Node, inSkip bool) {
		if n.Type == html.ElementNode {
			if skipTags[n.Data] {
				inSkip = true
			}
			if n.Data == "article" || n.Data == "main" {
				inSkip = false
			}
		}
		if n.Type == html.TextNode && !inSkip {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				sb.WriteString(text)
				sb.WriteString(" ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inSkip)
		}
	}

	walk(doc, false)
	return strings.TrimSpace(sb.String())
}

// truncateText 截断文本到指定长度
func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "..."
}
