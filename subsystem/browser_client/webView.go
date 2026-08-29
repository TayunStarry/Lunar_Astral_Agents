package BrowserClient

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"net"
	"net/url"
	"runtime"
	"time"

	webview "github.com/webview/webview_go"
)

// waitForServer 轮询等待 HTTP 服务就绪，最多等待 10 秒
func waitForServer(rawURL string) {
	LoggerGeneral.SubInfo("BrowserClient", "waitForServer", "开始等待服务器就绪")
	u, err := url.Parse(rawURL)
	if err != nil {
		LoggerGeneral.SubError("BrowserClient", "waitForServer", "URL解析失败 %v, 跳过等待", err)
		return
	}
	addr := u.Host
	if _, _, e := net.SplitHostPort(addr); e != nil {
		if u.Scheme == "https" {
			addr += ":443"
		} else {
			addr += ":80"
		}
	}
	LoggerGeneral.SubInfo("BrowserClient", "waitForServer", "检测目标地址 %s", addr)

	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			LoggerGeneral.SubError("BrowserClient", "waitForServer", "超时(10s)未检测到服务，继续启动 webview")
			return
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
			if err == nil {
				conn.Close()
				LoggerGeneral.SubInfo("BrowserClient", "waitForServer", "服务已就绪")
				return
			}
			LoggerGeneral.SubInfo("BrowserClient", "waitForServer", "连接失败 %v, 继续等待", err)
		}
	}
}

// StartWebViewBrowser 在主线程启动 webview（需在已锁定的 OS 线程中调用）
// initJS 为可选的页面注入脚本：通过 WebView2 AddScriptToExecuteOnDocumentCreated
// 在每个新文档创建时执行（如注入「返回主页面」悬浮按钮），为空则不注入。
func StartWebViewBrowser(url string, initJS string) {
	// 1. 通知调用者我们将进入主线程循环（此函数必须在专用 goroutine 中调用）
	LoggerGeneral.Info("BrowserClient", "准备进入主线程 webview 循环")
	runtime.LockOSThread() // 确保当前 goroutine 固定在一个系统线程

	// 2. 等待前一个实例清理完成，避免在脏状态上创建新实例
	waitForCleanup()

	// 3. 标记正在运行（外部可通过此位防重入）
	webviewMutex.Lock()
	if webviewRunning {
		webviewMutex.Unlock()
		runtime.UnlockOSThread()
		LoggerGeneral.Info("BrowserClient", "已有实例在运行，退出")
		return
	}
	webviewRunning = true
	webviewMutex.Unlock()

	// 4. 创建 webview（内部已加锁）
	LoggerGeneral.Info("BrowserClient", "准备创建 webview 实例")
	w := createWebView()
	if w == nil {
		LoggerGeneral.Error("BrowserClient", "创建 webview 失败，退出")
		webviewMutex.Lock()
		webviewRunning = false
		webviewMutex.Unlock()
		runtime.UnlockOSThread()
		return
	}

	// 5. 应用窗口图标与标题栏样式（需在主线程、Run 之前调用）
	applyWindowStyle(w)

	// 5.1 注册页面注入脚本（在每个新文档创建时执行，见 StartWebViewBrowser 注释）
	if initJS != "" {
		LoggerGeneral.SubInfo("BrowserClient", "StartWebViewBrowser", "注册页面注入脚本")
		w.Init(initJS)
	}

	// 6. 退出时清理资源并发送关闭通知（必须在 UnlockOSThread 之前执行）
	defer func() {
		LoggerGeneral.Info("BrowserClient", "开始清理资源")
		webviewMutex.Lock()
		webviewRunning = false
		webviewInstance = nil
		webviewMutex.Unlock()

		// Run() 已返回，调用 Destroy 释放底层 webview2 资源
		LoggerGeneral.Info("BrowserClient", "调用 Destroy 释放 webview 资源")
		w.Destroy()

		// 广播关闭信号
		select {
		case webviewClosedCh <- struct{}{}:
		default:
		}
		// 通知等待清理的 goroutine
		select {
		case webviewCleanupDone <- struct{}{}:
		default:
		}
		LoggerGeneral.Info("BrowserClient", "资源清理完毕")

		runtime.UnlockOSThread()
	}()

	// 7. 等待后端服务就绪
	waitForServer(url)

	// 8. 导航到目标 URL
	LoggerGeneral.Info("BrowserClient", "开始导航到 %s", url)
	navigateWebView(url)

	// 9. 运行事件循环（会阻塞直到窗口关闭）
	LoggerGeneral.Info("BrowserClient", "进入 Run 循环")
	w.Run()
	LoggerGeneral.Info("BrowserClient", "Run 循环结束，窗口已关闭")
}

