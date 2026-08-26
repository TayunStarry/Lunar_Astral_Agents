//go:build windows

package module

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	procGetWindowDC    = user32.NewProc("GetWindowDC")
	procGetWindowTextW = user32.NewProc("GetWindowTextW")
)

// getWindowDC 获取窗口 DC（含非客户区/标题栏）
func getWindowDC(hwnd win.HWND) win.HDC {
	ret, _, _ := procGetWindowDC.Call(uintptr(hwnd))
	return win.HDC(ret)
}

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

// captureForegroundWindow 捕获整个焦点窗口画面，返回 RGBA 图像与窗口标题
func captureForegroundWindow() (*image.RGBA, string, error) {
	hwnd, title, err := getForegroundWindow()
	if err != nil {
		return nil, "", err
	}

	var rect win.RECT
	if !win.GetWindowRect(hwnd, &rect) {
		return nil, "", fmt.Errorf("获取窗口区域失败")
	}
	width := int(rect.Right - rect.Left)
	height := int(rect.Bottom - rect.Top)
	if width <= 0 || height <= 0 {
		return nil, "", fmt.Errorf("窗口尺寸无效: %dx%d", width, height)
	}

	img, err := captureWindowSubRect(hwnd, 0, 0, width, height)
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

	var rect win.RECT
	if !win.GetWindowRect(hwnd, &rect) {
		return nil, "", fmt.Errorf("获取窗口区域失败")
	}
	winW := int(rect.Right - rect.Left)
	winH := int(rect.Bottom - rect.Top)

	if offsetX < 0 || offsetY < 0 || offsetX+width > winW || offsetY+height > winH {
		return nil, "", fmt.Errorf("窗口相对区域越界: 偏移(%d,%d)+大小(%dx%d) 超出窗口 %dx%d", offsetX, offsetY, width, height, winW, winH)
	}

	img, err := captureWindowSubRect(hwnd, offsetX, offsetY, width, height)
	if err != nil {
		return nil, "", err
	}
	return img, title, nil
}

// captureWindowSubRect 使用窗口 DC + BitBlt 捕获窗口内 (srcX, srcY) 起、大小为 (width, height) 的子区域
func captureWindowSubRect(hwnd win.HWND, srcX, srcY, width, height int) (*image.RGBA, error) {
	hdc := getWindowDC(hwnd)
	if hdc == 0 {
		return nil, fmt.Errorf("获取窗口 DC 失败")
	}
	defer win.ReleaseDC(hwnd, hdc)

	memDC := win.CreateCompatibleDC(hdc)
	if memDC == 0 {
		return nil, fmt.Errorf("创建兼容 DC 失败")
	}
	defer win.DeleteDC(memDC)

	bitmap := win.CreateCompatibleBitmap(hdc, int32(width), int32(height))
	if bitmap == 0 {
		return nil, fmt.Errorf("创建位图失败")
	}
	defer win.DeleteObject(win.HGDIOBJ(bitmap))

	old := win.SelectObject(memDC, win.HGDIOBJ(bitmap))
	if old == 0 {
		return nil, fmt.Errorf("SelectObject 失败")
	}
	defer win.SelectObject(memDC, old)

	if !win.BitBlt(memDC, 0, 0, int32(width), int32(height), hdc, int32(srcX), int32(srcY), win.SRCCOPY) {
		return nil, fmt.Errorf("BitBlt 拷贝失败")
	}

	return dibToRGBA(hdc, bitmap, width, height)
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

	// 将 DIB 位图数据视为 []byte 直接索引，避免 unsafe.Pointer(uintptr) 往返转换
	data := unsafe.Slice((*byte)(memptr), width*height*4)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < width*height*4; i += 4 {
		// BGRA -> RGBA，A 置 255
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = data[i+2], data[i+1], data[i], 255
	}
	return img, nil
}
