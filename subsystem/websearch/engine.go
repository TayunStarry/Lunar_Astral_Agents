package websearch

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// NewBingSearcher 创建必应搜索器
func NewBingSearcher(cfg HTTPConfig) *BingSearcher {
	return &BingSearcher{
		client:  &http.Client{Timeout: cfg.Timeout},
		httpCfg: cfg,
	}
}

func (b *BingSearcher) Name() string { return "Bing" }

func (b *BingSearcher) Search(query string, limit int) ([]SearchResult, error) {
	processedQuery := preprocessBingQuery(query)
	searchURL := fmt.Sprintf("https://cn.bing.com/search?q=%s&mkt=zh-CN&setlang=zh-hans",
		url.QueryEscape(processedQuery))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", b.httpCfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	setBrowserHeaders(req)

	resp, err := doWithRetry(b.client, req, b.httpCfg)
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
	searchURL := fmt.Sprintf("https://cn.bing.com/search?q=%s&mkt=zh-CN&setlang=zh-hans",
		url.QueryEscape(query))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", b.httpCfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	setBrowserHeaders(req)

	resp, err := doWithRetry(b.client, req, b.httpCfg)
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

	// 含特殊分隔符的查询：加引号做精确匹配，避免被拆成单字匹配到无关内容
	hasSpecial := false
	for _, r := range query {
		if r == '·' || r == '-' || r == '/' || r == '|' || r == '•' ||
			r == '\u2014' || r == '～' || r == '~' || r == '、' {
			hasSpecial = true
			break
		}
	}
	if hasSpecial {
		return "\"" + query + "\""
	}

	// 其他查询原样返回，让 Bing 自然分词
	// 加引号会触发精确短语匹配，对含修饰词的查询（如"超自然最近更新"）有害
	return query
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
			// 只提取h2标签内的链接作为搜索结果URL，避免提取网站卡片中的链接
			if inAlgo && inH2 && n.Data == "a" && currentResult.URL == "" {
				for _, attr := range n.Attr {
					if attr.Key == "href" && attr.Val != "" && !strings.HasPrefix(attr.Val, "javascript:") && !strings.HasPrefix(attr.Val, "#") {
						currentResult.URL = attr.Val
						break
					}
				}
			}
			// 检测官方标记：Bing 通常在标题附近的 span 或 div 中显示"官方"文字
			if inAlgo && n.Data == "span" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && (strings.Contains(attr.Val, "b_official") || strings.Contains(attr.Val, "label") || strings.Contains(attr.Val, "badge")) {
						// 检查子节点是否包含"官方"文字
						if hasOfficialText(n) {
							currentResult.IsOfficial = true
						}
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
				// 检测标题中的官方标记：所有搜索引擎都会在官方网站标题中包含"官方网站"或"Official Site"
				if strings.Contains(text, "官方网站") || strings.Contains(strings.ToLower(text), "official site") {
					currentResult.IsOfficial = true
				}
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
func NewBaiduSearcher(cfg HTTPConfig) *BaiduSearcher {
	jar, _ := cookiejar.New(nil)
	return &BaiduSearcher{
		client: &http.Client{
			Timeout: cfg.Timeout,
			Jar:     jar,
		},
		httpCfg: cfg,
	}
}

func (b *BaiduSearcher) Name() string { return "Baidu" }

// warmupCookie 访问百度首页获取 BAIDUID Cookie，降低后续搜索触发 CAPTCHA 的概率
func (b *BaiduSearcher) warmupCookie() {
	b.cookieWarmed = true
	req, _ := http.NewRequest("GET", "https://www.baidu.com/", nil)
	req.Header.Set("User-Agent", b.httpCfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	// 忽略预热请求的结果，只关心 Cookie 是否被设置
	resp, err := b.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

func (b *BaiduSearcher) Search(query string, limit int) ([]SearchResult, error) {
	// 如果已检测到 CAPTCHA 且距上次检测超过 5 分钟，尝试恢复 HTTP 请求
	if b.captchaDetected && time.Since(b.captchaDetectedAt) > 5*time.Minute {
		b.captchaDetected = false
	}
	// 已知被 CAPTCHA 拦截，跳过 HTTP 请求直接用浏览器
	if b.captchaDetected && b.browserRenderer != nil {
		return b.searchWithBrowser(query, limit)
	}

	// 首次搜索前预热Cookie：访问百度首页获取BAIDUID，降低CAPTCHA触发概率
	if !b.cookieWarmed {
		b.warmupCookie()
	}

	searchURL := fmt.Sprintf("https://www.baidu.com/s?wd=%s&ie=utf-8&rn=%d",
		url.QueryEscape(query), limit)

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建百度请求失败: %w", err)
	}
	req.Header.Set("User-Agent", b.httpCfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Referer", "https://www.baidu.com/")
	setBrowserHeaders(req)

	resp, err := doWithRetry(b.client, req, b.httpCfg)
	if err != nil {
		// HTTP 请求失败 → 标记 CAPTCHA 并回退到浏览器
		b.captchaDetected = true
		b.captchaDetectedAt = time.Now()
		if b.browserRenderer != nil {
			return b.searchWithBrowser(query, limit)
		}
		return nil, fmt.Errorf("请求百度搜索失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// HTTP 返回异常状态码 → 标记 CAPTCHA 并回退到浏览器
		b.captchaDetected = true
		b.captchaDetectedAt = time.Now()
		if b.browserRenderer != nil {
			return b.searchWithBrowser(query, limit)
		}
		return nil, fmt.Errorf("百度搜索返回异常状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取百度响应失败: %w", err)
	}

	results := extractBaiduResults(string(body), limit)

	// HTTP 返回空结果 → 标记 CAPTCHA（可能被拦截但没报错）并回退到浏览器
	if len(results) == 0 {
		b.captchaDetected = true
		b.captchaDetectedAt = time.Now()
		if b.browserRenderer != nil {
			return b.searchWithBrowser(query, limit)
		}
	}

	return results, nil
}

// searchWithBrowser 使用无头浏览器渲染百度搜索页面并提取结果
// 作为 HTTP 请求被 CAPTCHA 拦截时的回退方案
func (b *BaiduSearcher) searchWithBrowser(query string, limit int) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://www.baidu.com/s?wd=%s&ie=utf-8&rn=%d",
		url.QueryEscape(query), limit)

	html, err := b.browserRenderer.Render(searchURL)
	if err != nil {
		return nil, fmt.Errorf("浏览器渲染百度搜索失败: %w", err)
	}

	results := extractBaiduResults(html, limit)
	if len(results) == 0 {
		return nil, fmt.Errorf("浏览器渲染百度搜索无结果（可能被反爬拦截）")
	}

	return results, nil
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
					// 检测官方标记：百度在标题旁用特殊样式显示"官方"认证
					if attr.Key == "class" && (strings.Contains(attr.Val, "label") || strings.Contains(attr.Val, "badge") ||
						strings.Contains(attr.Val, "official") || strings.Contains(attr.Val, "icon") ||
						strings.Contains(attr.Val, "cert") || strings.Contains(attr.Val, "brand_tip")) {
						if hasOfficialText(n) {
							currentResult.IsOfficial = true
						}
					}
				}
			}
			// URL：只提取标题标签内的 a 标签 href，避免提取卡片链接
			if inResult && inTitle && n.Data == "a" && currentURL == "" {
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
				// 检测标题中的官方标记：所有搜索引擎都会在官方网站标题中包含"官方网站"或"Official Site"
				if strings.Contains(text, "官方网站") || strings.Contains(strings.ToLower(text), "official site") {
					currentResult.IsOfficial = true
				}
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
func NewSogouSearcher(cfg HTTPConfig) *SogouSearcher {
	return &SogouSearcher{
		client:  &http.Client{Timeout: cfg.Timeout},
		httpCfg: cfg,
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
	req.Header.Set("User-Agent", s.httpCfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	setBrowserHeaders(req)

	resp, err := doWithRetry(s.client, req, s.httpCfg)
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
			// 检测官方标记：搜狗在标题旁显示"官方"认证
			if inResult && n.Data == "span" {
				for _, attr := range n.Attr {
					if attr.Key == "class" && (strings.Contains(attr.Val, "label") || strings.Contains(attr.Val, "badge") ||
						strings.Contains(attr.Val, "official") || strings.Contains(attr.Val, "icon")) {
						if hasOfficialText(n) {
							currentResult.IsOfficial = true
						}
					}
				}
			}
			// URL：只提取标题标签内的 a 标签 href，避免提取卡片链接
			if inResult && inTitle && n.Data == "a" && currentURL == "" {
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
				// 检测标题中的官方标记：所有搜索引擎都会在官方网站标题中包含"官方网站"或"Official Site"
				if strings.Contains(text, "官方网站") || strings.Contains(strings.ToLower(text), "official site") {
					currentResult.IsOfficial = true
				}
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
func NewDuckDuckGoSearcher(cfg HTTPConfig) *DuckDuckGoSearcher {
	return &DuckDuckGoSearcher{
		client:  &http.Client{Timeout: cfg.Timeout},
		httpCfg: cfg,
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
	req.Header.Set("User-Agent", d.httpCfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	setBrowserHeaders(req)

	resp, err := doWithRetry(d.client, req, d.httpCfg)
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

// hasOfficialText 检查节点及其子节点是否包含"官方"文字
func hasOfficialText(n *html.Node) bool {
	if n.Type == html.TextNode {
		return strings.Contains(strings.TrimSpace(n.Data), "官方")
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if hasOfficialText(c) {
			return true
		}
	}
	return false
}

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

// setBrowserHeaders 设置现代浏览器的标配请求头，降低被搜索引擎反爬/CAPTCHA 的概率
func setBrowserHeaders(req *http.Request) {
	req.Header.Set("Sec-Ch-Ua", `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Cache-Control", "max-age=0")
}

// ---- 链接提取与跟踪 ----

// pageLink 页面中提取到的链接
type pageLink struct {
	URL  string
	Text string
}

// extractPageLinks 从 HTML 中提取所有链接及其可见文本
// 跳过导航、页脚、javascript等非内容链接
func extractPageLinks(htmlStr string) []pageLink {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}

	skipParents := map[string]bool{
		"nav": true, "footer": true, "header": true,
		"script": true, "style": true, "noscript": true,
	}

	var links []pageLink
	var walk func(n *html.Node, inSkip bool)
	walk = func(n *html.Node, inSkip bool) {
		if n.Type == html.ElementNode {
			if skipParents[n.Data] {
				inSkip = true
			}
			if n.Data == "a" && !inSkip {
				var href string
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						href = attr.Val
						break
					}
				}
				// 跳过空链接、锚点、javascript
				if href == "" || strings.HasPrefix(href, "#") ||
					strings.HasPrefix(href, "javascript:") {
					goto nextLink
				}
				// 跳过明显非内容的链接
				hrefLower := strings.ToLower(href)
				skipPatterns := []string{
					"login", "logout", "signin", "signup", "register",
					"mailto:", "tel:", "javascript",
				}
				skip := false
				for _, p := range skipPatterns {
					if strings.Contains(hrefLower, p) {
						skip = true
						break
					}
				}
				if skip {
					goto nextLink
				}
				// 提取链接文本
				text := strings.TrimSpace(extractTextContent(renderNode(n)))
				if text != "" && len([]rune(text)) >= 2 {
					links = append(links, pageLink{URL: href, Text: text})
				}
			}
		}
	nextLink:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inSkip)
		}
	}
	walk(doc, false)
	return links
}

// renderNode 将节点渲染为 HTML 字符串（用于提取链接文本）
func renderNode(n *html.Node) string {
	var sb strings.Builder
	_ = html.Render(&sb, n)
	return sb.String()
}

// isThinContent 判断提取的正文是否「太薄」——碎片化的列表页而非完整文章
// 条件：换行数 >= 5 且 平均每行 < 80 字符
func isThinContent(text string) bool {
	lines := strings.Split(text, "\n")
	realLines := 0
	totalLen := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 0 {
			realLines++
			totalLen += len([]rune(trimmed))
		}
	}
	if realLines < 5 {
		return false
	}
	avgLen := totalLen / realLines
	return avgLen < 80
}

// selectBestLinks 按链接文本与查询关键词的匹配度排序，返回 top N 个 URL
// baseURL 用于将相对路径转为绝对路径
func selectBestLinks(links []pageLink, queryKeywords []string, baseURL string, maxLinks int) []string {
	if len(links) == 0 || len(queryKeywords) == 0 {
		return nil
	}

	type scored struct {
		url   string
		score int
	}
	var scoredLinks []scored

	for _, link := range links {
		linkText := strings.ToLower(link.Text)
		score := 0
		for _, kw := range queryKeywords {
			kwLower := strings.ToLower(kw)
			if strings.Contains(linkText, kwLower) {
				score += 2 // 精确匹配加分
			}
			// 2-gram 子串匹配
			linkRunes := []rune(linkText)
			kwRunes := []rune(kwLower)
			if len(kwRunes) >= 2 {
				for i := 0; i <= len(linkRunes)-2; i++ {
					bigram := string(linkRunes[i : i+2])
					if strings.Contains(kwLower, bigram) {
						score++
					}
				}
			}
		}
		if score > 0 {
			// 解析 URL：相对路径转绝对路径
			resolvedURL := resolveURL(link.URL, baseURL)
			// 跳过外链（安全考虑：不跟踪到完全不同域名的页面）
			if !isExternalLink(resolvedURL, baseURL) {
				scoredLinks = append(scoredLinks, scored{url: resolvedURL, score: score})
			}
		}
	}

	// 按分数降序排序
	for i := 0; i < len(scoredLinks); i++ {
		for j := i + 1; j < len(scoredLinks); j++ {
			if scoredLinks[j].score > scoredLinks[i].score {
				scoredLinks[i], scoredLinks[j] = scoredLinks[j], scoredLinks[i]
			}
		}
	}

	// 去重并取 top N
	seen := make(map[string]bool)
	var result []string
	for _, s := range scoredLinks {
		if seen[s.url] {
			continue
		}
		seen[s.url] = true
		result = append(result, s.url)
		if len(result) >= maxLinks {
			break
		}
	}
	return result
}

// resolveURL 将相对路径解析为绝对 URL
func resolveURL(href, baseURL string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

// isExternalLink 判断目标 URL 是否与基础 URL 不同域名
func isExternalLink(targetURL, baseURL string) bool {
	t, err := url.Parse(targetURL)
	if err != nil {
		return true
	}
	b, err := url.Parse(baseURL)
	if err != nil {
		return true
	}
	return t.Host != b.Host
}
