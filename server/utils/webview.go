package utils

import (
	"log"
	"runtime"
	"sync"
	"time"

	webview "github.com/webview/webview_go"
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
