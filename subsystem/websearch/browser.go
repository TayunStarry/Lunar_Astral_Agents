package websearch

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

type BrowserRenderer interface {
	Render(url string, query ...string) (text string, title string, err error)
	Close()
}

type ChromeRenderer struct {
	browserCtx      context.Context
	browserCancel   context.CancelFunc
	allocatorCancel context.CancelFunc
	timeout         time.Duration
	browserPath     string
	debugLog        func(format string, args ...interface{})
	browserCmd      *exec.Cmd
	tempDir         string
}

func NewChromeRenderer(timeout time.Duration, debugLog func(format string, args ...interface{})) *ChromeRenderer {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	browserPath := findChromiumBrowser()
	if browserPath == "" {
		if debugLog != nil {
			debugLog("[浏览器渲染] 初始化失败：未找到Chromium浏览器（Chrome/Edge/Chromium）")
		}
		return nil
	}

	if debugLog != nil {
		debugLog("[浏览器渲染] 正在启动浏览器 path=%s", browserPath)
	}

	tempDir, err := os.MkdirTemp("", "yaraflow-chrome-*")
	if err != nil {
		if debugLog != nil {
			debugLog("[浏览器渲染] 初始化失败：创建临时目录失败 err=%v", err)
		}
		return nil
	}

	if debugLog != nil {
		debugLog("[浏览器渲染] 使用临时目录 tempDir=%s", tempDir)
	}

	args := []string{
		"--headless=new",
		"--remote-debugging-port=0",
		"--user-data-dir=" + tempDir,
		"--disable-gpu",
		"--disable-extensions",
		"--disable-background-networking",
		"--disable-sync",
		"--mute-audio",
		"--hide-scrollbars",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-features=TranslateUI,AutofillServerCommunication",
		"--disable-blink-features=AutomationControlled",
		"--window-size=1920,1080",
	}

	cmd := exec.Command(browserPath, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		if debugLog != nil {
			debugLog("[浏览器渲染] 初始化失败：创建stderr管道失败 err=%v", err)
		}
		os.RemoveAll(tempDir)
		return nil
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		if debugLog != nil {
			debugLog("[浏览器渲染] 初始化失败：创建stdout管道失败 err=%v", err)
		}
		os.RemoveAll(tempDir)
		return nil
	}

	if err := cmd.Start(); err != nil {
		if debugLog != nil {
			debugLog("[浏览器渲染] 初始化失败：启动浏览器进程失败 err=%v", err)
		}
		os.RemoveAll(tempDir)
		return nil
	}

	wsChan := make(chan string, 1)
	var stderrLines []string

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			stderrLines = append(stderrLines, line)
			if debugLog != nil {
				debugLog("[浏览器渲染] 浏览器stderr: %s", line)
			}
			if matches := regexp.MustCompile(`ws://[^\s]+`).FindString(line); matches != "" {
				wsChan <- matches
				return
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if debugLog != nil {
				debugLog("[浏览器渲染] 浏览器stdout: %s", line)
			}
			if matches := regexp.MustCompile(`ws://[^\s]+`).FindString(line); matches != "" {
				wsChan <- matches
				return
			}
		}
	}()

	go func() {
		activePortPath := filepath.Join(tempDir, "DevToolsActivePort")
		for i := 0; i < 30; i++ {
			time.Sleep(300 * time.Millisecond)
			data, err := os.ReadFile(activePortPath)
			if err != nil {
				continue
			}
			content := strings.TrimSpace(string(data))
			if content == "" {
				continue
			}
			lines := strings.Split(content, "\n")
			if len(lines) >= 2 {
				port := strings.TrimSpace(lines[0])
				wsPath := strings.TrimSpace(lines[1])
				wsURL := fmt.Sprintf("ws://127.0.0.1:%s%s", port, wsPath)
				if debugLog != nil {
					debugLog("[浏览器渲染] 从DevToolsActivePort获取WebSocket URL: %s", wsURL)
				}
				wsChan <- wsURL
				return
			}
		}
	}()

	var wsURL string
	select {
	case wsURL = <-wsChan:
	case <-time.After(15 * time.Second):
		if debugLog != nil {
			debugLog("[浏览器渲染] 初始化失败：等待WebSocket URL超时")
			if len(stderrLines) > 0 {
				debugLog("[浏览器渲染] 浏览器stderr输出:")
				for _, line := range stderrLines {
					debugLog("[浏览器渲染]   %s", line)
				}
			}
		}
		cmd.Process.Kill()
		killProcessTree(cmd.Process.Pid)
		os.RemoveAll(tempDir)
		return nil
	}

	if debugLog != nil {
		debugLog("[浏览器渲染] 获取到WebSocket URL: %s", wsURL)
	}

	time.Sleep(1 * time.Second)

	ctx := context.Background()
	allocatorCtx, allocatorCancel := chromedp.NewRemoteAllocator(ctx, wsURL)

	go func() {
		cmd.Wait()
	}()

	browserCtx, browserCancel := chromedp.NewContext(allocatorCtx)

	if debugLog != nil {
		debugLog("[浏览器渲染] 初始化成功 浏览器=%s", browserPath)
	}

	return &ChromeRenderer{
		browserCtx:      browserCtx,
		browserCancel:   browserCancel,
		allocatorCancel: allocatorCancel,
		timeout:         timeout,
		browserPath:     browserPath,
		browserCmd:      cmd,
		tempDir:         tempDir,
	}
}

