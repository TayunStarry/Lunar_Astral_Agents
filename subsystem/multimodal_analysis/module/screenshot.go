package module

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/kbinani/screenshot"
)

func init() {
	LoggerGeneral.SetDevMode(*GeneralConfig.Developer)
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

// firstFrameOfGIF 解码 GIF 首帧为 RGBA
// 静态图片链路只取首帧；动态图的时序理解由 AnimatedImageToMedia 转视频后交给观影者角色
func firstFrameOfGIF(imgData []byte) (*image.RGBA, error) {
	gifImg, err := gif.DecodeAll(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("GIF解码失败: %v", err)
	}
	if len(gifImg.Image) == 0 {
		return nil, fmt.Errorf("GIF无帧数据")
	}
	frame := gifImg.Image[0]
	bounds := frame.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, frame, bounds.Min, draw.Over)
	return rgba, nil
}

// ResizeImage 图片预处理：格式验证、非JPG/PNG转码、等比例缩放到1024、编码输出
// 仅输出JPG或PNG格式的base64数据
// 动态图（多帧 GIF / APNG / 动态 WebP）在此仅按首帧静态处理：
// 时序与动作理解由 AnimatedImageToMedia 转视频后走观影者链路，不再逐帧拆解
// 返回单元素数组
func ResizeImage(imgData []byte) ([]map[string]any, error) {
	// 1. 输入验证
	if len(imgData) == 0 {
		LoggerGeneral.Error("MultimodalAnalysis", "ResizeImage: 图片数据为空")
		return nil, fmt.Errorf("图片数据为空")
	}
	const maxImageSize = 50 * 1024 * 1024 // 50MB
	if len(imgData) > maxImageSize {
		LoggerGeneral.Error("MultimodalAnalysis", "ResizeImage: 图片数据过大: %d bytes", len(imgData))
		return nil, fmt.Errorf("图片数据过大，超过50MB限制")
	}

	LoggerGeneral.Info("MultimodalAnalysis", "ResizeImage: 开始处理，原始大小=%d bytes", len(imgData))

	// 2. 文件头格式检测
	originalFormat := detectImageFormat(imgData)
	LoggerGeneral.Info("MultimodalAnalysis", "ResizeImage: 检测到原始格式=%s", originalFormat)

	var processedData []byte

	// 3. 格式处理：GIF 统一取首帧转 PNG，JPG/PNG 直接使用，其余交给 FFmpeg 转码
	switch originalFormat {
	case "gif":
		// GIF（含动态图）：静态链路只取首帧；动态图理解见 AnimatedImageToMedia 的视频链路
		rgba, err := firstFrameOfGIF(imgData)
		if err != nil {
			LoggerGeneral.Error("MultimodalAnalysis", "ResizeImage: GIF首帧解码失败: %v", err)
			return nil, err
		}
		buf := &bytes.Buffer{}
		if err := png.Encode(buf, rgba); err != nil {
			LoggerGeneral.Error("MultimodalAnalysis", "ResizeImage: GIF首帧编码失败: %v", err)
			return nil, fmt.Errorf("GIF首帧编码失败: %v", err)
		}
		processedData = buf.Bytes()
	case "png":
		// APNG 动态图同样只取默认帧（Go 的 PNG 解码器行为），无需特殊处理
		processedData = imgData
	case "jpeg":
		processedData = imgData
	case "webp":
		// 静态与动态 WebP 均由 FFmpeg 转码：动态 WebP 经 image2pipe 输出多帧 PNG 流，
		// 后续解码仅取第一帧，等价于首帧静态降级
		LoggerGeneral.Info("MultimodalAnalysis", "ResizeImage: WebP，启动FFmpeg转码")
		converted, err := convertImageWithFFmpeg(imgData)
		if err != nil {
			LoggerGeneral.Error("MultimodalAnalysis", "ResizeImage: WebP转码失败: %v", err)
			return nil, fmt.Errorf("WebP转码失败: %v", err)
		}
		processedData = converted
		LoggerGeneral.Info("MultimodalAnalysis", "ResizeImage: WebP转码完成，转换后大小=%d bytes", len(processedData))
	default:
		LoggerGeneral.Info("MultimodalAnalysis", "ResizeImage: 非JPG/PNG格式(%s)，启动FFmpeg转码", originalFormat)
		converted, err := convertImageWithFFmpeg(imgData)
		if err != nil {
			LoggerGeneral.Error("MultimodalAnalysis", "ResizeImage: FFmpeg转码失败: %v", err)
			return nil, fmt.Errorf("FFmpeg转码失败（原始格式=%s）: %v", originalFormat, err)
		}
		processedData = converted
		LoggerGeneral.Info("MultimodalAnalysis", "ResizeImage: FFmpeg转码完成，转换后大小=%d bytes", len(processedData))
	}

	// 4. 解码图片
	img, format, err := image.Decode(bytes.NewReader(processedData))
	if err != nil {
		LoggerGeneral.Error("MultimodalAnalysis", "ResizeImage: 解码失败: %v", err)
		return nil, fmt.Errorf("解码图片失败: %v", err)
	}

	// 5. 尺寸验证
	bounds := img.Bounds()
	oriWidth, oriHeight := bounds.Dx(), bounds.Dy()
	if oriWidth <= 0 || oriHeight <= 0 {
		return nil, fmt.Errorf("图片尺寸无效: %dx%d", oriWidth, oriHeight)
	}
	if oriWidth > 16384 || oriHeight > 16384 {
		LoggerGeneral.Error("MultimodalAnalysis", "ResizeImage: 图片尺寸异常: %dx%d", oriWidth, oriHeight)
		return nil, fmt.Errorf("图片尺寸异常（%dx%d），超过16384px限制", oriWidth, oriHeight)
	}

	// 6. 转换为RGBA并等比例缩放到1024
	rgbaImg := ToRGBA(img)
	resizedImg := resizeToMax1024(rgbaImg)

	newWidth := resizedImg.Bounds().Dx()
	newHeight := resizedImg.Bounds().Dy()
	LoggerGeneral.Info("MultimodalAnalysis", "ResizeImage: 缩放完成 %dx%d -> %dx%d", oriWidth, oriHeight, newWidth, newHeight)

	// 7. 编码输出（严格限制仅PNG/JPG）
	buf := &bytes.Buffer{}
	var contentType, outputFormat string

	switch format {
	case "jpeg":
		outputFormat = "jpeg"
		contentType = "image/jpeg"
		if err := jpeg.Encode(buf, resizedImg, &jpeg.Options{Quality: 90}); err != nil {
			LoggerGeneral.Error("MultimodalAnalysis", "ResizeImage: JPEG编码失败: %v", err)
			return nil, fmt.Errorf("JPEG编码失败: %v", err)
		}
	case "png":
		outputFormat = "png"
		contentType = "image/png"
		if err := png.Encode(buf, resizedImg); err != nil {
			LoggerGeneral.Error("MultimodalAnalysis", "ResizeImage: PNG编码失败: %v", err)
			return nil, fmt.Errorf("PNG编码失败: %v", err)
		}
	default:
		// 兜底：非预期格式统一输出为JPEG
		LoggerGeneral.Info("MultimodalAnalysis", "ResizeImage: 非标准解码格式(%s)，兜底输出JPEG", format)
		outputFormat = "jpeg"
		contentType = "image/jpeg"
		if err := jpeg.Encode(buf, resizedImg, &jpeg.Options{Quality: 90}); err != nil {
			LoggerGeneral.Error("MultimodalAnalysis", "ResizeImage: JPEG兜底编码失败: %v", err)
			return nil, fmt.Errorf("JPEG编码失败: %v", err)
		}
	}

	// 8. 生成base64
	base64Data := base64.StdEncoding.EncodeToString(buf.Bytes())
	base64WithHeader := fmt.Sprintf("data:%s;base64,%s", contentType, base64Data)

	// 9. 构造响应（单帧数组）
	response := map[string]any{
		"image":  buf.Bytes(),
		"base64": base64WithHeader,
		"format": outputFormat,
		"width":  newWidth,
		"height": newHeight,
	}

	LoggerGeneral.Info("MultimodalAnalysis", "ResizeImage: 处理完成 格式=%s 尺寸=%dx%d 输出大小=%d bytes",
		outputFormat, newWidth, newHeight, len(buf.Bytes()))
	return []map[string]any{response}, nil
}

