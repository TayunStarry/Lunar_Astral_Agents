//go:build !windows

package BrowserClient

import (
	webview "github.com/webview/webview_go"
)

// applyDesktopWindowStyle 非 Windows 平台无窗口图标定制
func applyDesktopWindowStyle(w webview.WebView, iconPath string) {}

// desktopWindowAlive 非 Windows 平台无从校验句柄，视为存活
func desktopWindowAlive(w webview.WebView) bool { return true }

// focusDesktopWindow 非 Windows 平台无置前能力，视为已聚焦
func focusDesktopWindow(w webview.WebView) bool { return true }