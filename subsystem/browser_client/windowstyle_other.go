//go:build !windows

package BrowserClient

import (
	"LunarSubsystem/LoggerGeneral"

	webview "github.com/webview/webview_go"
)

// applyWindowStyle 非 Windows 平台无原生标题栏可配置，直接跳过
func applyWindowStyle(w webview.WebView) {
	LoggerGeneral.SubInfo("BrowserClient", "applyWindowStyle", "当前平台不支持窗口图标/标题栏样式设置，跳过")
}
