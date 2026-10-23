//go:build windows

package capture

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"

	"github.com/kbinani/screenshot"
	"github.com/lxn/win"
)

const (
	pwClientOnly        = 0x00000001
	pwRenderFullContent = 0x00000002 // Windows 8.1+：渲染 DWM/GPU 合成内容
)

var (
	winUser32 = syscall.NewLazyDLL("user32.dll")
	winGdi32  = syscall.NewLazyDLL("gdi32.dll")

	procPrintWindow = winUser32.NewProc("PrintWindow")

	procCreateCompatibleDC  = winGdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBmp = winGdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject        = winGdi32.NewProc("SelectObject")
	procDeleteObject        = winGdi32.NewProc("DeleteObject")
	procDeleteDC            = winGdi32.NewProc("DeleteDC")
	procGetDIBits           = winGdi32.NewProc("GetDIBits")
)

// screenRect 屏幕坐标下的窗口/客户区矩形
type screenRect struct {
	x, y, w, h int
}

// RectOnScreen 返回窗口或客户区在屏幕上的物理像素矩形
func RectOnScreen(hwnd uintptr, mode Mode) (x, y, w, h int, err error) {
	rect, err := rectOnScreen(hwnd, mode)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return rect.x, rect.y, rect.w, rect.h, nil
}

// Capture 捕获整个窗口或客户区
func Capture(hwnd uintptr, mode Mode) (*image.RGBA, error) {
	rect, err := rectOnScreen(hwnd, mode)
	if err != nil {
		return nil, err
	}
	return captureArea(hwnd, mode, rect, 0, 0, rect.w, rect.h)
}

// CaptureRegion 捕获窗口/客户区内相对 (rx, ry) 起、大小为 (rw, rh) 的子区域
func CaptureRegion(hwnd uintptr, mode Mode, rx, ry, rw, rh int) (*image.RGBA, error) {
	rect, err := rectOnScreen(hwnd, mode)
	if err != nil {
		return nil, err
	}
	if rx < 0 || ry < 0 || rw <= 0 || rh <= 0 || rx+rw > rect.w || ry+rh > rect.h {
		return nil, fmt.Errorf("截图区域越界: 偏移(%d,%d)+大小(%dx%d) 超出 %dx%d",
			rx, ry, rw, rh, rect.w, rect.h)
	}
	return captureArea(hwnd, mode, rect, rx, ry, rw, rh)
}

// rectOnScreen 计算窗口/客户区矩形（内部使用）
func rectOnScreen(hwnd uintptr, mode Mode) (screenRect, error) {
	if hwnd == 0 {
		return screenRect{}, fmt.Errorf("窗口句柄为空")
	}
	h := win.HWND(hwnd)
	if win.IsIconic(h) {
		return screenRect{}, fmt.Errorf("窗口已最小化，无法截图")
	}

	switch mode {
	case ModeClient:
		var rc win.RECT
		if !win.GetClientRect(h, &rc) {
			return screenRect{}, fmt.Errorf("GetClientRect 失败")
		}
		w, hgt := int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
		if w <= 0 || hgt <= 0 {
			return screenRect{}, fmt.Errorf("客户区尺寸无效: %dx%d", w, hgt)
		}
		var pt win.POINT
		if !win.ClientToScreen(h, &pt) {
			return screenRect{}, fmt.Errorf("ClientToScreen 失败")
		}
		return screenRect{x: int(pt.X), y: int(pt.Y), w: w, h: hgt}, nil
	case ModeWindow:
		var rc win.RECT
		if !win.GetWindowRect(h, &rc) {
			return screenRect{}, fmt.Errorf("GetWindowRect 失败")
		}
		w, hgt := int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
		if w <= 0 || hgt <= 0 {
			return screenRect{}, fmt.Errorf("窗口尺寸无效: %dx%d", w, hgt)
		}
		return screenRect{x: int(rc.Left), y: int(rc.Top), w: w, h: hgt}, nil
	default:
		return screenRect{}, fmt.Errorf("不支持的截图模式: %d", mode)
	}
}

// captureArea 先 PrintWindow 取内容，纯色或失败时回退屏幕可见区域
func captureArea(hwnd uintptr, mode Mode, rect screenRect, rx, ry, rw, rh int) (*image.RGBA, error) {
	var printImg *image.RGBA
	var printErr error

	full, err := captureViaPrint(hwnd, mode, rect.w, rect.h)
	if err != nil {
		printErr = err
	} else if rx == 0 && ry == 0 && rw == rect.w && rh == rect.h {
		printImg = full
	} else if cropped, cropErr := cropRGBA(full, rx, ry, rw, rh); cropErr == nil {
		printImg = cropped
	} else {
		printErr = cropErr
	}

	if printImg != nil && !IsUniform(printImg) {
		return printImg, nil
	}

	// 屏幕可见区域：窗口被遮挡时会截到遮挡物，但能正确拿到 GPU 窗口的真实画面
	screenImage, screenErr := screenshot.CaptureRect(image.Rect(
		rect.x+rx,
		rect.y+ry,
		rect.x+rx+rw,
		rect.y+ry+rh,
	))
	if screenErr == nil && screenImage != nil {
		return screenImage, nil
	}

	if printImg != nil {
		return printImg, nil
	}
	if printErr != nil && screenErr != nil {
		return nil, fmt.Errorf("窗口截图失败（PrintWindow 与屏幕捕获均失败）: %v / %v", printErr, screenErr)
	}
	return nil, fmt.Errorf("窗口截图失败: %v", screenErr)
}

