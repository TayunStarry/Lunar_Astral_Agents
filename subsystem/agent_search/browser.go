package AgentSearch

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"
)

// =============================================================================
// 浏览器生命周期管理
// =============================================================================

// LaunchBrowser 启动 Chromium 浏览器实例
// 使用默认的 headless 配置，创建全局浏览器上下文供后续操作复用
func LaunchBrowser() error {
	browserMutex.Lock()
	defer browserMutex.Unlock()

	if browserLaunched {
		return nil
	}

	// 自动检测浏览器路径
	browserPath := detectBrowserPath()
	if browserPath != "" {
		BrowserExecPath = browserPath
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), buildBrowserOpts()...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	// 启动浏览器并验证可用性
	if err := chromedp.Run(ctx); err != nil {
		allocCancel()
		cancel()
		return fmt.Errorf("浏览器启动失败: %w", err)
	}

	browserCtx = ctx
	browserCancel = cancel
	browserAllocCancel = allocCancel
	browserLaunched = true
	browserJustLaunched = true // 标记刚启动，跳过下次健康检查
	browserQueryCount = 0

	fmt.Printf("[%s] 浏览器已启动\n", ModuleName)
	return nil
}

// CloseBrowser 关闭浏览器实例，释放资源
func CloseBrowser() {
	browserMutex.Lock()
	defer browserMutex.Unlock()

	if !browserLaunched {
		return
	}

	if browserCancel != nil {
		browserCancel()
	}
	if browserAllocCancel != nil {
		browserAllocCancel()
	}

	browserCtx = nil
	browserCancel = nil
	browserAllocCancel = nil
	browserLaunched = false
	browserQueryCount = 0

	fmt.Printf("[%s] 浏览器已关闭\n", ModuleName)
}

// ensureBrowser 确保浏览器已启动，未启动则自动启动
func ensureBrowser() error {
	browserMutex.Lock()
	if browserLaunched {
		browserMutex.Unlock()
		return nil
	}
	browserMutex.Unlock()
	return LaunchBrowser()
}

// withBrowserRetry 执行浏览器操作，失败时清理资源并重启浏览器后重试（最多 MaxBrowserRetryAttempts 次）
// fn 每次重试都会重新执行，因此其内部必须每次都重新从 browserCtx 创建上下文，以使用重启后的浏览器
// 仅当前置浏览器可用但运行期加载/导航失败这类可恢复场景触发重试；从根上无法启动时由调用方透传错误
func withBrowserRetry(operationName string, fn func() error) error {
	var lastErr error
	for attempt := 1; attempt <= MaxBrowserRetryAttempts; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		fmt.Printf("[%s] %s 失败 (第 %d/%d 次): %v\n",
			ModuleName, operationName, attempt, MaxBrowserRetryAttempts, lastErr)

		if attempt < MaxBrowserRetryAttempts {
			fmt.Printf("[%s] 清理资源并重启浏览器后重试...\n", ModuleName)
			if restartErr := tryRestartBrowser(); restartErr != nil {
				fmt.Printf("[%s] 浏览器重启失败: %v\n", ModuleName, restartErr)
			}
		}
	}
	return lastErr
}

// buildBrowserOpts 构建 chromedp 浏览器启动选项
func buildBrowserOpts() []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-translate", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("disable-features", "TranslateUI"),
		chromedp.WindowSize(1920, 1080),
	}

	if BrowserExecPath != "" {
		opts = append(opts, chromedp.ExecPath(BrowserExecPath))
	}

	return opts
}

// detectBrowserPath 自动检测可用的浏览器
// 优先级：用户指定 > Edge > Chrome（默认）
func detectBrowserPath() string {
	if BrowserExecPath != "" {
		fmt.Printf("[%s] 使用用户指定的浏览器: %s\n", ModuleName, BrowserExecPath)
		return BrowserExecPath
	}

	// 尝试检测 Edge
	for _, p := range edgePaths {
		if _, err := os.Stat(p); err == nil {
			fmt.Printf("[%s] 自动检测到 Edge 浏览器: %s\n", ModuleName, p)
			return p
		}
	}

	// 未检测到，让 chromedp 使用默认 Chrome 查找逻辑
	fmt.Printf("[%s] 未检测到 Edge，使用默认浏览器查找逻辑\n", ModuleName)
	return ""
}

