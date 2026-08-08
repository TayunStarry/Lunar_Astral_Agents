package module

import (
	"bytes"
	"config"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"logger"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/kbinani/screenshot"
)

func init() {
	logger.SetDevMode(*config.Developer, "local_data/documents/debug")
}

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
	ScreenshotMutex.Lock()
	LastCapture = time.Now().UnixNano()
	ScreenshotMutex.Unlock()

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

// processGIF 处理GIF图片：抽取帧、缩放到512x512、纵向拼接至多5帧为长图
// 若拼接不可行则回退到首帧编码输出
func processGIF(imgData []byte) (map[string]any, error) {
	// 1. 解码GIF
	gifImg, err := gif.DecodeAll(bytes.NewReader(imgData))
	if err != nil {
		logger.Error("Screenshot", "processGIF: GIF解码失败: %v", err)
		return nil, fmt.Errorf("GIF解码失败: %v", err)
	}

	frameCount := len(gifImg.Image)
	if frameCount == 0 {
		logger.Error("Screenshot", "processGIF: GIF无帧数据")
		return nil, fmt.Errorf("GIF无帧数据")
	}

	logger.Info("Screenshot", "processGIF: GIF帧数=%d", frameCount)

	// 2. 选取至多5帧，尽量均分
	selectedIndices := selectFrameIndices(frameCount, 5)
	logger.Info("Screenshot", "processGIF: 选取帧索引=%v", selectedIndices)

	// 3. 逐帧转换为RGBA并缩放到512x512
	frames := make([]*image.RGBA, 0, len(selectedIndices))
	for _, idx := range selectedIndices {
		palettedFrame := gifImg.Image[idx]
		// 将Paletted图像绘制到RGBA（考虑GIF的位移偏移）
		bounds := palettedFrame.Bounds()
		rgba := image.NewRGBA(bounds)
		draw.Draw(rgba, bounds, palettedFrame, bounds.Min, draw.Over)

		// 缩放到512x512
		resized := ResizeToFit(rgba, 512, 512)
		frames = append(frames, resized)
	}

	// 4. 尝试纵向拼接
	concatenated, err := verticallyConcatFrames(frames)
	if err != nil {
		// 拼接不可行，回退到首帧
		logger.Info("Screenshot", "processGIF: 纵向拼接失败(%v)，回退到首帧", err)
		return encodeSingleFrame(frames[0], "gif")
	}

	logger.Info("Screenshot", "processGIF: 纵向拼接完成 尺寸=%dx%d",
		concatenated.Bounds().Dx(), concatenated.Bounds().Dy())

	return encodeSingleFrame(concatenated, "gif")
}

// selectFrameIndices 从total帧中选取最多maxCount帧，尽量在长度方向均分
func selectFrameIndices(total, maxCount int) []int {
	if total <= maxCount {
		// 帧数不足maxCount，全部选取
		indices := make([]int, total)
		for i := range total {
			indices[i] = i
		}
		return indices
	}

	// 均分选取：在total帧中均匀分布maxCount个采样点
	indices := make([]int, 0, maxCount)
	for i := range maxCount {
		// 使用浮点均分避免首尾偏移
		idx := int(math.Round(float64(i) * float64(total-1) / float64(maxCount-1)))
		indices = append(indices, idx)
	}
	return indices
}

// verticallyConcatFrames 将多帧图片纵向拼接为一张长图
func verticallyConcatFrames(frames []*image.RGBA) (*image.RGBA, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("无帧数据可拼接")
	}

	// 统一宽度为第一帧宽度
	targetWidth := frames[0].Bounds().Dx()
	totalHeight := 0
	for _, f := range frames {
		totalHeight += f.Bounds().Dy()
	}

	if totalHeight > 16384 {
		return nil, fmt.Errorf("拼接后高度%d超过16384px限制", totalHeight)
	}

	// 创建拼接画布
	result := image.NewRGBA(image.Rect(0, 0, targetWidth, totalHeight))

	currentY := 0
	for _, f := range frames {
		frameBounds := f.Bounds()
		// 居中绘制（帧宽度可能不同）
		offsetX := (targetWidth - frameBounds.Dx()) / 2
		draw.Draw(result,
			image.Rect(offsetX, currentY, offsetX+frameBounds.Dx(), currentY+frameBounds.Dy()),
			f,
			image.Point{0, 0},
			draw.Over,
		)
		currentY += frameBounds.Dy()
	}

	return result, nil
}

