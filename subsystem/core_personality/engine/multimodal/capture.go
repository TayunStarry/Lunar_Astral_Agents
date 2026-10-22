package multimodal

import (
	"LunarSubsystem/GeneralConfig"
	"bytes"
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/kbinani/screenshot"
)

// Capture 统一截图入口：按 CaptureRequest 的 mode 执行截图并返回结构化结果
//
// 优先级路由规则：
//   - auto：目标窗口（显式句柄/标题，缺省为焦点窗口）优先；识别失败降级为多屏拼接全屏
//   - window：强制目标窗口，识别失败直接报错
//   - fullscreen：多屏拼接全屏
//   - display：指定显示器（display_index=-1 表示全部显示器拼接）
//   - region：绝对屏幕坐标区域
//
// 目标窗口识别顺序：WindowHandle > WindowTitle（标题包含匹配）> 当前焦点窗口。
// 目标窗口走 WindowCapture 的 PrintWindow 渲染，因此被遮挡（未最小化）也能拿到画面。
//
// 坐标覆盖：目标窗口模式下 Width>0 && Height>0 时，
// 按窗口相对偏移 (OffsetX, OffsetY) + 区域大小 (Width, Height) 精准截取子区域。
func Capture(req CaptureRequest) (CaptureResult, error) {
	// 频率限制
	if err := checkScreenshotRateLimit(); err != nil {
		return CaptureResult{}, err
	}

	// 默认值
	if req.Format == "" {
		req.Format = *GeneralConfig.Format
	}
	if req.Quality == 0 {
		req.Quality = *GeneralConfig.JPEGQuality
	}
	if req.Mode == "" {
		req.Mode = ModeAuto
	}

	var (
		img          *image.RGBA
		usedMode     CaptureMode
		windowTitle  string
		windowHandle uint64
		displayIdx   int
		err          error
	)

	switch req.Mode {
	case ModeAuto, ModeWindow:
		img, windowTitle, windowHandle, err = captureTargetWindow(req)
		if err != nil {
			if req.Mode == ModeWindow {
				return CaptureResult{}, err
			}
			// auto 降级为多屏拼接全屏
			img, err = screenshotAllDisplaysOptimized()
			if err != nil {
				return CaptureResult{}, fmt.Errorf("窗口截图失败，全屏降级也失败: %v", err)
			}
			usedMode = ModeFullscreen
		} else {
			usedMode = ModeWindow
		}

	case ModeFullscreen:
		img, err = screenshotAllDisplaysOptimized()
		usedMode = ModeFullscreen

	case ModeDisplay:
		img, err = captureDisplay(req.DisplayIndex)
		usedMode = ModeDisplay
		displayIdx = req.DisplayIndex

	case ModeRegion:
		img, err = captureRegion(req)
		usedMode = ModeRegion

	default:
		return CaptureResult{}, fmt.Errorf("不支持的截图模式: %s", req.Mode)
	}

	if err != nil {
		return CaptureResult{}, err
	}

	// 缩放处理
	img, err = applyScale(img, req.Scale)
	if err != nil {
		return CaptureResult{}, fmt.Errorf("缩放失败: %v", err)
	}

	// 编码图片
	buf := &bytes.Buffer{}
	if err := encodeImage(buf, img, req.Format, req.Quality); err != nil {
		return CaptureResult{}, err
	}

	// 更新最后截图时间
	ScreenshotMutex.Lock()
	LastCapture = time.Now().UnixNano()
	ScreenshotMutex.Unlock()

	return CaptureResult{
		Image:        buf.Bytes(),
		Format:       normalizeFormat(req.Format),
		ContentType:  getContentType(req.Format),
		Width:        img.Bounds().Dx(),
		Height:       img.Bounds().Dy(),
		Mode:         usedMode,
		DisplayIndex: displayIdx,
		WindowTitle:  windowTitle,
		WindowHandle: windowHandle,
	}, nil
}

// ListWindows 枚举可作为截图目标的可见顶层窗口（Windows 平台）。
// 最小化窗口也会列出，但 capture 时会被明确拒绝，便于智能体提示用户先恢复窗口。
func ListWindows() ([]WindowInfo, error) {
	return listWindows()
}

// captureTargetWindow 解析目标窗口并截图（含窗口相对精准区域覆盖）
func captureTargetWindow(req CaptureRequest) (*image.RGBA, string, uint64, error) {
	hwnd, title, err := resolveTargetWindow(req.WindowTitle, req.WindowHandle)
	if err != nil {
		return nil, "", 0, err
	}
	img, err := captureTargetWindowArea(hwnd, req.ClientArea, req.OffsetX, req.OffsetY, req.Width, req.Height)
	if err != nil {
		return nil, "", 0, err
	}
	return img, title, uint64(hwnd), nil
}

// captureDisplay 捕获指定显示器；index<0 时返回全部显示器拼接
func captureDisplay(index int) (*image.RGBA, error) {
	if index < 0 {
		return screenshotAllDisplaysOptimized()
	}
	n := screenshot.NumActiveDisplays()
	if index >= n {
		return nil, fmt.Errorf("无效的显示器索引: %d（共 %d 个显示器）", index, n)
	}
	return screenshot.CaptureDisplay(index)
}

// captureRegion 捕获绝对屏幕坐标区域
func captureRegion(req CaptureRequest) (*image.RGBA, error) {
	if req.RegionW <= 0 || req.RegionH <= 0 {
		return nil, fmt.Errorf("区域宽高必须大于 0")
	}
	rect := image.Rect(req.RegionX, req.RegionY, req.RegionX+req.RegionW, req.RegionY+req.RegionH)
	return screenshot.CaptureRect(rect)
}

// CaptureScreenRegionRGBA 截取屏幕绝对坐标区域的原始 RGBA（基于屏幕 DC，可正确截到硬件加速窗口内容），
// 供外部智能体对指定窗口区域截图并叠加坐标标注使用，体积小于全屏截图。
func CaptureScreenRegionRGBA(x, y, w, h int) (*image.RGBA, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("区域宽高必须大于 0")
	}
	return screenshot.CaptureRect(image.Rect(x, y, x+w, y+h))
}

// normalizeFormat 归一化图片格式名称（jpg/jpeg → jpeg）
func normalizeFormat(format string) string {
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		return "jpeg"
	case "png":
		return "png"
	default:
		return "png"
	}
}
