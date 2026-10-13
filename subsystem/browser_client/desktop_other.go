//go:build !windows

package BrowserClient

import (
	webview "github.com/webview/webview_go"
)

// applyDesktopWindowStyle 非 Windows 平台无窗口图标定制
func applyDesktopWindowStyle(w webview.WebView, iconPath string) {}

// focusDesktopWindow 非 Windows 平台无置前能力
func focusDesktopWindow(w webview.WebView) {}