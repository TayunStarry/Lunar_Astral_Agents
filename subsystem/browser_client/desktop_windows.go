//go:build windows

package BrowserClient

import (
	webview "github.com/webview/webview_go"
)

// 桌面窗口置前相关 Win32 调用
var (
	procSetForegroundWindow = winUser32.NewProc("SetForegroundWindow")
	procShowWindow          = winUser32.NewProc("ShowWindow")
)

// swRestore 还原最小化窗口并激活
const swRestore = 9

// applyDesktopWindowStyle 应用桌面窗口图标（需在 UI 线程、Run 之前调用）
func applyDesktopWindowStyle(w webview.WebView, iconPath string) {
	if iconPath == "" {
		return
	}
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		return
	}
	setWindowIcon(hwnd, iconPath)
}

// focusDesktopWindow 将桌面窗口从最小化还原并置前
func focusDesktopWindow(w webview.WebView) {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		return
	}
	procShowWindow.Call(hwnd, swRestore)
	procSetForegroundWindow.Call(hwnd)
}