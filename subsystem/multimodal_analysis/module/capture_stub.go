//go:build !windows

package module

import (
	"fmt"
	"image"
)

// captureForegroundWindow 非 Windows 平台不支持窗口捕获
func captureForegroundWindow() (*image.RGBA, string, error) {
	return nil, "", fmt.Errorf("窗口截图仅在 Windows 平台可用")
}

// captureForegroundWindowRegion 非 Windows 平台不支持窗口区域捕获
func captureForegroundWindowRegion(offsetX, offsetY, width, height int) (*image.RGBA, string, error) {
	return nil, "", fmt.Errorf("窗口截图仅在 Windows 平台可用")
}