// StandardizeImageForPerception 静态图片标准化（感知者式入库预处理）：
// 解码（GIF/APNG 取首帧，WebP 等非 JPG/PNG 经 FFmpeg 转码）→ 宽或高超过 maxDim
// 时等比缩放（Lanczos）→ 统一编码 JPEG（Q90）/ PNG（PNG 源保留透明）→ 返回完整
// data URI。供记忆库图片入库前调用，降低存储体积与 vision token 消耗。
// 动态图的时序理解由 AnimatedImageToMedia 视频链路负责，此处仅取首帧静态降级。
func StandardizeImageForPerception(imgData []byte, maxDim int) (string, error) {
	if len(imgData) == 0 {
		return "", fmt.Errorf("图片数据为空")
	}
	if maxDim <= 0 {
		maxDim = 640
	}
	originalFormat := detectImageFormat(imgData)

	var processedData []byte
	switch originalFormat {
	case "gif":
		rgba, err := firstFrameOfGIF(imgData)
		if err != nil {
			return "", err
		}
		buf := &bytes.Buffer{}
		if err := png.Encode(buf, rgba); err != nil {
			return "", fmt.Errorf("GIF首帧编码失败: %v", err)
		}
		processedData = buf.Bytes()
	case "jpeg", "png":
		processedData = imgData
	default:
		converted, err := convertImageWithFFmpeg(imgData)
		if err != nil {
			return "", fmt.Errorf("转码失败（原始格式=%s）: %v", originalFormat, err)
		}
		processedData = converted
	}

	img, format, err := image.Decode(bytes.NewReader(processedData))
	if err != nil {
		return "", fmt.Errorf("解码图片失败: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return "", fmt.Errorf("图片尺寸无效: %dx%d", bounds.Dx(), bounds.Dy())
	}

	rgbaImg := ToRGBA(img)
	resizedImg := ResizeToFit(rgbaImg, maxDim, maxDim)

	buf := &bytes.Buffer{}
	var contentType string
	if format == "png" {
		contentType = "image/png"
		if err := png.Encode(buf, resizedImg); err != nil {
			return "", fmt.Errorf("PNG编码失败: %v", err)
		}
	} else {
		contentType = "image/jpeg"
		if err := jpeg.Encode(buf, resizedImg, &jpeg.Options{Quality: 90}); err != nil {
			return "", fmt.Errorf("JPEG编码失败: %v", err)
		}
	}
	return fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(buf.Bytes())), nil
}