func (c *ChromeRenderer) BrowserPath() string { return c.browserPath }

func (c *ChromeRenderer) SetDebugLog(fn func(format string, args ...interface{})) {
	c.debugLog = fn
}

func (c *ChromeRenderer) Render(url string, query ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(c.browserCtx, c.timeout)
	defer cancel()

	pageCtx, pageCancel := chromedp.NewContext(ctx)
	defer pageCancel()

	queryKeywords := ""
	if len(query) > 0 && query[0] != "" {
		queryKeywords = query[0]
	}

	if c.debugLog != nil {
		if queryKeywords != "" {
			c.debugLog("[浏览器渲染] 新标签页已创建 URL=%s query=%s", url, queryKeywords)
		} else {
			c.debugLog("[浏览器渲染] 新标签页已创建 URL=%s", url)
		}
	}

	var text string
	var pageTitle string
	var links []string
	var err error

	pageLoadCtx, pageLoadCancel := context.WithTimeout(pageCtx, 30*time.Second)
	defer pageLoadCancel()

	err = chromedp.Run(pageLoadCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
	)
	if err != nil {
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 页面加载失败 url=%s err=%v", url, err)
		}
		return "", "", err
	}

	err = chromedp.Run(pageCtx,
		chromedp.Sleep(3*time.Second),
		chromedp.Text("body", &text),
		chromedp.Title(&pageTitle),
	)
	if err != nil {
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 获取内容失败 url=%s err=%v", url, err)
		}
		return "", "", err
	}

	// 预加载交互：尝试点击页面中与查询关键词相关的元素
	text, _ = c.interactPage(pageCtx, queryKeywords, text)

	err = chromedp.Run(pageCtx,
		chromedp.Evaluate(`
			var allLinks = [];
			var seen = new Set();
			
			document.querySelectorAll('[href]').forEach(el => {
				var href = el.getAttribute('href');
				if (!href) return;
				if (href.startsWith('javascript:') || href.startsWith('#')) return;
				if (href.startsWith('//')) href = 'https:' + href;
				if (!href.startsWith('http')) {
					if (href.startsWith('/')) {
						href = window.location.origin + href;
					} else {
						var base = window.location.href.replace(/\/[^/]*$/, '/');
						href = base + href;
					}
				}
				try {
					var urlObj = new URL(href);
					if (urlObj.hostname === window.location.hostname && !seen.has(href)) {
						seen.add(href);
						allLinks.push(href);
					}
				} catch(e) {}
			});
			
			if (allLinks.length === 0) {
				var regex = /https?:\/\/[^\\s\"'<>]+/g;
				var matches = document.documentElement.innerHTML.match(regex);
				if (matches) {
					matches.forEach(m => {
						try {
							var urlObj = new URL(m);
							if (urlObj.hostname === window.location.hostname && !seen.has(m)) {
								seen.add(m);
								allLinks.push(m);
							}
						} catch(e) {}
					});
				}
			}
			
			if (allLinks.length === 0) {
				var scripts = document.querySelectorAll('script');
				scripts.forEach(script => {
					if (script.textContent) {
						var regex = /https?:\/\/[^\\s\"'<>]+/g;
						var matches = script.textContent.match(regex);
						if (matches) {
							matches.forEach(m => {
								try {
									var urlObj = new URL(m);
									if (urlObj.hostname === window.location.hostname && !seen.has(m)) {
										seen.add(m);
										allLinks.push(m);
									}
								} catch(e) {}
							});
						}
					}
				});
			}
			
			if (allLinks.length === 0 && window.__NEXT_DATA__) {
				try {
					var data = JSON.stringify(window.__NEXT_DATA__);
					var regex = /https?:\/\/[^\\s\"'<>]+/g;
					var matches = data.match(regex);
					if (matches) {
						matches.forEach(m => {
							try {
								var urlObj = new URL(m);
								if (urlObj.hostname === window.location.hostname && !seen.has(m)) {
									seen.add(m);
									allLinks.push(m);
								}
							} catch(e) {}
						});
					}
				} catch(e) {}
			}
			
			allLinks;
		`, &links),
	)
	if err != nil {
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 提取链接失败 url=%s err=%v", url, err)
		}
	}

	if err != nil {
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 渲染失败 url=%s err=%v", url, err)
		}
		return "", "", err
	}

	if c.debugLog != nil {
		c.debugLog("[浏览器渲染] 主页渲染完成 url=%s text_len=%d links_count=%d", url, len([]rune(text)), len(links))
	}

	// 按时效排序：新闻/资讯类链接按数字ID降序排列，最新在前
	links = sortLinksByRecency(links)
	if c.debugLog != nil && len(links) > 0 {
		c.debugLog("[浏览器渲染] 链接时效排序后 top3=%v", links[:min(3, len(links))])
	}

	textLen := len([]rune(text))
	// 预先计算关键词命中状态，供后续多处使用
	containsQueryKeywords := false
	if queryKeywords != "" {
		textLower := strings.ToLower(text)
		for _, kw := range strings.Split(queryKeywords, ",") {
			if strings.Contains(textLower, strings.ToLower(kw)) {
				containsQueryKeywords = true
				break
			}
		}
	}

	if textLen >= 800 {
		// 如果内容足够丰富且包含查询关键词，直接返回
		if containsQueryKeywords {
			if c.debugLog != nil {
				c.debugLog("[浏览器渲染] 页面内容已足够丰富且包含关键词，跳过深度渲染")
			}
			return text, pageTitle, nil
		}
		// 内容丰富但不含关键词，继续深度渲染
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 页面内容足够但不包含查询关键词，继续深度渲染")
		}
	}

	// links为0但不含关键词时，再次尝试交互（可能预加载时没点到有效元素）
	richButNoKeyword := !containsQueryKeywords && queryKeywords != ""
	if len(links) == 0 && richButNoKeyword {
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 无链接但不含关键词，再次尝试交互查找")
		}
		text, _ = c.interactPage(pageCtx, queryKeywords, text)
	}

	if len(links) > 0 {
		shouldDeepRender := textLen < 200

		if !shouldDeepRender && textLen < 1000 && len(links) >= 5 {
			shouldDeepRender = true
			if c.debugLog != nil {
				c.debugLog("[浏览器渲染] 页面内容较薄但链接较多，尝试深度渲染子页面")
			}
		}

		if !shouldDeepRender {
			newsLinks := filterNewsLinks(links)
			if len(newsLinks) >= 3 {
				shouldDeepRender = true
				if c.debugLog != nil {
					c.debugLog("[浏览器渲染] 检测到多个新闻详情链接，尝试深度渲染")
				}
			}
		}

		// 如果内容足够丰富但不包含查询关键词，也触发深度渲染
		if !shouldDeepRender && !containsQueryKeywords && queryKeywords != "" {
			shouldDeepRender = true
			if c.debugLog != nil {
				c.debugLog("[浏览器渲染] 页面内容足够但不包含查询关键词，尝试深度渲染子页面")
			}
		}

		if shouldDeepRender {
			if c.debugLog != nil {
				c.debugLog("[浏览器渲染] 尝试深度渲染子页面")
			}

			filteredLinks := filterNewsLinks(links)
			if len(filteredLinks) == 0 {
				filteredLinks = make([]string, 0)
				for _, link := range links {
					if !isStaticResource(link) {
						filteredLinks = append(filteredLinks, link)
					}
				}
			}

			if queryKeywords != "" {
				relevantLinks := filterLinksByQuery(filteredLinks, queryKeywords)
				if len(relevantLinks) > 0 {
					if c.debugLog != nil {
						c.debugLog("[浏览器渲染] 根据查询关键词筛选出 %d 个相关子页面", len(relevantLinks))
					}
					filteredLinks = relevantLinks
				}
			}

			maxDepthLinks := 3
			if len(filteredLinks) < maxDepthLinks {
				maxDepthLinks = len(filteredLinks)
			}

			// 资讯页优先：一旦采用新闻/资讯类子页面内容，后续非资讯页（如隐私政策）不再覆盖
			textIsNews := false
			for i := 0; i < maxDepthLinks; i++ {
				subUrl := filteredLinks[i]
				if c.debugLog != nil {
					c.debugLog("[浏览器渲染] 深度渲染子页面 url=%s", subUrl)
				}

				subCtx, subCancel := context.WithTimeout(c.browserCtx, c.timeout)
				subPageCtx, subPageCancel := chromedp.NewContext(subCtx)

				var subText string
				subErr := chromedp.Run(subPageCtx,
					chromedp.Navigate(subUrl),
					chromedp.Sleep(3*time.Second),
					chromedp.Text("body", &subText),
				)

				subPageCancel()
				subCancel()

				if subErr != nil {
					if c.debugLog != nil {
						c.debugLog("[浏览器渲染] 深度渲染失败 url=%s err=%v", subUrl, subErr)
					}
					continue
				}

				subIsNews := isNewsLink(subUrl)
				// 资讯页直接采用；非资讯页仅在当前内容非资讯类且更长时覆盖
				if subIsNews && len([]rune(subText)) > 0 {
					text = subText
					textIsNews = true
					if c.debugLog != nil {
						c.debugLog("[浏览器渲染] 深度渲染成功(资讯页) url=%s text_len=%d", subUrl, len([]rune(text)))
					}
				} else if !textIsNews && len([]rune(subText)) > len([]rune(text)) {
					text = subText
					if c.debugLog != nil {
						c.debugLog("[浏览器渲染] 深度渲染成功 url=%s text_len=%d", subUrl, len([]rune(text)))
					}
				}
			}
		}
	}

	return text, pageTitle, nil
}

