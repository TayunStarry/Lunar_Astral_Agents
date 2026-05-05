package screenshot

import (
	"bytes"
	"config"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/kbinani/screenshot"
)

// 截图互斥锁
var screenshotMutex sync.RWMutex

// Screenshot 执行截图操作
func Screenshot(req ScreenshotRequest) ([]byte, string, string, error) {
	// 检查频率限制
	if err := checkScreenshotRateLimit(); err != nil {
		return nil, "", "", err
	}

	// 使用默认值
	if req.Format == "" {
		req.Format = *config.Format
	}
	if req.Quality == 0 {
		req.Quality = *config.JPEGQuality
	}

	var img *image.RGBA
	var err error

	// 获取显示器数量
	displayCount := screenshot.NumActiveDisplays()

	// 根据参数决定截图方式
	if req.Region != "" {
		// 区域截图
		rect, err2 := parseRegion(req.Region)
		if err2 != nil {
			return nil, "", "", err2
		}
		img, err = screenshot.CaptureRect(rect)
	} else if req.DisplayIndex >= 0 && req.DisplayIndex < displayCount {
		// 指定显示器
		img, err = screenshot.CaptureDisplay(req.DisplayIndex)
	} else if req.DisplayIndex == -1 && displayCount > 1 {
		// 所有显示器拼接
		img, err = screenshotAllDisplaysOptimized()
	} else {
		// 默认第一个显示器
		img, err = screenshot.CaptureDisplay(0)
	}

	if err != nil {
		return nil, "", "", fmt.Errorf("截图失败: %v", err)
	}

	// 缩放处理
	img, err = applyScale(img, req.Scale)
	if err != nil {
		return nil, "", "", fmt.Errorf("缩放失败: %v", err)
	}

	// 编码图片
	buf := &bytes.Buffer{}
	var filename string

	if req.DisplayIndex >= 0 {
		filename = fmt.Sprintf("screenshot_d%d.%s", req.DisplayIndex, req.Format)
	} else {
		filename = fmt.Sprintf("screenshot.%s", req.Format)
	}

	if err := encodeImage(buf, img, req.Format, req.Quality); err != nil {
		return nil, "", "", err
	}

	// 更新最后截图时间
	screenshotMutex.Lock()
	lastCapture = time.Now()
	screenshotMutex.Unlock()

	contentType := getContentType(req.Format)
	return buf.Bytes(), filename, contentType, nil
}

// GetDisplays 获取所有显示器信息
func GetDisplays() []map[string]int {
	n := screenshot.NumActiveDisplays()
	displays := make([]map[string]int, n)

	for i := range n {
		bounds := screenshot.GetDisplayBounds(i)
		displays[i] = map[string]int{
			"index":  i,
			"x":      bounds.Min.X,
			"y":      bounds.Min.Y,
			"width":  bounds.Dx(),
			"height": bounds.Dy(),
		}
	}

	return displays
}

// ResizeImage 缩放图片
func ResizeImage(imgData []byte) (map[string]any, error) {
	// 解码图片
	img, format, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("解码图片失败: %v", err)
	}

	// 转换为RGBA格式
	rgbaImg := ToRGBA(img)

	// 缩放图片
	resizedImg := resizeImageTo1080(rgbaImg)

	// 编码缩放后的图片
	buf := &bytes.Buffer{}
	var contentType string
	switch format {
	case "jpeg":
		contentType = "image/jpeg"
		jpeg.Encode(buf, resizedImg, &jpeg.Options{Quality: 90})
	case "png":
		contentType = "image/png"
		png.Encode(buf, resizedImg)
	default:
		contentType = "image/jpeg"
		jpeg.Encode(buf, resizedImg, &jpeg.Options{Quality: 90})
	}

	// 生成base64编码
	base64Data := base64.StdEncoding.EncodeToString(buf.Bytes())
	base64WithHeader := fmt.Sprintf("data:%s;base64,%s", contentType, base64Data)

	// 构造响应
	response := map[string]any{
		"image":  buf.Bytes(),
		"base64": base64WithHeader,
		"format": format,
		"width":  resizedImg.Bounds().Dx(),
		"height": resizedImg.Bounds().Dy(),
	}

	return response, nil
}

// 解析区域字符串
func parseRegion(regionStr string) (image.Rectangle, error) {
	parts := strings.Split(regionStr, ",")
	if len(parts) != 4 {
		return image.Rectangle{}, fmt.Errorf("区域格式应为 'x,y,width,height'")
	}

	x, _ := strconv.Atoi(parts[0])
	y, _ := strconv.Atoi(parts[1])
	width, _ := strconv.Atoi(parts[2])
	height, _ := strconv.Atoi(parts[3])

	if width <= 0 || height <= 0 {
		return image.Rectangle{}, fmt.Errorf("宽高必须大于0")
	}

	return image.Rect(x, y, x+width, y+height), nil
}