// encodeSingleFrame 将单帧RGBA图像编码输出为PNG格式
func encodeSingleFrame(img *image.RGBA, _ string) (map[string]any, error) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	buf := &bytes.Buffer{}
	// GIF帧可能包含透明度，统一输出PNG
	outputFormat := "png"
	contentType := "image/png"

	if err := png.Encode(buf, img); err != nil {
		logger.Error("Screenshot", "encodeSingleFrame: PNG编码失败: %v", err)
		return nil, fmt.Errorf("PNG编码失败: %v", err)
	}

	base64Data := base64.StdEncoding.EncodeToString(buf.Bytes())
	base64WithHeader := fmt.Sprintf("data:%s;base64,%s", contentType, base64Data)

	response := map[string]any{
		"image":  buf.Bytes(),
		"base64": base64WithHeader,
		"format": outputFormat,
		"width":  width,
		"height": height,
	}

	logger.Info("Screenshot", "encodeSingleFrame: 编码完成 格式=%s 尺寸=%dx%d 输出大小=%d bytes",
		outputFormat, width, height, len(buf.Bytes()))
	return response, nil
}

// ResizeImage 图片预处理：格式验证、非JPG/PNG转码、等比例缩放到1024、编码输出
// 仅输出JPG或PNG格式的base64数据
// GIF格式特殊处理：帧抽取→缩放→纵向拼接→编码输出
func ResizeImage(imgData []byte) (map[string]any, error) {
	// 1. 输入验证
	if len(imgData) == 0 {
		logger.Error("Screenshot", "ResizeImage: 图片数据为空")
		return nil, fmt.Errorf("图片数据为空")
	}
	const maxImageSize = 50 * 1024 * 1024 // 50MB
	if len(imgData) > maxImageSize {
		logger.Error("Screenshot", "ResizeImage: 图片数据过大: %d bytes", len(imgData))
		return nil, fmt.Errorf("图片数据过大，超过50MB限制")
	}

	logger.Info("Screenshot", "ResizeImage: 开始处理，原始大小=%d bytes", len(imgData))

	// 2. 文件头格式检测
	originalFormat := detectImageFormat(imgData)
	logger.Info("Screenshot", "ResizeImage: 检测到原始格式=%s", originalFormat)

	var processedData []byte

	// 3. 格式处理：GIF特殊处理，JPG/PNG直接使用，否则FFmpeg转码
	switch originalFormat {
	case "gif":
		// GIF特殊处理：帧抽取→缩放→纵向拼接→编码输出
		logger.Info("Screenshot", "ResizeImage: 检测到GIF格式，启动GIF帧处理流程")
		result, err := processGIF(imgData)
		if err != nil {
			logger.Error("Screenshot", "ResizeImage: GIF处理失败: %v", err)
			return nil, fmt.Errorf("GIF处理失败: %v", err)
		}
		return result, nil
	case "jpeg", "png":
		processedData = imgData
	default:
		logger.Info("Screenshot", "ResizeImage: 非JPG/PNG格式(%s)，启动FFmpeg转码", originalFormat)
		converted, err := convertImageWithFFmpeg(imgData)
		if err != nil {
			logger.Error("Screenshot", "ResizeImage: FFmpeg转码失败: %v", err)
			return nil, fmt.Errorf("FFmpeg转码失败（原始格式=%s）: %v", originalFormat, err)
		}
		processedData = converted
		logger.Info("Screenshot", "ResizeImage: FFmpeg转码完成，转换后大小=%d bytes", len(processedData))
	}

	// 4. 解码图片
	img, format, err := image.Decode(bytes.NewReader(processedData))
	if err != nil {
		logger.Error("Screenshot", "ResizeImage: 解码失败: %v", err)
		return nil, fmt.Errorf("解码图片失败: %v", err)
	}

	// 5. 尺寸验证
	bounds := img.Bounds()
	oriWidth, oriHeight := bounds.Dx(), bounds.Dy()
	if oriWidth <= 0 || oriHeight <= 0 {
		return nil, fmt.Errorf("图片尺寸无效: %dx%d", oriWidth, oriHeight)
	}
	if oriWidth > 16384 || oriHeight > 16384 {
		logger.Error("Screenshot", "ResizeImage: 图片尺寸异常: %dx%d", oriWidth, oriHeight)
		return nil, fmt.Errorf("图片尺寸异常（%dx%d），超过16384px限制", oriWidth, oriHeight)
	}

	// 6. 转换为RGBA并等比例缩放到1024
	rgbaImg := ToRGBA(img)
	resizedImg := resizeToMax1024(rgbaImg)

	newWidth := resizedImg.Bounds().Dx()
	newHeight := resizedImg.Bounds().Dy()
	logger.Info("Screenshot", "ResizeImage: 缩放完成 %dx%d -> %dx%d", oriWidth, oriHeight, newWidth, newHeight)

	// 7. 编码输出（严格限制仅PNG/JPG）
	buf := &bytes.Buffer{}
	var contentType, outputFormat string

	switch format {
	case "jpeg":
		outputFormat = "jpeg"
		contentType = "image/jpeg"
		if err := jpeg.Encode(buf, resizedImg, &jpeg.Options{Quality: 90}); err != nil {
			logger.Error("Screenshot", "ResizeImage: JPEG编码失败: %v", err)
			return nil, fmt.Errorf("JPEG编码失败: %v", err)
		}
	case "png":
		outputFormat = "png"
		contentType = "image/png"
		if err := png.Encode(buf, resizedImg); err != nil {
			logger.Error("Screenshot", "ResizeImage: PNG编码失败: %v", err)
			return nil, fmt.Errorf("PNG编码失败: %v", err)
		}
	default:
		// 兜底：非预期格式统一输出为JPEG
		logger.Info("Screenshot", "ResizeImage: 非标准解码格式(%s)，兜底输出JPEG", format)
		outputFormat = "jpeg"
		contentType = "image/jpeg"
		if err := jpeg.Encode(buf, resizedImg, &jpeg.Options{Quality: 90}); err != nil {
			logger.Error("Screenshot", "ResizeImage: JPEG兜底编码失败: %v", err)
			return nil, fmt.Errorf("JPEG编码失败: %v", err)
		}
	}

	// 8. 生成base64
	base64Data := base64.StdEncoding.EncodeToString(buf.Bytes())
	base64WithHeader := fmt.Sprintf("data:%s;base64,%s", contentType, base64Data)

	// 9. 构造响应
	response := map[string]any{
		"image":  buf.Bytes(),
		"base64": base64WithHeader,
		"format": outputFormat,
		"width":  newWidth,
		"height": newHeight,
	}

	logger.Info("Screenshot", "ResizeImage: 处理完成 格式=%s 尺寸=%dx%d 输出大小=%d bytes",
		outputFormat, newWidth, newHeight, len(buf.Bytes()))
	return response, nil
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
	if *config.FfmpegPath != "" {
		ffmpegPath = *config.FfmpegPath
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
			logger.Error("Screenshot", "截取显示器 %d 失败: %v", i, err)
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
	ScreenshotMutex.RLock()
	elapsed := time.Now().UnixNano() - LastCapture
	ScreenshotMutex.RUnlock()

	if elapsed < CaptureCooldown {
		remaining := float64(CaptureCooldown-elapsed) / float64(time.Second)
		return fmt.Errorf("截图过于频繁，请等待 %.1f 秒", remaining)
	}
	return nil
}
