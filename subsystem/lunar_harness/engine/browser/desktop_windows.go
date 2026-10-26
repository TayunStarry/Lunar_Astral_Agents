//go:build windows

package browser

import (
	webview "github.com/webview/webview_go"
)

// 桌面窗口置前相关 Win32 调用
var (
	procSetForegroundWindow = winUser32.NewProc("SetForegroundWindow")
	procShowWindow          = winUser32.NewProc("ShowWindow")
	procIsWindow            = winUser32.NewProc("IsWindow")
)

// swRestore 还原最小化窗口并激活
const swRestore = 9

// applyDesktopWindowStyle 应用桌面窗口图标（需在 UI 线程、Run 之前调用）。
// 月华/琉璃前端显式传入各自图标；未指定时回退默认图标，与其他窗口保持一致。
func applyDesktopWindowStyle(w webview.WebView, iconPath string) {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		return
	}
	setWindowIcon(hwnd, iconPathOr(iconPath))
}

// desktopWindowAlive 判断 webview 窗口句柄是否仍然有效。
// webview 的 Run() 在窗口被用户关闭后可能不返回（见 desktop.go 文件头），
// 注册表会残留已销毁窗口的条目——IsWindow 校验用于识别这类死句柄。
func desktopWindowAlive(w webview.WebView) bool {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		return false
	}
	ret, _, _ := procIsWindow.Call(hwnd)
	return ret != 0
}

// focusDesktopWindow 将桌面窗口从最小化还原并置前；窗口句柄已失效时返回 false
func focusDesktopWindow(w webview.WebView) bool {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		return false
	}
	if ret, _, _ := procIsWindow.Call(hwnd); ret == 0 {
		return false
	}
	procShowWindow.Call(hwnd, swRestore)
	procSetForegroundWindow.Call(hwnd)
	return true
}
