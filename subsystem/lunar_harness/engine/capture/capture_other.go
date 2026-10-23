//go:build !windows

package capture

import (
	"fmt"
	"image"
)

// RectOnScreen 非 Windows 平台暂不支持窗口截图
func RectOnScreen(hwnd uintptr, mode Mode) (x, y, w, h int, err error) {
	return 0, 0, 0, 0, fmt.Errorf("当前平台不支持窗口截图")
}

// Capture 非 Windows 平台暂不支持窗口截图
func Capture(hwnd uintptr, mode Mode) (*image.RGBA, error) {
	return nil, fmt.Errorf("当前平台不支持窗口截图")
}

// CaptureRegion 非 Windows 平台暂不支持窗口截图
func CaptureRegion(hwnd uintptr, mode Mode, rx, ry, rw, rh int) (*image.RGBA, error) {
	return nil, fmt.Errorf("当前平台不支持窗口截图")
}
