//go:build !windows

package browser

import (
	webview "github.com/webview/webview_go"
)

// applyDesktopWindowStyle 非 Windows 平台无窗口图标定制
func applyDesktopWindowStyle(w webview.WebView, iconPath string) {}

// desktopWindowAlive 非 Windows 平台无从校验句柄，视为存活
func desktopWindowAlive(w webview.WebView) bool { return true }

// windowContentWidth 非 Windows 平台不支持内容区宽度读取
func windowContentWidth(hwnd uintptr) int { return 0 }

// setWindowTopmost 非 Windows 平台不支持置顶切换
func setWindowTopmost(hwnd uintptr, topmost bool) {}

// focusDesktopWindow 非 Windows 平台无置前能力，视为已聚焦
func focusDesktopWindow(w webview.WebView) bool { return true }