// interactPage 点击页面中与查询关键词相关的元素，触发SPA导航或内容展开
func (c *ChromeRenderer) interactPage(pageCtx context.Context, queryKeywords string, text string) (string, bool) {
	kwJS := "''"
	if queryKeywords != "" {
		kwJS = fmt.Sprintf("%q", queryKeywords)
	}

	interactJS := fmt.Sprintf(`(function() {
		var kw = %s;
		var keywords = kw.split(",").filter(Boolean);
		var allElements = document.querySelectorAll("a, button, [role='link'], div[onclick], span[onclick]");

		// 1. 优先点击文本包含查询关键词的元素
		for (var el of allElements) {
			var elText = (el.textContent || "").trim();
			if (elText.length > 50) continue;
			for (var k of keywords) {
				if (k && elText.toLowerCase().includes(k.toLowerCase())) {
					try { el.click(); return "kw:" + elText; } catch(e) {}
				}
			}
		}

		// 2. 点击常见的展开/导航元素
		var selectors = [
			'[class*="notice"]','[class*="Notice"]','[class*="公告"]',
			'[class*="more"]','[class*="More"]','[class*="expand"]','[class*="展开"]',
			'[class*="tab"]','[class*="Tab"]','[role="tab"]',
			'button','[class*="btn"]','[class*="Btn"]',
			'a[class*="nav"]','a[class*="Nav"]'
		];
		var seen = new Set();
		for (var sel of selectors) {
			var els = document.querySelectorAll(sel);
			for (var el of els) {
				var key = el.textContent || "";
				if (seen.has(key)) continue;
				seen.add(key);
				try { el.click(); } catch(e) {}
			}
		}
		return "generic";
	})()`, kwJS)

	var clickResult string
	interactCtx, interactCancel := context.WithTimeout(pageCtx, 5*time.Second)
	defer interactCancel()

	if err := chromedp.Run(interactCtx, chromedp.Evaluate(interactJS, &clickResult)); err != nil {
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 页面交互执行失败: %v", err)
		}
		return text, false
	}

	// 等待交互后的新内容
	var newText string
	if err := chromedp.Run(interactCtx,
		chromedp.Sleep(3*time.Second),
		chromedp.Text("body", &newText),
	); err != nil {
		return text, false
	}

	if len([]rune(newText)) > len([]rune(text)) {
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 页面交互后内容增加 text_len=%d click=%s", len([]rune(newText)), clickResult)
		}
		return newText, true
	}
	// SPA 导航会替换页面内容（而非追加），需检测内容是否真正变化
	if len(newText) > 0 && newText != text {
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 页面交互后内容变化 text_len=%d original_len=%d click=%s",
				len([]rune(newText)), len([]rune(text)), clickResult)
		}
		return newText, true
	}
	return text, false
}

