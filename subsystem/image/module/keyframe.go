package module

import (
	"bytes"
	"config"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"logger"
	"net/http"
	"os"
	"path/filepath"
	"screenshot"
	"slices"
	"strconv"
	"strings"
	"time"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// DefaultFPS 默认关键帧提取频率（每秒5帧）
const DefaultFPS = 5.0

// IsSupportedVideoFormat 检查文件格式是否支持
func IsSupportedVideoFormat(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return slices.Contains(supportedVideoFormats, ext)
}

// VideoKeyframeExtraction 提取视频关键帧（每秒5帧 + 相似度筛选）
//
// 通过 FFmpeg fps 滤镜一次性提取所有帧，再基于图像相似度筛选去重。
// 处理1小时视频的关键帧提取时间不超过3分钟。
func VideoKeyframeExtraction(inputFile string) ([]KeyFrame, error) {
	// 如果输入是HTTP URL，先下载到本地临时文件
	if strings.HasPrefix(inputFile, "http://") || strings.HasPrefix(inputFile, "https://") {
		tempFile, err := downloadToTempFile(inputFile)
		if err != nil {
			return nil, fmt.Errorf("下载视频文件失败: %w", err)
		}
		defer os.Remove(tempFile)
		inputFile = tempFile
	}

	// 获取视频时长
	duration, err := GetVideoDuration(inputFile)
	if err != nil {
		return nil, fmt.Errorf("获取视频时长失败: %w", err)
	}
	if duration > 3600 {
		return nil, fmt.Errorf("视频时长过长，最大支持1小时的视频")
	}

	logger.Info("LunarCore", "开始提取关键帧，视频时长: %.1f秒，提取频率: %.0ffps", duration, DefaultFPS)

	startTime := time.Now()

	// 第一步：使用 FFmpeg fps 滤镜一次性提取所有帧（单次 FFmpeg 调用，高性能）
	allFrames, err := extractAllFramesAtFPS(inputFile, DefaultFPS)
	if err != nil {
		return nil, fmt.Errorf("批量提取帧失败: %w", err)
	}

	extractElapsed := time.Since(startTime)
	logger.Info("LunarCore", "帧提取完成，共 %d 帧，耗时: %v", len(allFrames), extractElapsed)

	if len(allFrames) == 0 {
		return nil, fmt.Errorf("未提取到任何帧，请检查视频文件是否正常")
	}

	// 第二步：基于图像相似度筛选关键帧，去除冗余帧
	var keyFrames []KeyFrame
	var prevImage image.Image

	for i, currImage := range allFrames {
		// 计算与前一帧的相似度
		if prevImage != nil {
			diff := CalculateImageDifference(prevImage, currImage)
			// 相似度阈值 0.45：差异 <= 0.45 视为冗余帧，跳过
			if diff <= 0.45 {
				continue
			}
		}

		// 编码并创建关键帧
		frameFileName, frameDataBytes, err := CreateKeyframeFile(currImage, keyFrames)
		if err != nil {
			logger.Error("LunarCore", "创建关键帧文件失败(帧%d): %v", i, err)
			continue
		}

		// 计算时间戳
		timestamp := float64(i) / DefaultFPS
		timestampStr := FormatTimestamp(timestamp)

		keyFrames = append(keyFrames, CreateKeyFrame(frameFileName, timestampStr, len(keyFrames), frameDataBytes))
		prevImage = currImage
	}

	totalElapsed := time.Since(startTime)
	logger.Info("LunarCore", "关键帧筛选完成，从 %d 帧中保留 %d 帧，总耗时: %v",
		len(allFrames), len(keyFrames), totalElapsed)

	if len(keyFrames) == 0 {
		return nil, fmt.Errorf("未提取到关键帧，请检查视频文件是否正常")
	}

	return keyFrames, nil
}

// extractAllFramesAtFPS 使用 FFmpeg fps 滤镜一次性提取所有帧
//
// 单次 FFmpeg 调用，通过 image2pipe 格式输出连续的 JPEG 图像流，
// 大幅提升提取性能（相比逐帧调用 FFmpeg）。
func extractAllFramesAtFPS(inputFile string, fps float64) ([]image.Image, error) {
	buf := new(bytes.Buffer)

	kwargs := ffmpeg.KwArgs{
		"vf":       fmt.Sprintf("fps=%.0f", fps),
		"f":        "image2pipe",
		"vcodec":   "mjpeg",
		"qscale:v": "2",
	}

	stream := ffmpeg.Input(inputFile).Output("pipe:1", kwargs)
	if *config.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(*config.FfmpegPath)
	}

	err := stream.WithOutput(buf, os.Stderr).Run()
	if err != nil {
		return nil, fmt.Errorf("FFmpeg批量提取帧失败: %w", err)
	}

	if buf.Len() == 0 {
		return nil, fmt.Errorf("FFmpeg输出为空")
	}

	// 解析 JPEG 图像流（image2pipe 格式输出连续的 JPEG 图像）
	// 手动扫描 SOI (0xFF 0xD8) / EOI (0xFF 0xD9) 标记分割每帧，
	// 避免 jpeg.Decode 缓冲区导致的帧边界丢失问题
	var frames []image.Image
	data := buf.Bytes()
	frameCount := 0

	for len(data) > 0 {
		// 查找下一个 SOI 标记
		soiIdx := findMarker(data, 0xD8)
		if soiIdx < 0 {
			break
		}
		data = data[soiIdx:]

		// 查找 EOI 标记（从 SOI 之后开始搜索）
		eoiIdx := findMarker(data[2:], 0xD9)
		if eoiIdx < 0 {
			// 没有找到 EOI，使用所有剩余数据
			eoiIdx = len(data) - 2
		} else {
			eoiIdx += 2 + 2 // SOI偏移 + EOI标记2字节
		}

		// 解码当前帧
		frameData := data[:eoiIdx]
		img, err := jpeg.Decode(bytes.NewReader(frameData))
		if err != nil {
			logger.Warn("LunarCore", "解码帧%d失败（跳过）: %v", frameCount, err)
		} else {
			frames = append(frames, img)
			frameCount++
		}

		data = data[eoiIdx:]
	}

	return frames, nil
}

