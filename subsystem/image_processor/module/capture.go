package module

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
//   - auto：焦点窗口优先；识别失败降级为多屏拼接全屏
//   - window：强制焦点窗口，识别失败直接报错
//   - fullscreen：多屏拼接全屏
//   - display：指定显示器
//   - region：绝对屏幕坐标区域
//
// 坐标覆盖：mode 为 auto/window 且 Width>0 && Height>0 时，
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
		img         *image.RGBA
		usedMode    CaptureMode
		windowTitle string
		displayIdx  int
		err         error
	)

	switch req.Mode {
	case ModeAuto, ModeWindow:
		img, windowTitle, err = captureFocusedWindow(req)
		if err != nil {
			if req.Mode == ModeWindow {
				return CaptureResult{}, err
			}
			// auto 降级为多屏拼接全屏
			img, err = screenshotAllDisplaysOptimized()
			if err != nil {
				return CaptureResult{}, fmt.Errorf("焦点窗口截图失败，全屏降级也失败: %v", err)
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
	}, nil
}

// captureFocusedWindow 捕获焦点窗口，含窗口相对精准区域覆盖
func captureFocusedWindow(req CaptureRequest) (*image.RGBA, string, error) {
	if req.Width > 0 && req.Height > 0 {
		// 坐标覆盖：窗口相对精准区域
		return captureForegroundWindowRegion(req.OffsetX, req.OffsetY, req.Width, req.Height)
	}
	return captureForegroundWindow()
}

// captureDisplay 捕获指定显示器
func captureDisplay(index int) (*image.RGBA, error) {
	n := screenshot.NumActiveDisplays()
	if index < 0 || index >= n {
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
