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
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/kbinani/screenshot"
)

func init() {
	LoggerGeneral.SetDevMode(*GeneralConfig.Developer, "local_data/documents/debug")
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

// processGIF 处理GIF图片：帧数>2时截取至多15帧独立返回；帧数≤2时按静态图处理
func processGIF(imgData []byte) ([]map[string]any, error) {
	// 1. 解码GIF
	gifImg, err := gif.DecodeAll(bytes.NewReader(imgData))
	if err != nil {
		LoggerGeneral.Error("ImageProcessor", "processGIF: GIF解码失败: %v", err)
		return nil, fmt.Errorf("GIF解码失败: %v", err)
	}

	frameCount := len(gifImg.Image)
	if frameCount == 0 {
		LoggerGeneral.Error("ImageProcessor", "processGIF: GIF无帧数据")
		return nil, fmt.Errorf("GIF无帧数据")
	}

	LoggerGeneral.Info("ImageProcessor", "processGIF: GIF帧数=%d", frameCount)

	// 2. 帧数≤2：按静态图处理，取首帧缩放输出
	if frameCount <= 2 {
		LoggerGeneral.Info("ImageProcessor", "processGIF: 帧数≤2，按静态图处理首帧")
		palettedFrame := gifImg.Image[0]
		bounds := palettedFrame.Bounds()
		rgba := image.NewRGBA(bounds)
		draw.Draw(rgba, bounds, palettedFrame, bounds.Min, draw.Over)
		resized := resizeToMax1024(rgba)
		result, err := encodeFrameToMap(resized, "png")
		if err != nil {
			return nil, err
		}
		return []map[string]any{result}, nil
	}

	// 3. 动态图（帧数>2）：选取至多15帧，均分采样
	selectedIndices := selectFrameIndices(frameCount, 15)
	LoggerGeneral.Info("ImageProcessor", "processGIF: 动态图%d帧→选取%d帧索引=%v", frameCount, len(selectedIndices), selectedIndices)

	// 4. 逐帧转换为RGBA、缩放到1024、编码输出
	results := make([]map[string]any, 0, len(selectedIndices))
	for _, idx := range selectedIndices {
		palettedFrame := gifImg.Image[idx]
		bounds := palettedFrame.Bounds()
		rgba := image.NewRGBA(bounds)
		draw.Draw(rgba, bounds, palettedFrame, bounds.Min, draw.Over)

		resized := resizeToMax1024(rgba)
		result, err := encodeFrameToMap(resized, "png")
		if err != nil {
			LoggerGeneral.Error("ImageProcessor", "processGIF: 第%d帧编码失败: %v", idx, err)
			return nil, fmt.Errorf("GIF第%d帧编码失败: %v", idx, err)
		}
		results = append(results, result)
	}

	LoggerGeneral.Info("ImageProcessor", "processGIF: 处理完成 输出%d帧", len(results))
	return results, nil
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

// encodeFrameToMap 将单帧RGBA图像编码为PNG并构造响应map
func encodeFrameToMap(img *image.RGBA, _ string) (map[string]any, error) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	buf := &bytes.Buffer{}
	outputFormat := "png"
	contentType := "image/png"

	if err := png.Encode(buf, img); err != nil {
		LoggerGeneral.Error("ImageProcessor", "encodeFrameToMap: PNG编码失败: %v", err)
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

	LoggerGeneral.Info("ImageProcessor", "encodeFrameToMap: 编码完成 格式=%s 尺寸=%dx%d 输出大小=%d bytes",
		outputFormat, width, height, len(buf.Bytes()))
	return response, nil
}

// ResizeImage 图片预处理：格式验证、非JPG/PNG转码、等比例缩放到1024、编码输出
// 仅输出JPG或PNG格式的base64数据
// 动态图(GIF/APNG/WebP)帧数>2时：截取至多15帧→逐帧缩放→返回base64数组
// 静态图或帧数≤2时：返回单元素数组
func ResizeImage(imgData []byte) ([]map[string]any, error) {
	// 1. 输入验证
	if len(imgData) == 0 {
		LoggerGeneral.Error("ImageProcessor", "ResizeImage: 图片数据为空")
		return nil, fmt.Errorf("图片数据为空")
	}
	const maxImageSize = 50 * 1024 * 1024 // 50MB
	if len(imgData) > maxImageSize {
		LoggerGeneral.Error("ImageProcessor", "ResizeImage: 图片数据过大: %d bytes", len(imgData))
		return nil, fmt.Errorf("图片数据过大，超过50MB限制")
	}

	LoggerGeneral.Info("ImageProcessor", "ResizeImage: 开始处理，原始大小=%d bytes", len(imgData))

	// 2. 文件头格式检测
	originalFormat := detectImageFormat(imgData)
	LoggerGeneral.Info("ImageProcessor", "ResizeImage: 检测到原始格式=%s", originalFormat)

	var processedData []byte

	// 3. 格式处理：动态图特殊处理，JPG/PNG直接使用，否则FFmpeg转码
	switch originalFormat {
	case "gif":
		// GIF动态图处理：帧数>2→截取至多15帧独立返回；帧数≤2→静态处理
		LoggerGeneral.Info("ImageProcessor", "ResizeImage: 检测到GIF格式，启动GIF帧处理流程")
		result, err := processGIF(imgData)
		if err != nil {
			LoggerGeneral.Error("ImageProcessor", "ResizeImage: GIF处理失败: %v", err)
			return nil, fmt.Errorf("GIF处理失败: %v", err)
		}
		return result, nil
	case "png":
		// 检测是否为APNG动态图
		if isAPNG(imgData) {
			LoggerGeneral.Info("ImageProcessor", "ResizeImage: 检测到APNG动态图，启动FFmpeg帧提取")
			result, err := processAnimatedWithFFmpeg(imgData)
			if err != nil {
				LoggerGeneral.Error("ImageProcessor", "ResizeImage: APNG处理失败: %v", err)
				return nil, fmt.Errorf("APNG处理失败: %v", err)
			}
			return result, nil
		}
		processedData = imgData
	case "jpeg":
		processedData = imgData
	case "webp":
		// 检测是否为动态WebP
		if isAnimatedWebP(imgData) {
			LoggerGeneral.Info("ImageProcessor", "ResizeImage: 检测到动态WebP，启动FFmpeg帧提取")
			result, err := processAnimatedWithFFmpeg(imgData)
			if err != nil {
				LoggerGeneral.Error("ImageProcessor", "ResizeImage: 动态WebP处理失败: %v", err)
				return nil, fmt.Errorf("动态WebP处理失败: %v", err)
			}
			return result, nil
		}
		// 静态WebP：FFmpeg转码为PNG
		LoggerGeneral.Info("ImageProcessor", "ResizeImage: 静态WebP，启动FFmpeg转码")
		converted, err := convertImageWithFFmpeg(imgData)
		if err != nil {
			LoggerGeneral.Error("ImageProcessor", "ResizeImage: WebP转码失败: %v", err)
			return nil, fmt.Errorf("WebP转码失败: %v", err)
		}
		processedData = converted
		LoggerGeneral.Info("ImageProcessor", "ResizeImage: WebP转码完成，转换后大小=%d bytes", len(processedData))
	default:
		LoggerGeneral.Info("ImageProcessor", "ResizeImage: 非JPG/PNG格式(%s)，启动FFmpeg转码", originalFormat)
		converted, err := convertImageWithFFmpeg(imgData)
		if err != nil {
			LoggerGeneral.Error("ImageProcessor", "ResizeImage: FFmpeg转码失败: %v", err)
			return nil, fmt.Errorf("FFmpeg转码失败（原始格式=%s）: %v", originalFormat, err)
		}
		processedData = converted
		LoggerGeneral.Info("ImageProcessor", "ResizeImage: FFmpeg转码完成，转换后大小=%d bytes", len(processedData))
	}

	// 4. 解码图片
	img, format, err := image.Decode(bytes.NewReader(processedData))
	if err != nil {
		LoggerGeneral.Error("ImageProcessor", "ResizeImage: 解码失败: %v", err)
		return nil, fmt.Errorf("解码图片失败: %v", err)
	}

	// 5. 尺寸验证
	bounds := img.Bounds()
	oriWidth, oriHeight := bounds.Dx(), bounds.Dy()
	if oriWidth <= 0 || oriHeight <= 0 {
		return nil, fmt.Errorf("图片尺寸无效: %dx%d", oriWidth, oriHeight)
	}
	if oriWidth > 16384 || oriHeight > 16384 {
		LoggerGeneral.Error("ImageProcessor", "ResizeImage: 图片尺寸异常: %dx%d", oriWidth, oriHeight)
		return nil, fmt.Errorf("图片尺寸异常（%dx%d），超过16384px限制", oriWidth, oriHeight)
	}

	// 6. 转换为RGBA并等比例缩放到1024
	rgbaImg := ToRGBA(img)
	resizedImg := resizeToMax1024(rgbaImg)

	newWidth := resizedImg.Bounds().Dx()
	newHeight := resizedImg.Bounds().Dy()
	LoggerGeneral.Info("ImageProcessor", "ResizeImage: 缩放完成 %dx%d -> %dx%d", oriWidth, oriHeight, newWidth, newHeight)

	// 7. 编码输出（严格限制仅PNG/JPG）
	buf := &bytes.Buffer{}
	var contentType, outputFormat string

	switch format {
	case "jpeg":
		outputFormat = "jpeg"
		contentType = "image/jpeg"
		if err := jpeg.Encode(buf, resizedImg, &jpeg.Options{Quality: 90}); err != nil {
			LoggerGeneral.Error("ImageProcessor", "ResizeImage: JPEG编码失败: %v", err)
			return nil, fmt.Errorf("JPEG编码失败: %v", err)
		}
	case "png":
		outputFormat = "png"
		contentType = "image/png"
		if err := png.Encode(buf, resizedImg); err != nil {
			LoggerGeneral.Error("ImageProcessor", "ResizeImage: PNG编码失败: %v", err)
			return nil, fmt.Errorf("PNG编码失败: %v", err)
		}
	default:
		// 兜底：非预期格式统一输出为JPEG
		LoggerGeneral.Info("ImageProcessor", "ResizeImage: 非标准解码格式(%s)，兜底输出JPEG", format)
		outputFormat = "jpeg"
		contentType = "image/jpeg"
		if err := jpeg.Encode(buf, resizedImg, &jpeg.Options{Quality: 90}); err != nil {
			LoggerGeneral.Error("ImageProcessor", "ResizeImage: JPEG兜底编码失败: %v", err)
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

	LoggerGeneral.Info("ImageProcessor", "ResizeImage: 处理完成 格式=%s 尺寸=%dx%d 输出大小=%d bytes",
		outputFormat, newWidth, newHeight, len(buf.Bytes()))
	return []map[string]any{response}, nil
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

// isAPNG 检测PNG数据是否为动态PNG（APNG）
// 通过扫描PNG chunk，检测是否存在acTL（Animation Control）chunk
func isAPNG(data []byte) bool {
	// PNG签名: 8字节
	if len(data) < 8+4+4+4 {
		return false
	}
	// 跳过PNG签名(8字节)，从第一个chunk开始扫描
	offset := 8
	// 限制扫描100个chunk，防止恶意数据
	for i := 0; i < 100 && offset+12 <= len(data); i++ {
		// chunk结构: 4字节长度 + 4字节类型 + 数据 + 4字节CRC
		chunkLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
		chunkType := string(data[offset+4 : offset+8])

		if chunkType == "acTL" {
			return true
		}
		// 跳转到下一个chunk: 4(length) + 4(type) + chunkLen(data) + 4(CRC)
		offset += 12 + chunkLen
	}
	return false
}

// isAnimatedWebP 检测WebP数据是否为动态WebP
// 通过扫描RIFF容器中的chunk，检测是否存在ANIM chunk
func isAnimatedWebP(data []byte) bool {
	// RIFF容器最小长度: RIFF(4) + size(4) + WEBP(4) + chunk(4+4) = 20
	if len(data) < 20 {
		return false
	}
	// 检查RIFF头
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return false
	}
	// 从第一个chunk开始扫描（偏移12字节跳过RIFF头+WEBP标识）
	offset := 12
	fileEnd := 8 + int(data[4]) | int(data[5])<<8 | int(data[6])<<16 | int(data[7])<<24

	// 限制扫描100个chunk
	for i := 0; i < 100 && offset+8 <= len(data) && offset+8 <= fileEnd; i++ {
		chunkType := string(data[offset : offset+4])
		chunkSize := int(data[offset+4]) | int(data[offset+5])<<8 | int(data[offset+6])<<16 | int(data[offset+7])<<24

		if chunkType == "ANIM" {
			return true
		}
		// 跳转到下一个chunk（注意chunk大小奇偶填充）
		offset += 8 + chunkSize
		if chunkSize%2 == 1 {
			offset++ // 奇数大小会有1字节填充
		}
	}
	return false
}

// processAnimatedWithFFmpeg 使用FFmpeg提取动态图帧，截取至多15帧，逐帧缩放后返回base64数组
// 支持APNG和动态WebP格式
func processAnimatedWithFFmpeg(imgData []byte) ([]map[string]any, error) {
	// 1. 使用FFmpeg提取所有帧
	frames, err := extractFramesWithFFmpeg(imgData)
	if err != nil {
		return nil, fmt.Errorf("FFmpeg帧提取失败: %v", err)
	}

	frameCount := len(frames)
	if frameCount == 0 {
		return nil, fmt.Errorf("动态图无帧数据")
	}

	LoggerGeneral.Info("ImageProcessor", "processAnimatedWithFFmpeg: 提取到%d帧", frameCount)

	// 2. 帧数≤2：按静态图处理
	if frameCount <= 2 {
		LoggerGeneral.Info("ImageProcessor", "processAnimatedWithFFmpeg: 帧数≤2，按静态图处理")
		rgba := ToRGBA(frames[0])
		resized := resizeToMax1024(rgba)
		result, err := encodeFrameToMap(resized, "png")
		if err != nil {
			return nil, err
		}
		return []map[string]any{result}, nil
	}

	// 3. 动态图：选取至多15帧
	selectedIndices := selectFrameIndices(frameCount, 15)
	LoggerGeneral.Info("ImageProcessor", "processAnimatedWithFFmpeg: %d帧→选取%d帧", frameCount, len(selectedIndices))

	// 4. 逐帧缩放并编码
	results := make([]map[string]any, 0, len(selectedIndices))
	for _, idx := range selectedIndices {
		rgba := ToRGBA(frames[idx])
		// 尺寸验证
		bounds := rgba.Bounds()
		if bounds.Dx() > 16384 || bounds.Dy() > 16384 {
			LoggerGeneral.Info("ImageProcessor", "processAnimatedWithFFmpeg: 第%d帧尺寸异常(%dx%d)，跳过", idx, bounds.Dx(), bounds.Dy())
			continue
		}
		resized := resizeToMax1024(rgba)
		result, err := encodeFrameToMap(resized, "png")
		if err != nil {
			LoggerGeneral.Error("ImageProcessor", "processAnimatedWithFFmpeg: 第%d帧编码失败: %v", idx, err)
			return nil, fmt.Errorf("第%d帧编码失败: %v", idx, err)
		}
		results = append(results, result)
	}

	LoggerGeneral.Info("ImageProcessor", "processAnimatedWithFFmpeg: 处理完成 输出%d帧", len(results))
	return results, nil
}

// extractFramesWithFFmpeg 使用FFmpeg从动态图中提取所有帧，返回解码后的image.Image切片
// 通过image2pipe管道输出PNG流，然后按PNG签名分割为独立帧
func extractFramesWithFFmpeg(input []byte) ([]image.Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ffmpegPath := "ffmpeg"
	if *GeneralConfig.FfmpegPath != "" {
		ffmpegPath = *GeneralConfig.FfmpegPath
	}

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-i", "pipe:0",
		"-f", "image2pipe",
		"-vcodec", "png",
		"pipe:1",
	)

	stdoutBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = errBuf

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("FFmpeg帧提取超时（60秒）")
		}
		return nil, fmt.Errorf("FFmpeg帧提取失败: %v, stderr: %s", err, errBuf.String())
	}

	if stdoutBuf.Len() == 0 {
		return nil, fmt.Errorf("FFmpeg帧提取输出为空")
	}

	return splitPNGFrames(stdoutBuf.Bytes())
}

// splitPNGFrames 按PNG文件签名分割连续PNG流为独立帧
// PNG签名: 89 50 4E 47 0D 0A 1A 0A
func splitPNGFrames(data []byte) ([]image.Image, error) {
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	var frames []image.Image
	offset := 0

	for offset < len(data) {
		// 查找下一个PNG签名
		idx := bytes.Index(data[offset:], pngSig)
		if idx == -1 {
			break
		}
		idx += offset

		// 查找下一个PNG签名（确定当前帧结束位置）
		nextIdx := bytes.Index(data[idx+len(pngSig):], pngSig)
		var frameData []byte
		if nextIdx == -1 {
			frameData = data[idx:]
		} else {
			frameData = data[idx : idx+len(pngSig)+nextIdx]
		}

		// 解码PNG帧
		img, _, err := image.Decode(bytes.NewReader(frameData))
		if err != nil {
			LoggerGeneral.Error("ImageProcessor", "splitPNGFrames: 解码第%d帧失败: %v", len(frames), err)
			offset = idx + len(frameData)
			continue
		}

		frames = append(frames, img)
		offset = idx + len(frameData)

		// 安全限制：最多读取300帧
		if len(frames) >= 300 {
			LoggerGeneral.Info("ImageProcessor", "splitPNGFrames: 已达300帧上限，停止读取")
			break
		}
	}

	if len(frames) == 0 {
		return nil, fmt.Errorf("未能从PNG流中解析出任何帧")
	}

	LoggerGeneral.Info("ImageProcessor", "splitPNGFrames: 成功解析%d帧", len(frames))
	return frames, nil
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
			LoggerGeneral.Error("ImageProcessor", "截取显示器 %d 失败: %v", i, err)
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
