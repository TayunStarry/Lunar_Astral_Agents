package browser

import (
    "sync"
    webview "github.com/webview/webview_go"
)

type ipCandidate struct {
    IP        string
    Priority  int
    Interface string
}

var (
    webviewMutex     sync.Mutex
    webviewInstance  webview.WebView
    webviewRunning   bool           // 标记是否有实例在运行
    webviewCmdCh     = make(chan webviewCmd, 1)  // 向主线程发送命令
    webviewClosedCh  = make(chan struct{}, 1)    // 外部关闭通知
)

// webviewCmd 控制命令类型
type webviewCmd int
const (
    cmdQuit webviewCmd = iota
)

// WebViewClosed 返回一个 channel，当 webview 关闭时收到信号
func WebViewClosed() <-chan struct{} {
    return webviewClosedCh
}