package browser

import (
	"sync"

	webview "github.com/webview/webview_go"
)

// ipCandidate IP地址候选对象
type ipCandidate struct {
	IP        string
	Priority  int
	Interface string
}

// webviewMutex WebView 控制互斥锁
var webviewMutex sync.Mutex

// webviewInstance WebView 实例
var webviewInstance webview.WebView

// webviewClosedChan 用于通知 webview 已关闭
var webviewClosedChan = make(chan struct{}, 1)

// WebViewClosed 返回一个 channel，用于接收 webview 关闭事件
func WebViewClosed() <-chan struct{} {
	return webviewClosedChan
}
