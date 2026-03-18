package browser

// 引入必要的包
import (
	webview "github.com/webview/webview_go"
	"sync"
)

var (
	webviewInstance webview.WebView
	webviewMutex    sync.Mutex
	webviewReady    = make(chan bool, 1)
)

type WebViewConfig struct {
	Title     string
	Width     int
	Height    int
	Resizable bool
	Debug     bool
}
