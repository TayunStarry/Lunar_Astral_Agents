package browser

import (
	"log"
	"os/exec"
	"runtime"
	"sync"

	webview "github.com/webview/webview_go"
)

var (
	webviewMutex    sync.Mutex
	webviewInstance webview.WebView
)

// WebViewConfig 包含创建 WebView 所需的基本配置
type WebViewConfig struct {
	Title     string
	Width     int
	Height    int
	Resizable bool
	Debug     bool
}

// OpenBrowser 在系统默认浏览器中打开指定 URL
func OpenBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux, freebsd, etc.
		cmd = "xdg-open"
		args = []string{url}
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		log.Printf("Web服务[ERROR] -> %v 建议手动访问: %s", err, url)
	}
}

// CreateWebView 创建并返回一个 WebView 实例（单例模式）
func CreateWebView(config WebViewConfig) webview.WebView {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()

	if webviewInstance != nil {
		return webviewInstance
	}

	w := webview.New(config.Debug)
	if w == nil {
		log.Printf("Webview[ERROR] -> 无法创建 WebView 实例")
		return nil
	}

	w.SetTitle(config.Title)
	w.SetSize(config.Width, config.Height, webview.HintNone)

	if !config.Resizable {
		w.SetSize(config.Width, config.Height, webview.HintFixed)
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