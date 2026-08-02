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
	Render(url string, query ...string) (string, error)
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

func (c *ChromeRenderer) Render(url string, query ...string) (string, error) {
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
		return "", err
	}

	err = chromedp.Run(pageCtx,
		chromedp.Sleep(3*time.Second),
		chromedp.Evaluate(`
			var noticeItems = document.querySelectorAll('[class*="notice"],[class*="Notice"],[class*="公告"]');
			noticeItems.forEach(function(item) {
				if (item.getAttribute('class') && item.getAttribute('class').includes('noticeItem')) {
					item.click();
				}
			});
		`, nil),
		chromedp.Sleep(1*time.Second),
		chromedp.Text("body", &text),
	)
	if err != nil {
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 获取内容失败 url=%s err=%v", url, err)
		}
		return "", err
	}

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
		return "", err
	}

	if c.debugLog != nil {
		c.debugLog("[浏览器渲染] 主页渲染完成 url=%s text_len=%d links_count=%d", url, len([]rune(text)), len(links))
	}

	textLen := len([]rune(text))
	if textLen >= 800 {
		// 检查内容是否包含查询关键词
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

		// 如果内容足够丰富但不包含查询关键词，不跳过深度渲染
		if !containsQueryKeywords && queryKeywords != "" {
			if c.debugLog != nil {
				c.debugLog("[浏览器渲染] 页面内容足够但不包含查询关键词，继续深度渲染")
			}
		} else {
			if c.debugLog != nil {
				c.debugLog("[浏览器渲染] 页面内容已足够丰富，跳过深度渲染")
			}
			return text, nil
		}
	}

	if len(links) > 0 {
		// 检查内容是否包含查询关键词
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

	return text, nil
}

func (c *ChromeRenderer) Close() {
	c.browserCancel()
	c.allocatorCancel()

	if c.browserCmd != nil && c.browserCmd.Process != nil {
		c.browserCmd.Process.Kill()
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 浏览器进程已终止 pid=%d", c.browserCmd.Process.Pid)
		}
	}

	if c.tempDir != "" {
		os.RemoveAll(c.tempDir)
		if c.debugLog != nil {
			c.debugLog("[浏览器渲染] 临时目录已清理 dir=%s", c.tempDir)
		}
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
	return len([]rune(text)) < 200
}

// isNewsLink 判断 URL 是否为新闻/资讯类页面（不要求尾斜杠，/news 和 /news/ 均可匹配）
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

func renderWithBrowser(renderer BrowserRenderer, url string, debugLog func(format string, args ...interface{}), query ...string) string {
	if renderer == nil {
		return ""
	}
	rendered, err := renderer.Render(url, query...)
	if err != nil {
		if debugLog != nil {
			debugLog("[浏览器渲染] 渲染失败 url=%s err=%v", url, err)
		}
		return ""
	}
	if len([]rune(rendered)) < 50 {
		if debugLog != nil {
			debugLog("[浏览器渲染] 渲染结果仍为空壳 url=%s rendered_len=%d", url, len([]rune(rendered)))
		}
		return ""
	}
	return rendered
}