// 截取所有显示器并拼接（优化版）
func screenshotAllDisplaysOptimized() (*image.RGBA, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return nil, fmt.Errorf("未找到显示器")
	}

	// 获取所有显示器的边界
	displays := make([]image.Rectangle, n)
	for i := range n {
		displays[i] = screenshot.GetDisplayBounds(i)
	}

	// 计算总边界
	minX, minY := displays[0].Min.X, displays[0].Min.Y
	maxX, maxY := displays[0].Max.X, displays[0].Max.Y

	for i := 1; i < n; i++ {
		bounds := displays[i]
		if bounds.Min.X < minX {
			minX = bounds.Min.X
		}
		if bounds.Min.Y < minY {
			minY = bounds.Min.Y
		}
		if bounds.Max.X > maxX {
			maxX = bounds.Max.X
		}
		if bounds.Max.Y > maxY {
			maxY = bounds.Max.Y
		}
	}

	totalWidth := maxX - minX
	totalHeight := maxY - minY

	// 创建大图
	img := image.NewRGBA(image.Rect(0, 0, totalWidth, totalHeight))

	// 使用 draw.Draw 合并每个显示器的截图
	for i, bounds := range displays {
		displayImg, err := screenshot.CaptureDisplay(i)
		if err != nil {
			// 记录错误但继续处理其他显示器
			fmt.Printf("截取显示器 %d 失败: %v\n", i, err)
			continue
		}

		// 计算在总图中的位置
		dx := bounds.Min.X - minX
		dy := bounds.Min.Y - minY

		// 使用 draw.Draw 复制图像，提高性能
		draw.Draw(
			img,
			image.Rect(dx, dy, dx+bounds.Dx(), dy+bounds.Dy()),
			displayImg,
			image.Point{0, 0},
			draw.Src,
		)
	}

	return img, nil
}

// 将 NRGBA 转换为 RGBA
func ToRGBA(img image.Image) *image.RGBA {
	// 如果已经是 RGBA，直接返回
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	// 创建新的 RGBA 图像
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)

	// 使用 draw.Draw 复制像素，提高性能
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	return rgba
}

// 缩放图片
func resizeImage(img *image.RGBA, scaleStr string) (*image.RGBA, error) {
	// 如果包含逗号，表示指定宽高
	if strings.Contains(scaleStr, ",") {
		parts := strings.Split(scaleStr, ",")
		if len(parts) != 2 {
			return nil, fmt.Errorf("缩放格式应为 'width,height' 或 '0.5'")
		}
		width, _ := strconv.Atoi(parts[0])
		height, _ := strconv.Atoi(parts[1])

		if width <= 0 || height <= 0 {
			return nil, fmt.Errorf("缩放宽高必须大于0")
		}

		resized := imaging.Resize(img, width, height, imaging.Lanczos)
		return ToRGBA(resized), nil
	}

	// 否则是比例
	scale, err := strconv.ParseFloat(scaleStr, 64)
	if err != nil {
		return nil, fmt.Errorf("无效的缩放比例: %v", err)
	}

	if scale <= 0 {
		return nil, fmt.Errorf("缩放比例必须大于0")
	}

	newWidth := int(float64(img.Bounds().Dx()) * scale)
	newHeight := int(float64(img.Bounds().Dy()) * scale)

	// 限制最小尺寸
	if newWidth < 1 || newHeight < 1 {
		newWidth = 1
		newHeight = 1
	}

	resized := imaging.Resize(img, newWidth, newHeight, imaging.Lanczos)
	return ToRGBA(resized), nil
}

// 缩放到合适大小
func ResizeToFit(img *image.RGBA, maxWidth, maxHeight int) *image.RGBA {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	if width <= maxWidth && height <= maxHeight {
		return img
	}

	ratio := float64(width) / float64(height)

	if width > maxWidth {
		width = maxWidth
		height = int(float64(width) / ratio)
	}

	if height > maxHeight {
		height = maxHeight
		width = int(float64(height) * ratio)
	}

	resized := imaging.Resize(img, width, height, imaging.Lanczos)
	return ToRGBA(resized)
}

// 应用缩放处理
func applyScale(img *image.RGBA, scaleStr string) (*image.RGBA, error) {
	if scaleStr != "" {
		return resizeImage(img, scaleStr)
	}

	// 使用配置的最大尺寸限制
	return ResizeToFit(img, *config.MaxWidth, *config.MaxHeight), nil
}

// 编码图片
func encodeImage(buf *bytes.Buffer, img *image.RGBA, format string, quality int) error {
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		if quality < 1 || quality > 100 {
			quality = *config.JPEGQuality // 默认质量
		}
		return jpeg.Encode(buf, img, &jpeg.Options{Quality: quality})
	case "png":
		return png.Encode(buf, img)
	default:
		return fmt.Errorf("不支持的图片格式: %s", format)
	}
}

// 获取内容类型
func getContentType(format string) string {
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	default:
		return "image/png"
	}
}

// 检查截图频率限制
func checkScreenshotRateLimit() error {
	screenshotMutex.RLock()
	timeSinceLastCapture := time.Since(lastCapture)
	screenshotMutex.RUnlock()

	if timeSinceLastCapture < captureCooldown {
		return fmt.Errorf("截图过于频繁，请等待 %.1f 秒", float64(captureCooldown-timeSinceLastCapture)/float64(time.Second))
	}
	return nil
}

// 缩放图片到最大尺寸1080
func resizeImageTo1080(img *image.RGBA) *image.RGBA {
	return ResizeToFit(img, 1080, 1080)
}