// findMarker 在字节数组中查找 JPEG 标记 (0xFF 0xXX)
func findMarker(data []byte, marker byte) int {
	for i := 0; i < len(data)-1; i++ {
		if data[i] == 0xFF && data[i+1] == marker {
			return i
		}
	}
	return -1
}

// GetVideoDuration 获取视频时长
func GetVideoDuration(inputFile string) (float64, error) {
	data, err := ffmpeg.Probe(inputFile)
	if err != nil {
		return 0, err
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(data), &output); err != nil {
		return 0, fmt.Errorf("解析ffprobe输出失败: %w", err)
	}

	if format, ok := output["format"].(map[string]interface{}); ok {
		if durationStr, ok := format["duration"].(string); ok {
			duration, err := strconv.ParseFloat(durationStr, 64)
			if err != nil {
				return 0, fmt.Errorf("解析时长失败: %w", err)
			}
			if duration <= 0 {
				return 0, fmt.Errorf("视频时长为0秒")
			}
			return duration, nil
		}
	}
	return 0, fmt.Errorf("从ffprobe输出中提取时长失败")
}

// CalculateImageDifference 计算两张图像的差异（归一化到 [0, 1]）
func CalculateImageDifference(img1, img2 image.Image) float64 {
	bounds1 := img1.Bounds()
	bounds2 := img2.Bounds()

	minWidth := min(bounds1.Dx(), bounds2.Dx())
	minHeight := min(bounds1.Dy(), bounds2.Dy())

	if minWidth == 0 || minHeight == 0 {
		return 1.0
	}

	totalPixels := minWidth * minHeight
	totalDiff := 0.0

	for y := 0; y < minHeight; y++ {
		for x := 0; x < minWidth; x++ {
			c1 := img1.At(x, y)
			c2 := img2.At(x, y)

			r1, g1, b1, _ := c1.RGBA()
			r2, g2, b2, _ := c2.RGBA()

			rDiff := float64(r1-r2) / 65535.0
			gDiff := float64(g1-g2) / 65535.0
			bDiff := float64(b1-b2) / 65535.0

			pixelDiff := (rDiff*rDiff + gDiff*gDiff + bDiff*bDiff) / 3.0
			totalDiff += pixelDiff
		}
	}

	averageDiff := totalDiff / float64(totalPixels)
	return averageDiff
}

// FormatTimestamp 格式化时间戳为 HH:MM:SS 格式
func FormatTimestamp(seconds float64) string {
	if seconds <= 0 {
		return "00:00:00"
	}
	t := time.Duration(seconds * float64(time.Second))
	return fmt.Sprintf("%02d:%02d:%02d", int(t.Hours()), int(t.Minutes())%60, int(t.Seconds())%60)
}

// CreateKeyframeFile 编码关键帧图像为JPEG字节数据
func CreateKeyframeFile(currImage image.Image, keyFrames []KeyFrame) (string, []byte, error) {
	frameFileName := fmt.Sprintf("key_frame_%d.jpg", len(keyFrames)+1)

	rgbaImage := screenshot.ToRGBA(currImage)
	resizedImage := screenshot.ResizeToFit(rgbaImage, 1024, 1024)

	buf := new(bytes.Buffer)
	opt := &jpeg.Options{
		Quality: 85,
	}
	if err := jpeg.Encode(buf, resizedImage, opt); err != nil {
		return frameFileName, nil, fmt.Errorf("编码图像失败: %w", err)
	}

	return frameFileName, buf.Bytes(), nil
}

// CreateKeyFrame 创建关键帧结构体
func CreateKeyFrame(frameFileName string, timestamp string, frameNum int, frameData []byte) KeyFrame {
	return KeyFrame{FilePath: frameFileName, Timestamp: timestamp, FrameNum: frameNum, Data: frameData}
}

// downloadToTempFile 下载HTTP URL的视频文件到本地临时文件
func downloadToTempFile(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP请求返回状态码: %d", resp.StatusCode)
	}

	ext := filepath.Ext(url)
	if ext == "" || !slices.Contains(supportedVideoFormats, strings.ToLower(ext)) {
		ext = ".mp4"
	}

	tempFile, err := os.CreateTemp("", "video_download_*"+ext)
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	tempFileName := tempFile.Name()

	if _, err := io.Copy(tempFile, resp.Body); err != nil {
		tempFile.Close()
		os.Remove(tempFileName)
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	tempFile.Close()

	return tempFileName, nil
}
