package config

import "flag"

var (
	// WebViewTitle webview 窗口标题
	WebViewTitle = flag.String("webview-title", "星月智能 - 月之华", "webview 窗口标题")
	// WebViewWidth webview 窗口宽度
	WebViewWidth = flag.Int("webview-width", 440, "webview 窗口宽度")
	// WebViewHeight webview 窗口高度
	WebViewHeight = flag.Int("webview-height", 825, "webview 窗口高度")
	// WebViewMinWidth webview 窗口最小宽度
	WebViewMinWidth = flag.Int("webview-min-width", 460, "webview 窗口最小宽度")
	// WebViewMinHeight webview 窗口最小高度
	WebViewMinHeight = flag.Int("webview-min-height", 640, "webview 窗口最小高度")
	// WebViewResizable webview 窗口是否可调整大小
	WebViewResizable = flag.Bool("webview-resizable", true, "webview 窗口是否可调整大小")
	// WebViewDebug webview 调试模式
	WebViewDebug = flag.Bool("webview-debug", true, "webview 调试模式")
	// AllowBrowser 是否允许使用浏览器
	AllowBrowser = flag.Bool("allow-browser", true, "是否允许使用浏览器")
)
