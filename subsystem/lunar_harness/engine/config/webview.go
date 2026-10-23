package config

var (
	// WebViewTitle webview 窗口标题
	WebViewTitle = "嵌入式 webview 窗口"
	// WebViewWidth webview 窗口宽度
	WebViewWidth = 648
	// WebViewHeight webview 窗口高度
	WebViewHeight = 648
	// WebViewMinWidth webview 窗口最小宽度
	WebViewMinWidth = 324
	// WebViewMinHeight webview 窗口最小高度
	WebViewMinHeight = 324
	// WebViewResizable webview 窗口是否可调整大小
	WebViewResizable = true
	// WebViewIconPath webview 窗口图标 .ico 文件路径（空则不设置）
	WebViewIconPath = ""
	// WebViewCaptionColor webview 标题栏背景色（#RRGGBB，仅 Windows 11 生效）
	WebViewCaptionColor = ""
	// WebViewBorderColor webview 窗口边框色（#RRGGBB，仅 Windows 11 生效）
	WebViewBorderColor = ""
	// WebViewDarkTitleBar webview 标题栏使用深色模式
	WebViewDarkTitleBar = false
)
