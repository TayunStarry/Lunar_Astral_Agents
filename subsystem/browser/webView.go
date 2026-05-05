package browser

import (
	"config"
	"log"
	"runtime"
	"time"

	webview "github.com/webview/webview_go"
)

// StartWebViewBrowser 启动 webview 浏览器
func StartWebViewBrowser(url string) {
	// 等待服务器启动完成
	time.Sleep(1 * time.Second)
	// 创建 webview 实例
	w := CreateWebView()
	if w == nil {
		log.Printf("Webview[ERROR] -> 无法创建 webview 实例")
		return
	}
	// 导航到指定 URL
	NavigateWebView(url)
	// 运行 webview（阻塞）
	RunWebView()
	// webview 关闭后，发送关闭信号
	webviewClosedChan <- struct{}{}
}

// CreateWebView 创建并返回一个 WebView 实例（单例模式）
func CreateWebView() webview.WebView {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()

	if webviewInstance != nil {
		return webviewInstance
	}

	w := webview.New(*config.Developer)
	if w == nil {
		log.Printf("Webview[ERROR] -> 无法创建 WebView 实例")
		return nil
	}

	w.SetTitle(*config.WebViewTitle)
	w.SetSize(*config.WebViewWidth, *config.WebViewHeight, webview.HintNone)

	// 设置最小尺寸限制
	if *config.WebViewMinWidth > 0 && *config.WebViewMinHeight > 0 {
		w.SetSize(*config.WebViewMinWidth, *config.WebViewMinHeight, webview.HintMin)
	}

	if !*config.WebViewResizable {
		w.SetSize(*config.WebViewWidth, *config.WebViewHeight, webview.HintFixed)
	}

	webviewInstance = w
	return w
}

// NavigateWebView 让当前 WebView 导航到指定 URL
func NavigateWebView(url string) {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()
	if webviewInstance == nil {
		log.Printf("Webview[ERROR] -> WebView 未初始化")
		return
	}
	webviewInstance.Navigate(url)
}

// RunWebView 进入 WebView 事件循环（阻塞）
func RunWebView() {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()

	if webviewInstance == nil {
		log.Printf("Webview[ERROR] -> WebView 未初始化")
		return
	}
	webviewInstance.Run()
}

// CloseWebView 关闭并销毁当前 WebView 实例
func CloseWebView() {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()

	if webviewInstance == nil {
		return
	}
	webviewInstance.Terminate()
	webviewInstance = nil
}

// IsWebViewSupported 检查当前平台是否支持 WebView
func IsWebViewSupported() bool {
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		return true
	default:
		return false
	}
}
