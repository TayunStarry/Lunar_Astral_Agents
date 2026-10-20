// Package windowcapture 提供 Windows 窗口/客户区截图的统一实现。
//
// 设计目标：
//   - 统一 GDI 生命周期：DC、兼容位图、SelectObject/DeleteObject/DeleteDC 成对释放；
//   - 统一空图判定：PrintWindow 对部分 DWM/GPU 窗口会“成功返回纯黑/纯白”，
//     统一识别后回退屏幕可见区域，避免调用方各自判断；
//   - 统一两种模式：ModeWindow（整个窗口，含标题栏/边框）与 ModeClient（仅客户区）。
//
// 调用方只需要提供窗口句柄（uintptr），不需要接触 win32 类型与 GDI 句柄。
package windowcapture

import "image"

// Mode 截图范围模式
type Mode int

const (
	// ModeWindow 捕获整个窗口矩形（含标题栏、边框，与 GetWindowRect 一致）
	ModeWindow Mode = iota
	// ModeClient 捕获客户区（与 GetClientRect / ClientToScreen 一致，不含标题栏）
	ModeClient
)

// IsUniform 判断图像是否整张只有一种颜色。
//
// 这是 PrintWindow 的兜底判据：部分 DWM/GPU 合成窗口（WebView2、Electron、
// 视频等）会让 PrintWindow 返回成功，但整张位图是纯黑/纯白。调用方应把
// “纯色”视为不可用，改走屏幕可见区域截图。真正的纯色页面也会被判为纯色，
// 此时回退屏幕截图得到的仍是同一画面，不会产生副作用。
func IsUniform(img *image.RGBA) bool {
	if img == nil || len(img.Pix) < 4 || img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		return true
	}
	r, g, b, a := img.Pix[0], img.Pix[1], img.Pix[2], img.Pix[3]
	for i := 4; i+3 < len(img.Pix); i += 4 {
		if img.Pix[i] != r || img.Pix[i+1] != g || img.Pix[i+2] != b || img.Pix[i+3] != a {
			return false
		}
	}
	return true
}