// captureViaPrint 使用 PrintWindow 将窗口/客户区画入兼容位图
func captureViaPrint(hwnd uintptr, mode Mode, width, height int) (*image.RGBA, error) {
	screenDC := win.GetDC(0)
	if screenDC == 0 {
		return nil, fmt.Errorf("获取屏幕 DC 失败")
	}
	defer win.ReleaseDC(0, screenDC)

	memDC := win.CreateCompatibleDC(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("创建兼容 DC 失败")
	}
	defer win.DeleteDC(memDC)

	bitmap := win.CreateCompatibleBitmap(screenDC, int32(width), int32(height))
	if bitmap == 0 {
		return nil, fmt.Errorf("创建位图失败")
	}
	defer win.DeleteObject(win.HGDIOBJ(bitmap))

	old := win.SelectObject(memDC, win.HGDIOBJ(bitmap))
	if old == 0 {
		return nil, fmt.Errorf("SelectObject 失败")
	}
	defer win.SelectObject(memDC, old)

	// 先请求完整内容；老系统不支持该标志时退回仅客户区/无标志
	flags := uintptr(pwRenderFullContent)
	if mode == ModeClient {
		flags |= pwClientOnly
	}
	ret, _, _ := procPrintWindow.Call(hwnd, uintptr(memDC), flags)
	if ret == 0 && mode == ModeClient {
		// 客户区模式不能退到 0：那样会把整个窗口（含标题栏）画进客户区大小的位图，内容错位。
		ret, _, _ = procPrintWindow.Call(hwnd, uintptr(memDC), uintptr(pwClientOnly))
	}
	if ret == 0 && mode == ModeWindow {
		ret, _, _ = procPrintWindow.Call(hwnd, uintptr(memDC), 0)
	}
	if ret == 0 {
		return nil, fmt.Errorf("PrintWindow 失败")
	}

	return dibToRGBA(screenDC, bitmap, width, height)
}

// cropRGBA 从图像 (x, y) 起裁剪出 w×h 的子图（返回新的 RGBA，避免共享底层数组）
func cropRGBA(src *image.RGBA, x, y, w, h int) (*image.RGBA, error) {
	if src == nil {
		return nil, fmt.Errorf("源图像为空")
	}
	bounds := src.Bounds()
	if x < 0 || y < 0 || w <= 0 || h <= 0 || x+w > bounds.Dx() || y+h > bounds.Dy() {
		return nil, fmt.Errorf("裁剪区域越界: (%d,%d)+%dx%d 超出 %dx%d", x, y, w, h, bounds.Dx(), bounds.Dy())
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	rowBytes := w * 4
	for row := 0; row < h; row++ {
		srcStart := (y+row)*src.Stride + x*4
		copy(dst.Pix[row*dst.Stride:row*dst.Stride+rowBytes], src.Pix[srcStart:srcStart+rowBytes])
	}
	return dst, nil
}

// dibToRGBA 将 HBITMAP 通过 GetDIBits 提取为 RGBA 图像（BGRA → RGBA 转换）
func dibToRGBA(hdc win.HDC, bitmap win.HBITMAP, width, height int) (*image.RGBA, error) {
	var header win.BITMAPINFOHEADER
	header.BiSize = uint32(unsafe.Sizeof(header))
	header.BiPlanes = 1
	header.BiBitCount = 32
	header.BiWidth = int32(width)
	header.BiHeight = int32(-height)
	header.BiCompression = win.BI_RGB
	header.BiSizeImage = 0

	bitmapDataSize := uintptr(((int64(width)*int64(header.BiBitCount) + 31) / 32) * 4 * int64(height))
	hmem := win.GlobalAlloc(win.GMEM_MOVEABLE, bitmapDataSize)
	if hmem == 0 {
		return nil, fmt.Errorf("分配内存失败")
	}
	defer win.GlobalFree(hmem)

	memptr := win.GlobalLock(hmem)
	if memptr == nil {
		return nil, fmt.Errorf("锁定内存失败")
	}
	defer win.GlobalUnlock(hmem)

	if win.GetDIBits(hdc, bitmap, 0, uint32(height), (*byte)(memptr), (*win.BITMAPINFO)(unsafe.Pointer(&header)), win.DIB_RGB_COLORS) == 0 {
		return nil, fmt.Errorf("GetDIBits 失败")
	}

	data := unsafe.Slice((*byte)(memptr), width*height*4)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < width*height*4; i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = data[i+2], data[i+1], data[i], 255
	}
	return img, nil
}