// createWebView 创建单例 webview（调用者需保证在主线程）
func createWebView() webview.WebView {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()

	if webviewInstance != nil {
		LoggerGeneral.SubInfo("BrowserClient", "createWebView", "返回已有实例")
		return webviewInstance
	}

	LoggerGeneral.SubInfo("BrowserClient", "createWebView", "调用 webview.New")
	w := webview.New(*GeneralConfig.Developer)
	if w == nil {
		LoggerGeneral.SubError("BrowserClient", "createWebView", "webview.New 返回 nil")
		return nil
	}

	LoggerGeneral.SubInfo("BrowserClient", "createWebView", "设置窗口标题和尺寸")
	w.SetTitle(*GeneralConfig.WebViewTitle)
	w.SetSize(*GeneralConfig.WebViewWidth, *GeneralConfig.WebViewHeight, webview.HintNone)

	if *GeneralConfig.WebViewMinWidth > 0 && *GeneralConfig.WebViewMinHeight > 0 {
		w.SetSize(*GeneralConfig.WebViewMinWidth, *GeneralConfig.WebViewMinHeight, webview.HintMin)
	}

	if !*GeneralConfig.WebViewResizable {
		w.SetSize(*GeneralConfig.WebViewWidth, *GeneralConfig.WebViewHeight, webview.HintFixed)
	}

	webviewInstance = w
	LoggerGeneral.SubInfo("BrowserClient", "createWebView", "实例创建成功")
	return w
}

// navigateWebView 导航到 URL
func navigateWebView(url string) {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()

	if webviewInstance == nil {
		LoggerGeneral.SubError("BrowserClient", "navigateWebView", "实例为 nil，无法导航")
		return
	}
	webviewInstance.Navigate(url)
	LoggerGeneral.SubInfo("BrowserClient", "navigateWebView", "导航调用完成")
}

// CloseWebView 安全关闭 webview（从任意 goroutine 调用）
// 调用 Terminate 中断 Run 循环，实际资源释放由 StartWebViewBrowser 的 defer 完成
func CloseWebView() {
	LoggerGeneral.SubInfo("BrowserClient", "CloseWebView", "收到关闭请求")
	webviewMutex.Lock()
	w := webviewInstance
	webviewMutex.Unlock()

	if w != nil {
		LoggerGeneral.SubInfo("BrowserClient", "CloseWebView", "调用 Terminate 中断事件循环")
		w.Terminate()
	}
}

// IsWebViewSupported 保持不变
func IsWebViewSupported() bool {
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		return true
	default:
		return false
	}
}

// WebViewClosed 返回一个 channel，当 webview 关闭时收到信号
func WebViewClosed() <-chan struct{} {
	return webviewClosedCh
}

// IsWebViewRunning 返回当前是否有 webView 实例在运行
func IsWebViewRunning() bool {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()
	return webviewRunning
}

// waitForCleanup 等待前一个 webview 实例清理完成
func waitForCleanup() {
	webviewMutex.Lock()
	running := webviewRunning
	webviewMutex.Unlock()
	if !running {
		return
	}
	// 等待清理完成信号或超时
	select {
	case <-webviewCleanupDone:
	case <-time.After(5 * time.Second):
	}
}
