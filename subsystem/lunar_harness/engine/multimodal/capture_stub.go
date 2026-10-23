//go:build !windows

package multimodal

import (
	"fmt"
	"image"
)

// listWindows 非 Windows 平台不支持窗口枚举
func listWindows() ([]WindowInfo, error) {
	return nil, fmt.Errorf("窗口截图仅在 Windows 平台可用")
}

// resolveTargetWindow 非 Windows 平台不支持窗口目标定位
func resolveTargetWindow(title string, handle uint64) (uintptr, string, error) {
	return 0, "", fmt.Errorf("窗口截图仅在 Windows 平台可用")
}

// captureTargetWindowArea 非 Windows 平台不支持窗口区域捕获
func captureTargetWindowArea(hwnd uintptr, clientArea bool, offsetX, offsetY, width, height int) (*image.RGBA, error) {
	return nil, fmt.Errorf("窗口截图仅在 Windows 平台可用")
}

// CaptureWindowRGBA 非 Windows 平台不支持窗口截图
func CaptureWindowRGBA(hwnd uintptr, clientArea bool) (*image.RGBA, error) {
	return nil, fmt.Errorf("窗口截图仅在 Windows 平台可用")
}