// =============================================================================
// 搜索执行器 — 搜索引擎降级链
// =============================================================================

// ExecuteSearch 在指定搜索引擎上执行搜索，返回搜索结果列表
// 按 engineFallbackOrder 顺序尝试，首个返回非空结果的引擎即为有效引擎
// maxResults 控制每个搜索引擎返回的最大结果数
func ExecuteSearch(query string, maxResults int) ([]SearchResult, error) {
	if err := ensureBrowser(); err != nil {
		return nil, err
	}

	var lastErr error
	for _, engine := range engineFallbackOrder {
		fmt.Printf("[%s] 尝试使用 %s 搜索: %s\n", ModuleName, engine, query)

		results, err := searchOnEngine(engine, query, maxResults)
		if err != nil {
			fmt.Printf("[%s] %s 搜索失败: %v\n", ModuleName, engine, err)
			lastErr = err
			continue
		}

		if len(results) > 0 {
			fmt.Printf("[%s] %s 返回 %d 条结果\n", ModuleName, engine, len(results))
			return results, nil
		}

		fmt.Printf("[%s] %s 无搜索结果，尝试下一个引擎\n", ModuleName, engine)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("所有搜索引擎均失败，最后错误: %w", lastErr)
	}
	return nil, fmt.Errorf("所有搜索引擎均无结果")
}

