//go:build !windows

package browser

import (
	webview "github.com/webview/webview_go"
)

// applyDefaultWindowIcon 非 Windows 平台无窗口图标定制
func applyDefaultWindowIcon(w webview.WebView) {}