// detectImageFormat 通过文件头魔数检测图片格式
func detectImageFormat(data []byte) string {
	if len(data) < 12 {
		return "unknown"
	}

	// JPEG: FF D8 FF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "jpeg"
	}

	// PNG: 89 50 4E 47 0D 0A 1A 0A
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
		data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A {
		return "png"
	}

	// GIF: 47 49 46 38 (GIF8)
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 {
		return "gif"
	}

	// BMP: 42 4D
	if data[0] == 0x42 && data[1] == 0x4D {
		return "bmp"
	}

	// WebP: 52 49 46 46 ... 57 45 42 50 (RIFF....WEBP)
	if len(data) >= 12 &&
		data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "webp"
	}

	// TIFF (little-endian): 49 49 2A 00
	if data[0] == 0x49 && data[1] == 0x49 && data[2] == 0x2A && data[3] == 0x00 {
		return "tiff"
	}

	// TIFF (big-endian): 4D 4D 00 2A
	if data[0] == 0x4D && data[1] == 0x4D && data[2] == 0x00 && data[3] == 0x2A {
		return "tiff"
	}

	return "unknown"
}

// convertImageWithFFmpeg 使用FFmpeg将非JPG/PNG图片转换为PNG
// 通过管道输入/输出，避免临时文件，并设置30秒超时
func convertImageWithFFmpeg(input []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ffmpegPath := "ffmpeg"
	if *GeneralConfig.FfmpegPath != "" {
		ffmpegPath = *GeneralConfig.FfmpegPath
	}

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-i", "pipe:0",
		"-f", "image2",
		"-vcodec", "png",
		"-pix_fmt", "rgba",
		"pipe:1",
	)

	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = buf
	cmd.Stderr = errBuf

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("FFmpeg转码超时（30秒）")
		}
		return nil, fmt.Errorf("FFmpeg转码失败: %v, stderr: %s", err, errBuf.String())
	}

	if buf.Len() == 0 {
		return nil, fmt.Errorf("FFmpeg转码输出为空")
	}

	return buf.Bytes(), nil
}

// resizeToMax1024 等比例缩放图片，长宽均不超过1024像素
func resizeToMax1024(img *image.RGBA) *image.RGBA {
	return ResizeToFit(img, 1024, 1024)
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
			LoggerGeneral.Error("MultimodalAnalysis", "截取显示器 %d 失败: %v", i, err)
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

// ToRGBA 将 image.Image 转换为 RGBA
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

// ResizeToFit 缩放到合适大小
func ResizeToFit(img *image.RGBA, maxWidth, maxHeight int) *image.RGBA {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	if width <= maxWidth && height <= maxHeight {
		return img
	}

	// 使用单一比例因子，一次到位等比例缩放，避免二次修正导致的长宽比失真
	wRatio := float64(maxWidth) / float64(width)
	hRatio := float64(maxHeight) / float64(height)
	scale := wRatio
	if hRatio < wRatio {
		scale = hRatio
	}

	width = int(float64(width)*scale + 0.5)
	height = int(float64(height)*scale + 0.5)

	// 最终 clamp，防止浮点舍入导致超出限制
	if width > maxWidth {
		width = maxWidth
	}
	if height > maxHeight {
		height = maxHeight
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
	return ResizeToFit(img, *GeneralConfig.MaxWidth, *GeneralConfig.MaxHeight), nil
}

// 编码图片
func encodeImage(buf *bytes.Buffer, img *image.RGBA, format string, quality int) error {
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		if quality < 1 || quality > 100 {
			quality = *GeneralConfig.JPEGQuality // 默认质量
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
	ScreenshotMutex.RLock()
	elapsed := time.Now().UnixNano() - LastCapture
	ScreenshotMutex.RUnlock()

	if elapsed < CaptureCooldown {
		remaining := float64(CaptureCooldown-elapsed) / float64(time.Second)
		return fmt.Errorf("截图过于频繁，请等待 %.1f 秒", remaining)
	}
	return nil
}
