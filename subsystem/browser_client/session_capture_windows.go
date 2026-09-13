//go:build windows

package BrowserClient

// 会话窗口内容截图：PrintWindow(PW_CLIENTONLY|PW_RENDERFULLCONTENT) 直接从窗口取内容，
// 不依赖窗口是否被其他窗口遮挡（屏幕 DC 区域截图在窗口被遮挡时会截到遮挡内容）。

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"
)

const (
	pwClientOnly        = 1
	pwRenderFullContent = 2 // Windows 8.1+：强制渲染直接合成内容（Chromium/WebView2 必需）
	dibRGBColors        = 0
)

type sessionBitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type sessionBitmapInfo struct {
	BmiHeader sessionBitmapInfoHeader
	BmiColors [1]uint32
}

var (
	winGdi32          = syscall.NewLazyDLL("gdi32.dll")
	procGetClientRect = winUser32.NewProc("GetClientRect")
	procGetDC         = winUser32.NewProc("GetDC")
	procReleaseDC     = winUser32.NewProc("ReleaseDC")
	procPrintWindow   = winUser32.NewProc("PrintWindow")

	procCreateCompatibleDC   = winGdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBmp  = winGdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject         = winGdi32.NewProc("SelectObject")
	procDeleteObject         = winGdi32.NewProc("DeleteObject")
	procDeleteDC             = winGdi32.NewProc("DeleteDC")
	procGetDIBits            = winGdi32.NewProc("GetDIBits")
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
	if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
		return nil, fmt.Errorf("会话窗口已最小化，无法截图")
	}

	var rc sessionRect
	if ret, _, _ := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc))); ret == 0 {
		return nil, fmt.Errorf("GetClientRect 失败")
	}
	w, h := int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("客户区尺寸无效: %dx%d", w, h)
	}

	hdcWindow, _, _ := procGetDC.Call(hwnd)
	if hdcWindow == 0 {
		return nil, fmt.Errorf("GetDC 失败")
	}
	defer procReleaseDC.Call(hwnd, hdcWindow)

	memDC, _, _ := procCreateCompatibleDC.Call(hdcWindow)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC 失败")
	}
	defer procDeleteDC.Call(memDC)

	hbm, _, _ := procCreateCompatibleBmp.Call(hdcWindow, uintptr(w), uintptr(h))
	if hbm == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap 失败")
	}
	oldObj, _, _ := procSelectObject.Call(memDC, hbm)
	defer procSelectObject.Call(memDC, oldObj)
	defer procDeleteObject.Call(hbm)

	if ret, _, callErr := procPrintWindow.Call(hwnd, memDC, uintptr(pwClientOnly|pwRenderFullContent)); ret == 0 {
		return nil, fmt.Errorf("PrintWindow 失败: %v", callErr)
	}

	bmi := sessionBitmapInfo{}
	bmi.BmiHeader.BiSize = uint32(unsafe.Sizeof(bmi.BmiHeader))
	bmi.BmiHeader.BiWidth = int32(w)
	bmi.BmiHeader.BiHeight = int32(-h) // 负高度 = 自上而下行序
	bmi.BmiHeader.BiPlanes = 1
	bmi.BmiHeader.BiBitCount = 32
	bmi.BmiHeader.BiCompression = 0 // BI_RGB

	buf := make([]byte, w*h*4)
	if ret, _, callErr := procGetDIBits.Call(memDC, hbm, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bmi)), dibRGBColors); ret == 0 {
		return nil, fmt.Errorf("GetDIBits 失败: %v", callErr)
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, j := 0, 0; i < len(buf); i, j = i+4, j+4 {
		img.Pix[j+0] = buf[i+2] // BGRA → RGBA
		img.Pix[j+1] = buf[i+1]
		img.Pix[j+2] = buf[i+0]
		img.Pix[j+3] = 255
	}
	return img, nil
}
