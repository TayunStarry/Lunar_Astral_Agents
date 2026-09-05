//go:build windows

package AutoLTP

// ==== 窗口截图与差异检测 ====
// 负责前台窗口截图、缩放编码，以及操作前后画面差异判断。

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strconv"

	imageproc "LunarSubsystem/ImageProcessor/module"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/lxn/win"
)

// DTCaptureAnnotated 截取当前前台窗口，返回叠加「原始窗口像素坐标」网格后的 PNG 字节。
// 网格在最终出图分辨率（缩放后 768 上限）上加粗高对比绘制，缩放到模型输入后依旧清晰；
// 刻度值按截取前的原始窗口宽高换算，视觉/规划/鼠标操作者报告与使用的 (x,y) 就直接等于
// click/mouse_hold/move_mouse/scroll_wheel 的窗口内相对坐标，避免截图缩放造成的坐标偏差。
func DTCaptureAnnotated() ([]byte, error) {
	img, origW, origH, err := dtCaptureWindowRGBA()
	if err != nil {
		return nil, err
	}
	dtDrawCoordGrid(img, origW, origH)
	return encodePNG(img), nil
}

// dtCaptureWindowRGBA 截取前台窗口并缩放到 768 上限尺寸的 RGBA 图，同时返回原始窗口宽高。
func dtCaptureWindowRGBA() (*image.RGBA, int, int, error) {
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return nil, 0, 0, fmt.Errorf("无前台窗口")
	}
	var rect win.RECT
	if !win.GetWindowRect(hwnd, &rect) {
		return nil, 0, 0, fmt.Errorf("获取窗口区域失败")
	}
	w := int(rect.Right - rect.Left)
	h := int(rect.Bottom - rect.Top)
	if w <= 0 || h <= 0 {
		return nil, 0, 0, fmt.Errorf("窗口尺寸无效: %dx%d", w, h)
	}
	img, err := imageproc.CaptureScreenRegionRGBA(int(rect.Left), int(rect.Top), w, h)
	if err != nil {
		return nil, 0, 0, err
	}
	return imageproc.ResizeToFit(img, 768, 768), w, h, nil
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

// ==== 截图坐标网格绘制 ====

// gridDivs 每轴的网格划分数。格数固定且较少，保证缩放图上格距足够大，缩小后依旧可辨。
const gridDivs = 10

// gridLineW 网格线厚度（像素）。加粗绘制以增强截图/模型缩放后的可见性。
const gridLineW = 2

// dtFillRect 用指定颜色填充图像内矩形区域（自动裁剪到图像边界外的部分）。
func dtFillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	r := image.Rect(x0, y0, x1, y1).Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	draw.Draw(img, r, image.NewUniform(c), image.ZP, draw.Src)
}

// dtDrawText 在图像上以 basicfont 绘制文本；先铺半透明背景条，保证文字在任意画面上清晰。
func dtDrawText(img *image.RGBA, s string, x, y int, fg, bg color.RGBA) {
	face := basicfont.Face7x13
	w := font.MeasureString(face, s).Ceil()
	h := 13
	dtFillRect(img, x, y, x+w, y+h, bg)
	d := &font.Drawer{Dst: img, Src: image.NewUniform(fg), Face: face, Dot: fixed.P(x, y+12)}
	d.DrawString(s)
}

// dtDrawCoordGrid 在缩放后的截图 img 上叠加「原始窗口像素坐标」网格。
// 网格线位置与刻度值都按原始窗口宽高 origW/origH 等比例换算：X 刻度沿顶边、Y 刻度沿左边，
// 模型读到的刻度值即为 window 内相对坐标，可直接喂给 click/mouse_hold/move_mouse/scroll_wheel 的 x/y 参数。
func dtDrawCoordGrid(img *image.RGBA, origW, origH int) {
	b := img.Bounds()
	W, H := b.Dx(), b.Dy()
	if W <= 0 || H <= 0 || origW <= 0 || origH <= 0 {
		return
	}
	line := color.RGBA{R: 0, G: 255, B: 235, A: 240}    // 网格线：青色加粗、高对比
	labelBG := color.RGBA{R: 0, G: 0, B: 0, A: 150}     // 刻度背景：半透明黑，垫底提升可读性
	labelFG := color.RGBA{R: 255, G: 255, B: 255, A: 255} // 刻度文字：纯白

	// 纵向网格线与 X 刻度（顶边）
	for i := 1; i < gridDivs; i++ {
		sx := int(float64(i) * float64(W) / float64(gridDivs))
		dtFillRect(img, sx, 0, sx+gridLineW, H, line)
		origX := int(float64(i) * float64(origW) / float64(gridDivs))
		dtDrawText(img, strconv.Itoa(origX), sx+gridLineW+1, 0, labelFG, labelBG)
	}
	// 横向网格线与 Y 刻度（左边）
	for i := 1; i < gridDivs; i++ {
		sy := int(float64(i) * float64(H) / float64(gridDivs))
		dtFillRect(img, 0, sy, W, sy+gridLineW, line)
		origY := int(float64(i) * float64(origH) / float64(gridDivs))
		dtDrawText(img, strconv.Itoa(origY), 0, sy+gridLineW, labelFG, labelBG)
	}
	// 顶边最左端补一个 X=0 的明确参照（原点在窗口左上角）
	dtDrawText(img, "0", 1, 0, labelFG, labelBG)
}