// searchOnEngine 在单个搜索引擎上执行搜索
func searchOnEngine(engine string, query string, maxResults int) ([]SearchResult, error) {
	searchURL := searchEngineURLs[engine] + url.QueryEscape(query)

	var pageHTML string
	var currentURL string
	err := withBrowserRetry(engine+" 搜索", func() error {
		browserMutex.Lock()
		browserQueryCount++
		ctx := browserCtx
		browserMutex.Unlock()

		// 创建带超时的搜索上下文
		searchCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
		defer cancel()

		// 导航到搜索结果页并等待加载
		currentURL = ""
		if err := chromedp.Run(searchCtx,
			chromedp.Navigate(searchURL),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second), // 等待动态内容渲染
			chromedp.Location(&currentURL),
		); err != nil {
			return fmt.Errorf("导航到搜索页失败: %w", err)
		}

		fmt.Printf("[%s] 搜索页加载完成: %s\n", ModuleName, currentURL)

		// 获取页面 HTML
		pageHTML = ""
		if err := chromedp.Run(searchCtx,
			chromedp.OuterHTML("html", &pageHTML, chromedp.ByQuery),
		); err != nil {
			return fmt.Errorf("获取页面 HTML 失败: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 根据引擎选择解析器
	switch engine {
	case "bing":
		return parseBingResults(pageHTML, maxResults), nil
	case "baidu":
		return parseBaiduResults(pageHTML, maxResults), nil
	case "sogou":
		return parseSogouResults(pageHTML, maxResults), nil
	default:
		return nil, fmt.Errorf("不支持的搜索引擎: %s", engine)
	}
}

// =============================================================================
// 搜索引擎 HTML 解析器
// =============================================================================

// parseBingResults 解析 Bing 搜索结果页 HTML
// Bing 结果结构：<li class="b_algo"> → <h2><a href="...">标题</a></h2> + <p>摘要</p>
func parseBingResults(pageHTML string, maxResults int) []SearchResult {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil
	}

	var results []SearchResult

	// 查找 class="b_algo" 的 <li> 元素
	algoNodes := findNodesByClass(doc, "li", "b_algo")
	for _, algo := range algoNodes {
		result := SearchResult{Engine: "bing"}

		// 提取标题和 URL：<h2> 内的 <a>
		h2Nodes := findTags(algo, "h2")
		for _, h2 := range h2Nodes {
			aNodes := findTags(h2, "a")
			for _, a := range aNodes {
				result.Title = extractTextContent(a)
				for _, attr := range a.Attr {
					if attr.Key == "href" {
						result.URL = attr.Val
						break
					}
				}
				break
			}
			break
		}

		// 提取摘要：<p> 标签内的文本
		pNodes := findTags(algo, "p")
		for _, p := range pNodes {
			text := strings.TrimSpace(extractTextContent(p))
			if text != "" {
				result.Snippet = text
				break
			}
		}

		if result.Title != "" && result.URL != "" {
			results = append(results, result)
			if len(results) >= maxResults {
				break
			}
		}
	}

	return results
}

// parseBaiduResults 解析百度搜索结果页 HTML
// 百度结果结构：<div class="result c-container"> → <h3 class="t"><a>标题</a></h3> + <span class="content-right_*">摘要</span>
func parseBaiduResults(pageHTML string, maxResults int) []SearchResult {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil
	}

	var results []SearchResult

	// 查找 class 包含 "result" 的 <div> 元素
	resultNodes := findNodesByPartialClass(doc, "div", "result")
	for _, node := range resultNodes {
		result := SearchResult{Engine: "baidu"}

		// 提取标题和 URL：<h3> 内的 <a>
		h3Nodes := findTags(node, "h3")
		for _, h3 := range h3Nodes {
			aNodes := findTags(h3, "a")
			for _, a := range aNodes {
				result.Title = extractTextContent(a)
				for _, attr := range a.Attr {
					if attr.Key == "href" {
						result.URL = attr.Val
						break
					}
				}
				break
			}
			break
		}

		// 提取摘要：查找 class 包含 "content" 或 "c-abstract" 的元素
		snippetNodes := findNodesByPartialClass(node, "span", "content")
		if len(snippetNodes) == 0 {
			snippetNodes = findNodesByClass(node, "div", "c-abstract")
		}
		for _, sn := range snippetNodes {
			text := strings.TrimSpace(extractTextContent(sn))
			if text != "" {
				result.Snippet = text
				break
			}
		}

		if result.Title != "" && result.URL != "" {
			results = append(results, result)
			if len(results) >= maxResults {
				break
			}
		}
	}

	return results
}

// parseSogouResults 解析搜狗搜索结果页 HTML
// 搜狗结果结构：<div class="rb"> 或 <div class="vrwrap"> → <h3 class="vrTitle"><a>标题</a></h3> + <p>摘要</p>
func parseSogouResults(pageHTML string, maxResults int) []SearchResult {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil
	}

	var results []SearchResult

	// 查找结果容器
	var resultNodes []*html.Node
	resultNodes = append(resultNodes, findNodesByClass(doc, "div", "rb")...)
	resultNodes = append(resultNodes, findNodesByClass(doc, "div", "vrwrap")...)

	for _, node := range resultNodes {
		result := SearchResult{Engine: "sogou"}

		// 提取标题和 URL：<h3> 内 <a>
		h3Nodes := findTags(node, "h3")
		for _, h3 := range h3Nodes {
			aNodes := findTags(h3, "a")
			for _, a := range aNodes {
				result.Title = extractTextContent(a)
				for _, attr := range a.Attr {
					if attr.Key == "href" {
						result.URL = attr.Val
						break
					}
				}
				break
			}
			break
		}

		// 提取摘要：<p> 或 <div class="str-text">
		pNodes := findTags(node, "p")
		if len(pNodes) == 0 {
			pNodes = findNodesByClass(node, "div", "str-text")
		}
		for _, p := range pNodes {
			text := strings.TrimSpace(extractTextContent(p))
			if text != "" {
				result.Snippet = text
				break
			}
		}

		if result.Title != "" && result.URL != "" {
			results = append(results, result)
			if len(results) >= maxResults {
				break
			}
		}
	}

	return results
}

// =============================================================================
// 网页内容提取
// =============================================================================

