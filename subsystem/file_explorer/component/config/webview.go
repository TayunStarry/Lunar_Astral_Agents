package config

import "flag"

var (
	// WebViewTitle webview 窗口标题
	WebViewTitle = flag.String("webview-title", "文件浏览器", "webview 窗口标题")
	// WebViewWidth webview 窗口宽度
	WebViewWidth = flag.Int("webview-width", 1400, "webview 窗口宽度")
	// WebViewHeight webview 窗口高度
	WebViewHeight = flag.Int("webview-height", 1050, "webview 窗口高度")
	// WebViewMinWidth webview 窗口最小宽度
	WebViewMinWidth = flag.Int("webview-min-width", 640, "webview 窗口最小宽度")
	// WebViewMinHeight webview 窗口最小高度
	WebViewMinHeight = flag.Int("webview-min-height", 640, "webview 窗口最小高度")
	// WebViewResizable webview 窗口是否可调整大小
	WebViewResizable = flag.Bool("webview-resizable", false, "webview 窗口是否可调整大小")
	// WebViewDebug webview 调试模式
	WebViewDebug = flag.Bool("webview-debug", false, "webview 调试模式")
)
