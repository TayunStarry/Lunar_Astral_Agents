package BrowserClient

import (
	"sync"

	webview "github.com/webview/webview_go"
)

var (
	webviewMutex       sync.Mutex
	webviewInstance    webview.WebView
	webviewRunning     bool                     // 标记是否有实例在运行
	webviewClosedCh    = make(chan struct{}, 1) // 外部关闭通知
	webviewCleanupDone = make(chan struct{}, 1) // 清理完成信号，用于同步重开
)
