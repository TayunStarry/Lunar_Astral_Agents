package browser

import (
	"config"
	"logger"
	"net"
	"net/url"
	"runtime"
	"time"

	webview "github.com/webview/webview_go"
)

// waitForServer 轮询等待 HTTP 服务就绪，最多等待 10 秒
func waitForServer(rawURL string) {
	logger.Info("Browser", "waitForServer: 开始等待服务器就绪")
	u, err := url.Parse(rawURL)
	if err != nil {
		logger.Error("Browser", "waitForServer: URL解析失败 %v, 跳过等待", err)
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
	logger.Info("Browser", "waitForServer: 检测目标地址 %s", addr)

	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			logger.Error("Browser", "waitForServer: 超时(10s)未检测到服务，继续启动 webview")
			return
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
			if err == nil {
				conn.Close()
				logger.Info("Browser", "waitForServer: 服务已就绪")
				return
			}
			logger.Info("Browser", "waitForServer: 连接失败 %v, 继续等待", err)
		}
	}
}

// StartWebViewBrowser 在主线程启动 webview（需在已锁定的 OS 线程中调用）
func StartWebViewBrowser(url string) {
	// 1. 通知调用者我们将进入主线程循环（此函数必须在专用 goroutine 中调用）
	logger.Info("Browser", "准备进入主线程 webview 循环")
	runtime.LockOSThread() // 确保当前 goroutine 固定在一个系统线程
	defer runtime.UnlockOSThread()

	// 2. 标记正在运行（外部可通过此位防重入）
	webviewMutex.Lock()
	if webviewRunning {
		webviewMutex.Unlock()
		logger.Info("Browser", "已有实例在运行，退出")
		return
	}
	webviewRunning = true
	webviewMutex.Unlock()

	// 3. 退出时清理资源并发送关闭通知
	defer func() {
		logger.Info("Browser", "开始清理资源")
		webviewMutex.Lock()
		webviewRunning = false
		w := webviewInstance
		webviewInstance = nil
		webviewMutex.Unlock()

		if w != nil {
			logger.Info("Browser", "调用 Terminate 销毁窗口")
			w.Terminate()
		}
		// 广播关闭信号
		select {
		case webviewClosedCh <- struct{}{}:
		default:
		}
		logger.Info("Browser", "资源清理完毕")
	}()

	// 4. 等待后端服务就绪
	waitForServer(url)

	// 5. 创建 webview（内部已加锁）
	logger.Info("Browser", "准备创建 webview 实例")
	w := createWebView()
	if w == nil {
		logger.Error("Browser", "创建 webview 失败，退出")
		return
	}

	// 6. 导航到目标 URL
	logger.Info("Browser", "开始导航到 %s", url)
	navigateWebView(url)

	// 7. 运行事件循环（会阻塞直到窗口关闭）
	logger.Info("Browser", "进入 Run 循环")
	w.Run()
	logger.Info("Browser", "Run 循环结束，窗口已关闭")
}

// createWebView 创建单例 webview（调用者需保证在主线程）
func createWebView() webview.WebView {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()

	if webviewInstance != nil {
		logger.Info("Browser", "createWebView: 返回已有实例")
		return webviewInstance
	}

	logger.Info("Browser", "createWebView: 调用 webview.New")
	w := webview.New(*config.Developer)
	if w == nil {
		logger.Error("Browser", "createWebView: webview.New 返回 nil")
		return nil
	}

	logger.Info("Browser", "createWebView: 设置窗口标题和尺寸")
	w.SetTitle(*config.WebViewTitle)
	w.SetSize(*config.WebViewWidth, *config.WebViewHeight, webview.HintNone)

	if *config.WebViewMinWidth > 0 && *config.WebViewMinHeight > 0 {
		w.SetSize(*config.WebViewMinWidth, *config.WebViewMinHeight, webview.HintMin)
	}

	if !*config.WebViewResizable {
		w.SetSize(*config.WebViewWidth, *config.WebViewHeight, webview.HintFixed)
	}

	webviewInstance = w
	logger.Info("Browser", "createWebView: 实例创建成功")
	return w
}

// navigateWebView 导航到 URL
func navigateWebView(url string) {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()

	if webviewInstance == nil {
		logger.Error("Browser", "navigateWebView: 实例为 nil，无法导航")
		return
	}
	webviewInstance.Navigate(url)
	logger.Info("Browser", "navigateWebView: 导航调用完成")
}

// CloseWebView 安全关闭 webview（从任意 goroutine 调用）
func CloseWebView() {
	logger.Info("Browser", "CloseWebView: 收到关闭请求")
	webviewMutex.Lock()
	w := webviewInstance
	webviewInstance = nil
	webviewRunning = false // 提前标记，以便主循环退出时不再重复清理
	webviewMutex.Unlock()

	if w != nil {
		logger.Info("Browser", "CloseWebView: 向主线程发送 Terminate")
		select {
		case webviewCmdCh <- cmdQuit:
		default:
		}
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
