package GeneralConfig

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
	// WebViewIconPath webview 窗口图标（.ico 文件路径，空则不设置，使用默认图标）
	WebViewIconPath = flag.String("webview-icon", "icon.ico", "webview 窗口图标 .ico 文件路径（空则不设置）")
	// WebViewCaptionColor webview 标题栏背景色（#RRGGBB，仅 Windows 11 生效）
	WebViewCaptionColor = flag.String("webview-caption-color", "", "webview 标题栏背景色，格式 #RRGGBB（仅 Windows 11 生效）")
	// WebViewBorderColor webview 窗口边框色（#RRGGBB，仅 Windows 11 生效）
	WebViewBorderColor = flag.String("webview-border-color", "", "webview 窗口边框色，格式 #RRGGBB（仅 Windows 11 生效）")
	// WebViewDarkTitleBar webview 标题栏深色模式开关（Windows 10 1809+/Windows 11）
	WebViewDarkTitleBar = flag.Bool("webview-dark-titlebar", false, "webview 标题栏使用深色模式（Win10 1809+/Win11）")
)
