//go:build windows

package BrowserClient

// 会话窗口内容截图：具体 GDI 生命周期、PrintWindow 空图判定与屏幕回退
// 统一由 LunarSubsystem/WindowCapture 实现，本文件只负责交出 WebView 句柄。

import (
	"fmt"
	"image"

	windowcapture "LunarSubsystem/WindowCapture"
)

// ClientScreenshot 截取会话窗口客户区内容（与窗口遮挡状态无关），返回 RGBA 图像
func (s *WebViewSession) ClientScreenshot() (*image.RGBA, error) {
	if !s.IsAlive() || s.wv == nil {
		return nil, fmt.Errorf("会话已关闭")
	}
	hwnd := uintptr(s.wv.Window())
	if hwnd == 0 {
		return nil, fmt.Errorf("获取窗口句柄失败")
	}
	return windowcapture.Capture(hwnd, windowcapture.ModeClient)
}
