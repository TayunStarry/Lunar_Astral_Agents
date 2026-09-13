//go:build !windows

package BrowserClient

import (
	"fmt"
	"image"
)

// WindowRect 非 Windows 平台暂不支持会话窗口矩形
func (s *WebViewSession) WindowRect() (x, y, w, h int, err error) {
	return 0, 0, 0, 0, fmt.Errorf("当前平台不支持会话窗口截图")
}

// ClientScreenshot 非 Windows 平台暂不支持会话窗口内容截图
func (s *WebViewSession) ClientScreenshot() (*image.RGBA, error) {
	return nil, fmt.Errorf("当前平台不支持会话窗口截图")
}