func (c *ChromeRenderer) Close() {
	c.browserCancel()
	c.allocatorCancel()

	if c.browserCmd != nil && c.browserCmd.Process != nil {
		killProcessTree(c.browserCmd.Process.Pid)
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 浏览器进程树已终止 pid=%d", c.browserCmd.Process.Pid)
		}
	}

	if c.tempDir != "" {
		os.RemoveAll(c.tempDir)
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 临时目录已清理 dir=%s", c.tempDir)
		}
	}
}

// killProcessTree 终止进程及其所有子进程（Windows 用 taskkill，Unix 用 kill）
func killProcessTree(pid int) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)).Run()
	default:
		exec.Command("kill", "-TERM", fmt.Sprintf("-%d", pid)).Run()
	}
}

func findChromiumBrowser() string {
	switch runtime.GOOS {
	case "windows":
		return findChromiumWindows()
	case "darwin":
		return findChromiumDarwin()
	case "linux":
		return findChromiumLinux()
	default:
		return ""
	}
}

func findChromiumWindows() string {
	possiblePaths := []string{
		filepath.Join(os.Getenv("PROGRAMFILES"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("PROGRAMFILES"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("PROGRAMFILES"), "Chromium", "Application", "chromium.exe"),
		filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Chromium", "Application", "chromium.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "Application", "msedge.exe"),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func findChromiumDarwin() string {
	possiblePaths := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func findChromiumLinux() string {
	possiblePaths := []string{
		"/usr/bin/google-chrome",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/local/bin/google-chrome",
		"/usr/local/bin/chromium",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func CleanupOrphanBrowsers() {
	switch runtime.GOOS {
	case "windows":
		cleanupOrphanBrowsersWindows()
	case "darwin":
		cleanupOrphanBrowsersDarwin()
	case "linux":
		cleanupOrphanBrowsersLinux()
	}
}

func cleanupOrphanBrowsersWindows() {
	cmd := exec.Command("powershell", "-Command",
		"Get-CimInstance Win32_Process | Where-Object { $_.Name -match 'chrome|msedge' -and $_.CommandLine -match 'yaraflow-chrome' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }")
	cmd.Run()
}

func cleanupOrphanBrowsersDarwin() {
	cmd := exec.Command("pkill", "-f", "yaraflow-chrome")
	cmd.Run()
}

func cleanupOrphanBrowsersLinux() {
	cmd := exec.Command("pkill", "-f", "yaraflow-chrome")
	cmd.Run()
}

func isSPAShell(text string) bool {
	// 文本太短 → SPA空壳
	if len([]rune(text)) < 200 {
		return true
	}
	// 去除常见导航词后剩余有效文本极少 → 主要是导航内容
	navKeywords := []string{
		"首页", "导航", "菜单", "登录", "注册", "帮助", "关于",
		"home", "menu", "login", "register", "help", "about",
		"联系", "公告", "新闻中心", "产品", "服务", "支持",
	}
	lower := strings.ToLower(text)
	effective := text
	for _, kw := range navKeywords {
		effective = strings.ReplaceAll(effective, kw, "")
	}
	// 去掉导航关键词后剩下不到100字符 → SPA空壳
	if len([]rune(strings.TrimSpace(effective))) < 100 {
		return true
	}
	_ = lower
	return false
}

// isNewsLink 判断URL是否为新闻/资讯类页面
func isNewsLink(url string) bool {
	newsPatterns := []string{"/news", "/detail", "/article", "/post", "/blog", "/story", "/info", "/announcement"}
	for _, pattern := range newsPatterns {
		if strings.Contains(url, pattern) {
			return true
		}
	}
	return false
}

func filterNewsLinks(links []string) []string {
	var result []string
	for _, link := range links {
		if isStaticResource(link) || !isNewsLink(link) {
			continue
		}
		result = append(result, link)
	}
	return result
}

func isStaticResource(url string) bool {
	staticExtensions := []string{".js", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".woff", ".woff2", ".ttf", ".ico", ".json", ".map"}
	for _, ext := range staticExtensions {
		if strings.HasSuffix(strings.ToLower(url), ext) {
			return true
		}
	}
	if strings.Contains(url, "/_static/") || strings.Contains(url, "/static/") {
		return true
	}
	return false
}

func filterLinksByQuery(links []string, query string) []string {
	var result []string
	queryLower := strings.ToLower(query)

	// 关键词分组：覆盖常见主题术语
	keywordGroups := map[string][]string{
		"卡池": {"卡池", "祈愿", "up", "up池", "角色池", "活动祈愿", "常驻祈愿", "限定", "寻访", "抽卡", "卡包"},
		"角色": {"角色", "干员", "英雄", "人物", "图鉴", "角色介绍", "角色攻略"},
		"版本": {"版本", "更新", "维护", "patch", "update", "新版本"},
		"活动": {"活动", "限时", "庆典", "节日", "福利", "活动公告", "限时活动"},
		"公告": {"公告", "通知", "声明", "发布", "情报", "新闻"},
	}

	var relevantKeywords []string
	for groupName, keywords := range keywordGroups {
		for _, kw := range keywords {
			if strings.Contains(queryLower, strings.ToLower(kw)) {
				relevantKeywords = append(relevantKeywords, groupName)
				break
			}
		}
	}

	// URL模式分组：覆盖不同网站的URL结构
	linkPatternGroups := map[string][]string{
		"卡池": {"/banner/", "/wish/", "/gacha/", "/summon/", "/event/", "/draw/", "/pull/", "/pickup/"},
		"角色": {"/character/", "/operator/", "/hero/", "/char/", "/avatar/", "/unit/"},
		"版本": {"/version/", "/update/", "/patch/", "/release/", "/ver/"},
		"活动": {"/event/", "/activity/", "/campaign/", "/festival/", "/limited/"},
		"公告": {"/notice/", "/announcement/", "/news/", "/detail/", "/info/", "/article/", "/post/"},
	}

	// 第一阶段：按URL模式匹配
	for _, link := range links {
		if isStaticResource(link) {
			continue
		}
		for _, group := range relevantKeywords {
			for _, pattern := range linkPatternGroups[group] {
				if strings.Contains(strings.ToLower(link), pattern) {
					result = append(result, link)
					break
				}
			}
		}
	}

	// 第二阶段：如果没有匹配到，按关键词匹配链接文本或路径
	if len(result) == 0 && len(relevantKeywords) > 0 {
		for _, link := range links {
			if isStaticResource(link) {
				continue
			}
			// 检查链接路径是否包含关键词
			for _, group := range relevantKeywords {
				if strings.Contains(strings.ToLower(link), group) {
					result = append(result, link)
					break
				}
			}
		}
	}

	// 第三阶段：如果还是没有匹配到，返回新闻类链接作为兜底
	if len(result) == 0 && len(relevantKeywords) > 0 {
		for _, link := range links {
			if isStaticResource(link) {
				continue
			}
			// 返回新闻/详情类链接
			for _, pattern := range []string{"/news/", "/detail/", "/article/", "/post/", "/info/"} {
				if strings.Contains(strings.ToLower(link), pattern) {
					result = append(result, link)
					break
				}
			}
		}
	}

	return result
}

// sortLinksByRecency 新闻/资讯类链接按URL中数字ID降序排列，最新在前
func sortLinksByRecency(links []string) []string {
	if len(links) <= 1 {
		return links
	}

	// 提取每个链接的排序键：数字ID + 是否为新闻/资讯页
	type scoredLink struct {
		url    string
		numID  int // 数字ID，越大越新
		isNews bool
	}
	scored := make([]scoredLink, 0, len(links))
	newsPatterns := []string{"/news/", "/detail/", "/article/", "/post/", "/info/", "/announcement/"}

	for _, link := range links {
		sl := scoredLink{url: link}

		// 检查是否为新闻/资讯类页面
		for _, p := range newsPatterns {
			if strings.Contains(link, p) {
				sl.isNews = true
				break
			}
		}

		// 提取 URL 路径中的最后一个数字段作为ID
		// 如 /news/9335 → 9335, /news/7231 → 7231
		if idx := strings.LastIndex(link, "/"); idx >= 0 {
			numStr := link[idx+1:]
			// 去掉可能的查询参数和尾部斜杠
			if qIdx := strings.Index(numStr, "?"); qIdx >= 0 {
				numStr = numStr[:qIdx]
			}
			if numStr != "" {
				// 提取连续数字
				var digits []byte
				for _, ch := range numStr {
					if ch >= '0' && ch <= '9' {
						digits = append(digits, byte(ch))
					} else {
						break
					}
				}
				if len(digits) > 0 {
					fmt.Sscanf(string(digits), "%d", &sl.numID)
				}
			}
		}

		scored = append(scored, sl)
	}

	// 排序：新闻/资讯页按数字ID降序，非新闻页保持原顺序
	// 新闻页之间：ID大的在前（最新）
	// 非新闻页之间：保持原顺序
	newsIdx := 0
	for i := range scored {
		if scored[i].isNews {
			// 将新闻项移到前面，按ID降序插入
			insert := newsIdx
			for j := newsIdx; j < i; j++ {
				if scored[j].isNews && scored[j].numID < scored[i].numID {
					insert = j
					break
				}
				if !scored[j].isNews {
					insert = j
					break
				}
				insert = j + 1
			}
			if insert < i {
				tmp := scored[i]
				copy(scored[insert+1:], scored[insert:i])
				scored[insert] = tmp
			}
			newsIdx++
		}
	}

	result := make([]string, len(scored))
	for i, sl := range scored {
		result[i] = sl.url
	}
	return result
}

func renderWithBrowser(renderer BrowserRenderer, url string, debugLog func(format string, args ...interface{}), query ...string) (string, string) {
	if renderer == nil {
		return "", ""
	}
	// 安全兜底：过滤字典/百科网站，避免浪费资源渲染无关页面
	if isDictionarySite(url) {
		if debugLog != nil {
			debugLog("[浏览器渲染] 字典网站过滤跳过 URL=%s", url)
		}
		return "", ""
	}
	rendered, title, err := renderer.Render(url, query...)
	if err != nil {
		if debugLog != nil {
			debugLog("[浏览器渲染] 渲染失败 url=%s err=%v", url, err)
		}
		return "", ""
	}
	if len([]rune(rendered)) < 50 {
		if debugLog != nil {
			debugLog("[浏览器渲染] 渲染结果仍为空壳 url=%s rendered_len=%d", url, len([]rune(rendered)))
		}
		return "", ""
	}
	return rendered, title
}
