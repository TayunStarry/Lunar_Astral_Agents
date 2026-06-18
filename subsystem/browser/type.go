package browser

import (
	"sync"
	"time"

	webview "github.com/webview/webview_go"
)

type ipCandidate struct {
	IP        string
	Priority  int
	Interface string
}

var (
	webviewMutex       sync.Mutex
	webviewInstance    webview.WebView
	webviewRunning     bool                     // 标记是否有实例在运行
	webviewClosedCh    = make(chan struct{}, 1) // 外部关闭通知
	webviewCleanupDone = make(chan struct{}, 1) // 清理完成信号，用于同步重开
)

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
