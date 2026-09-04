//go:build windows

package AutoLTP

// ==== 窗口截图与差异检测 ====
// 负责前台窗口截图、缩放编码，以及操作前后画面差异判断。

import (
	"bytes"
	"fmt"
	"image"
	"image/png"

	imageproc "LunarSubsystem/ImageProcessor/module"

	"github.com/lxn/win"
)

// DTCaptureAnnotated 截取当前前台窗口，返回叠加坐标网格后的 PNG 字节。
func DTCaptureAnnotated() ([]byte, error) {
	img, err := dtCaptureWindowRGBA()
	if err != nil {
		return nil, err
	}
	return encodePNG(img), nil
}

// dtCaptureWindowRGBA 截取前台窗口并缩放到 768 上限尺寸的 RGBA 图。
func dtCaptureWindowRGBA() (*image.RGBA, error) {
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return nil, fmt.Errorf("无前台窗口")
	}
	var rect win.RECT
	if !win.GetWindowRect(hwnd, &rect) {
		return nil, fmt.Errorf("获取窗口区域失败")
	}
	w := int(rect.Right - rect.Left)
	h := int(rect.Bottom - rect.Top)
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("窗口尺寸无效: %dx%d", w, h)
	}
	img, err := imageproc.CaptureScreenRegionRGBA(int(rect.Left), int(rect.Top), w, h)
	if err != nil {
		return nil, err
	}
	img = imageproc.ResizeToFit(img, 768, 768)
	return img, nil
}

// encodePNG 将 RGBA 图像编码为 PNG 字节。
func encodePNG(img *image.RGBA) []byte {
	buf := &bytes.Buffer{}
	_ = png.Encode(buf, img)
	return buf.Bytes()
}

// dtCaptureWindowSized 截取前台窗口并缩放到指定最大边长，供差异检测使用。
func dtCaptureWindowSized(maxDim int) (*image.RGBA, error) {
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return nil, fmt.Errorf("无前台窗口")
	}
	var rect win.RECT
	if !win.GetWindowRect(hwnd, &rect) {
		return nil, fmt.Errorf("获取窗口区域失败")
	}
	w := int(rect.Right - rect.Left)
	h := int(rect.Bottom - rect.Top)
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("窗口尺寸无效: %dx%d", w, h)
	}
	img, err := imageproc.CaptureScreenRegionRGBA(int(rect.Left), int(rect.Top), w, h)
	if err != nil {
		return nil, fmt.Errorf("窗口区域截图失败: %v", err)
	}
	return imageproc.ResizeToFit(img, maxDim, maxDim), nil
}

// dtCaptureWindowThumb 截取前台窗口缩小版缩略图，用于操作前后差异对比。
func dtCaptureWindowThumb() (*image.RGBA, error) {
	return dtCaptureWindowSized(96)
}

// DTDiffWindowThumb 对比操作前后两张窗口缩略图，返回是否有变化、变化占比与描述。
func DTDiffWindowThumb(before, after *image.RGBA) (changed bool, ratio float64, detail string) {
	if before == nil || after == nil {
		return false, 0, "无操作前后对照图"
	}
	b := before.Bounds()
	a := after.Bounds()
	if b.Dx() != a.Dx() || b.Dy() != a.Dy() {
		return true, 1, "前后窗口尺寸不同（窗口可能已变化）"
	}
	total := b.Dx() * b.Dy()
	if total == 0 {
		return false, 0, "窗口为空"
	}
	changedPix := 0
	thr := 3 * 12 * 12
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c1 := before.RGBAAt(x, y)
			c2 := after.RGBAAt(x, y)
			dr := int(c1.R) - int(c2.R)
			dg := int(c1.G) - int(c2.G)
			db := int(c1.B) - int(c2.B)
			if dr*dr+dg*dg+db*db > thr {
				changedPix++
			}
		}
	}
	ratio = float64(changedPix) / float64(total)
	if ratio < 0.003 {
		return false, ratio, "画面几乎无变化"
	}
	return true, ratio, fmt.Sprintf("画面发生变化（变化像素占比 %.1f%%）", ratio*100)
}
