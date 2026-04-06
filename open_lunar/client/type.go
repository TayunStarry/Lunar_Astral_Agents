package client

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