// ExtractPageContent 导航到指定 URL 并提取页面内容
// 先提取 DOM 文本，根据文本量判定为 text 或 visual 类型
// text 类型（≥500 字）：清洗后直接返回文本
// visual 类型（<500 字）：调用分页截图获取视觉内容
func ExtractPageContent(targetURL string) (*PageContent, error) {
	if err := ensureBrowser(); err != nil {
		return nil, err
	}

	browserMutex.Lock()
	ctx := browserCtx
	browserMutex.Unlock()

	// 导航到目标页面：单次 10 秒超时，打不开直接跳过（不重启浏览器重试，避免对无法访问的站点反复浪费重试时间）
	var finalURL string
	loadCtx, cancel := context.WithTimeout(ctx, PageFastSkipTimeout)
	defer cancel()

	finalURL = ""
	if err := chromedp.Run(loadCtx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(1*time.Second), // 等待懒加载和动态内容
		chromedp.Location(&finalURL),
	); err != nil {
		return nil, fmt.Errorf("页面加载超时/失败 %s: %w", targetURL, err)
	}
	fmt.Printf("[%s] 页面加载完成: %s\n", ModuleName, finalURL)

	// 提取 DOM 文本
	rawText, err := extractDOMText(ctx)
	if err != nil {
		return nil, fmt.Errorf("提取 DOM 文本失败: %w", err)
	}

	// 清洗文本
	cleanedText := cleanDOMText(rawText)
	textLen := len([]rune(cleanedText))

	content := &PageContent{
		URL:         finalURL,
		TextContent: cleanedText,
		TextLength:  textLen,
	}

	if textLen >= TextHeavyThreshold {
		// 文本密集型：使用 DOM 文本
		content.ContentType = "text"
		fmt.Printf("[%s] 判定为文本密集型 (%d 字) %s\n", ModuleName, textLen, finalURL)
	} else {
		// 视觉主导型：获取截图
		content.ContentType = "visual"
		fmt.Printf("[%s] 判定为视觉主导型 (%d 字)，开始截图 %s\n", ModuleName, textLen, finalURL)

		screenshots, err := CapturePageScreenshots(ctx, MaxScreenshotsPerPage)
		if err != nil {
			fmt.Printf("[%s] 截图警告: %v\n", ModuleName, err)
			// 截图失败不阻断流程，继续使用文本内容
		} else {
			content.Screenshots = screenshots
		}
	}

	return content, nil
}

// extractDOMText 从当前页面提取 DOM 文本内容
func extractDOMText(ctx context.Context) (string, error) {
	var text string
	err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.body ? document.body.innerText : ""`, &text),
	)
	if err != nil {
		return "", err
	}
	return text, nil
}

// =============================================================================
// DOM 文本清洗
// =============================================================================

// cleanDOMText 清洗 DOM 提取的文本
// 移除噪声行、压缩空白、规范化输出
func cleanDOMText(raw string) string {
	lines := strings.Split(raw, "\n")
	var cleaned []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// 跳过太短的行（通常是 UI 碎片）
		if len([]rune(trimmed)) < 4 {
			continue
		}

		// 检查是否为噪声行
		isNoise := false
		for _, pattern := range noisePatterns {
			if pattern.MatchString(trimmed) {
				isNoise = true
				break
			}
		}
		if isNoise {
			continue
		}

		cleaned = append(cleaned, trimmed)
	}

	// 合并为单个文本块，保留自然换行
	result := strings.Join(cleaned, "\n")

	// 压缩连续空行
	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")

	return result
}

// =============================================================================
// HTML 解析辅助函数
// =============================================================================

// findNodesByClass 递归查找指定标签且 class 属性完全匹配的节点
func findNodesByClass(n *html.Node, tag string, className string) []*html.Node {
	var results []*html.Node
	var find func(*html.Node)
	find = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == tag {
			for _, attr := range node.Attr {
				if attr.Key == "class" && attr.Val == className {
					results = append(results, node)
					break
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(n)
	return results
}

// findNodesByPartialClass 递归查找指定标签且 class 属性包含指定字符串的节点
func findNodesByPartialClass(n *html.Node, tag string, classPart string) []*html.Node {
	var results []*html.Node
	var find func(*html.Node)
	find = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == tag {
			for _, attr := range node.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, classPart) {
					results = append(results, node)
					break
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(n)
	return results
}

// findTags 递归查找指定标签名的所有节点
func findTags(n *html.Node, tag string) []*html.Node {
	var results []*html.Node
	var find func(*html.Node)
	find = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == tag {
			results = append(results, node)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(n)
	return results
}

// extractTextContent 递归提取节点内所有文本内容
func extractTextContent(n *html.Node) string {
	var text strings.Builder
	var extract func(*html.Node)
	extract = func(node *html.Node) {
		if node.Type == html.TextNode {
			text.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}
	extract(n)
	return strings.TrimSpace(text.String())
}
