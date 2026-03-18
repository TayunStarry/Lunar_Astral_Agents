package browser

import (
	webview "github.com/webview/webview_go"
	"log"     // 用于记录日志信息
	"os/exec" // 用于执行外部命令
	"runtime" // 用于获取运行时的系统信息
	"time"    // 用于处理时间相关操作
)

// OpenBrowser 函数用于在不同操作系统上打开指定 URL 的浏览器
func OpenBrowser(url string) {
	// 定义要执行的命令名称
	var cmd string
	// 定义命令的参数列表
	var args []string
	// 根据不同的操作系统选择对应的浏览器打开命令
	switch runtime.GOOS {
	case "windows":
		// Windows 系统下使用 cmd 命令，/c 表示执行完命令后关闭命令行窗口，start 用于打开指定 URL
		cmd = "cmd"
		args = []string{"/c", "start", url}

	case "darwin":
		// macOS 系统下使用 open 命令打开指定 URL
		cmd = "open"
		args = []string{url}

	default:
		// 其他类 Unix 系统（如 Linux）使用 xdg-open 命令打开指定 URL
		cmd = "xdg-open"
		args = []string{url}
	}

	// 执行命令尝试打开浏览器
	if err := exec.Command(cmd, args...).Start(); err != nil {
		// 若打开失败，记录错误日志并提示手动访问
		log.Printf("Web服务[ERROR] -> %v 建议手动访问 : %s", err, url)
	}
}
func CreateWebView(config WebViewConfig) webview.WebView {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()

	if webviewInstance != nil {
		return webviewInstance
	}

	debug := false
	if config.Debug {
		debug = true
	}

	w := webview.New(debug)
	if w == nil {
		log.Printf("Webview[ERROR] -> Failed to create webview instance")
		return nil
	}

	w.SetTitle(config.Title)
	w.SetSize(config.Width, config.Height, webview.HintNone)

	if !config.Resizable {
		w.SetSize(config.Width, config.Height, webview.HintFixed)
	}

	webviewInstance = w
	webviewReady <- true

	return w
}

func NavigateWebView(url string) {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()

	if webviewInstance == nil {
		log.Printf("Webview[ERROR] -> Webview instance not initialized")
		return
	}

	webviewInstance.Navigate(url)
}

func RunWebView() {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()

	if webviewInstance == nil {
		log.Printf("Webview[ERROR] -> Webview instance not initialized")
		return
	}

	webviewInstance.Run()
}

func CloseWebView() {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()

	if webviewInstance == nil {
		return
	}

	webviewInstance.Terminate()
	webviewInstance = nil
}

func IsWebViewSupported() bool {
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		return true
	default:
		return false
	}
}

func WaitForWebViewReady(timeout time.Duration) bool {
	select {
	case <-webviewReady:
		return true
	case <-time.After(timeout):
		return false
	}
}

func GetWebViewInstance() webview.WebView {
	webviewMutex.Lock()
	defer webviewMutex.Unlock()
	return webviewInstance
}
