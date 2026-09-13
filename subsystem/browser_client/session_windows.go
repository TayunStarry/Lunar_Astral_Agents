//go:build windows

package BrowserClient

import (
	"fmt"
	"unsafe"
)

// Windows 平台下通过 user32 获取会话窗口矩形（供区域截图使用）

type sessionRect struct {
	Left, Top, Right, Bottom int32
}

var (
	procGetWindowRect = winUser32.NewProc("GetWindowRect")
	procIsIconic      = winUser32.NewProc("IsIconic")
)

// WindowRect 返回会话窗口在屏幕上的物理像素矩形（GetWindowRect 为屏幕坐标）
func (s *WebViewSession) WindowRect() (x, y, w, h int, err error) {
	if !s.IsAlive() || s.wv == nil {
		return 0, 0, 0, 0, fmt.Errorf("会话已关闭")
	}
	hwnd := uintptr(s.wv.Window())
	if hwnd == 0 {
		return 0, 0, 0, 0, fmt.Errorf("获取窗口句柄失败")
	}
	if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
		return 0, 0, 0, 0, fmt.Errorf("会话窗口已最小化，无法截图")
	}
	var r sessionRect
	ret, _, callErr := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if ret == 0 {
		return 0, 0, 0, 0, fmt.Errorf("GetWindowRect 失败: %v", callErr)
	}
	x, y = int(r.Left), int(r.Top)
	w, h = int(r.Right-r.Left), int(r.Bottom-r.Top)
	if w <= 0 || h <= 0 {
		return 0, 0, 0, 0, fmt.Errorf("窗口矩形无效: %dx%d", w, h)
	}
	return x, y, w, h, nil
}
