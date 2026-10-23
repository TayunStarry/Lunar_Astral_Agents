//go:build windows

package browser

// 会话窗口内容截图：具体 GDI 生命周期、PrintWindow 空图判定与屏幕回退
// 统一由 LunarSubsystem/WindowCapture 实现，本文件只负责交出 WebView 句柄。

import (
	"fmt"
	"image"

	windowcapture "LunarSubsystem/WindowCapture"
)

// WindowHandle 返回会话窗口句柄（供持有统一截图机制的调用方按句柄截图）
func (s *WebViewSession) WindowHandle() (uintptr, error) {
	if !s.IsAlive() || s.wv == nil {
		return 0, fmt.Errorf("会话已关闭")
	}
	hwnd := uintptr(s.wv.Window())
	if hwnd == 0 {
		return 0, fmt.Errorf("获取窗口句柄失败")
	}
	return hwnd, nil
}

// ClientScreenshot 截取会话窗口客户区内容（与窗口遮挡状态无关），返回 RGBA 图像
func (s *WebViewSession) ClientScreenshot() (*image.RGBA, error) {
	hwnd, err := s.WindowHandle()
	if err != nil {
		return nil, err
	}
	return windowcapture.Capture(hwnd, windowcapture.ModeClient)
}
