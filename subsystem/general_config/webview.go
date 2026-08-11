package config

import "flag"

var (
	// WebViewTitle webview 窗口标题
	WebViewTitle = flag.String("webview-title", "星月智能 - 月之华", "webview 窗口标题")
	// WebViewWidth webview 窗口宽度
	WebViewWidth = flag.Int("webview-width", 345, "webview 窗口宽度")
	// WebViewHeight webview 窗口高度
	WebViewHeight = flag.Int("webview-height", 500, "webview 窗口高度")
	// WebViewMinWidth webview 窗口最小宽度
	WebViewMinWidth = flag.Int("webview-min-width", 260, "webview 窗口最小宽度")
	// WebViewMinHeight webview 窗口最小高度
	WebViewMinHeight = flag.Int("webview-min-height", 260, "webview 窗口最小高度")
	// WebViewResizable webview 窗口是否可调整大小
	WebViewResizable = flag.Bool("webview-resizable", true, "webview 窗口是否可调整大小")
)
