package parameter

import "flag"

var (
	// UseWebView 是否使用 webview 内嵌浏览器
	UseWebView = flag.Bool("use-webview", true, "是否使用 webview 内嵌浏览器")
	// WebViewTitle webview 窗口标题
	WebViewTitle = flag.String("webview-title", "< 空月辉光 - 群星韶华 >", "webview 窗口标题")
	// WebViewWidth webview 窗口宽度
	WebViewWidth = flag.Int("webview-width", 440, "webview 窗口宽度")
	// WebViewHeight webview 窗口高度
	WebViewHeight = flag.Int("webview-height", 825, "webview 窗口高度")
	// WebViewResizable webview 窗口是否可调整大小
	WebViewResizable = flag.Bool("webview-resizable", true, "webview 窗口是否可调整大小")
	// WebViewDebug webview 调试模式
	WebViewDebug = flag.Bool("webview-debug", true, "webview 调试模式")
)
