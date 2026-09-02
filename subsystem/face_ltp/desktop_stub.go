//go:build !windows

package FaceLTP

import (
	"fmt"
	"image"
)

// 非 Windows 平台桌面自动化桩实现：Face-LTP 需依赖 Win32 API，仅 Windows 可用。

func listWindows() []windowInfo { return nil }

func getForegroundTitle() string { return "" }

func activateWindow(title string) error {
	return fmt.Errorf("当前平台不支持桌面窗口操作")
}

func closeWindow(title string) (string, error) {
	return "", fmt.Errorf("当前平台不支持关闭窗口")
}

func launchProgram(name string) (string, error) {
	return "", fmt.Errorf("当前平台不支持启动程序")
}

func openFolder(path string) (string, error) {
	return "", fmt.Errorf("当前平台不支持打开文件夹")
}

func mouseClick(x, y int, button string, double bool) error {
	return fmt.Errorf("当前平台不支持鼠标模拟")
}

func mouseButton(x, y int, button string, holdMs int) error {
	return fmt.Errorf("当前平台不支持鼠标按键模拟")
}

func mouseMove(x, y int) error {
	return fmt.Errorf("当前平台不支持鼠标移动")
}

func typeText(text string) error {
	return fmt.Errorf("当前平台不支持键盘模拟")
}

func typeAndSend(text string) (string, error) {
	return "", fmt.Errorf("当前平台不支持键入发送")
}

func pressKey(key string) error {
	return fmt.Errorf("当前平台不支持按键模拟")
}

func scrollWheelAt(x, y int, direction string, amount int) error {
	return fmt.Errorf("当前平台不支持滚轮模拟")
}

func captureAnnotatedScreen() ([]byte, error) {
	return nil, fmt.Errorf("当前平台不支持屏幕捕获")
}

func windowCoordsToScreen(wx, wy int) (int, int, error) {
	return 0, 0, fmt.Errorf("当前平台不支持窗口操作")
}

func windowCoordsDrag(wx1, wy1, wx2, wy2 int, steps int) error {
	return fmt.Errorf("当前平台不支持拖拽操作")
}

func captureWindowThumb() (*image.RGBA, error) {
	return nil, fmt.Errorf("当前平台不支持窗口缩略图")
}

func diffWindowThumb(before, after *image.RGBA) (bool, float64, string) {
	return false, 0, "当前平台不支持画面差异检测"
}
