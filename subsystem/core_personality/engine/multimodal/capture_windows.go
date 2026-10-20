//go:build windows

package multimodal

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"

	windowcapture "LunarSubsystem/WindowCapture"
	"github.com/lxn/win"
)

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	procGetWindowTextW = user32.NewProc("GetWindowTextW")
)

// getWindowTitle 获取窗口标题
func getWindowTitle(hwnd win.HWND) string {
	buf := make([]uint16, 512)
	ret, _, _ := procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if ret == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:int(ret)])
}

// getForegroundWindow 返回焦点窗口句柄与标题
func getForegroundWindow() (win.HWND, string, error) {
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return 0, "", fmt.Errorf("未检测到焦点窗口")
	}
	if win.IsIconic(hwnd) {
		return 0, "", fmt.Errorf("焦点窗口已最小化，无法截图")
	}
	return hwnd, getWindowTitle(hwnd), nil
}

// captureForegroundWindow 捕获整个焦点窗口画面，返回 RGBA 图像与窗口标题。
// GDI 生命周期、PrintWindow 空图判定与屏幕回退统一在 WindowCapture 中实现。
func captureForegroundWindow() (*image.RGBA, string, error) {
	hwnd, title, err := getForegroundWindow()
	if err != nil {
		return nil, "", err
	}
	img, err := windowcapture.Capture(uintptr(hwnd), windowcapture.ModeWindow)
	if err != nil {
		return nil, "", err
	}
	return img, title, nil
}

// captureForegroundWindowRegion 捕获焦点窗口内相对偏移 (offsetX, offsetY) 起、大小为 (width, height) 的子区域
func captureForegroundWindowRegion(offsetX, offsetY, width, height int) (*image.RGBA, string, error) {
	hwnd, title, err := getForegroundWindow()
	if err != nil {
		return nil, "", err
	}
	img, err := windowcapture.CaptureRegion(uintptr(hwnd), windowcapture.ModeWindow, offsetX, offsetY, width, height)
	if err != nil {
		return nil, "", err
	}
	return img, title, nil
}
