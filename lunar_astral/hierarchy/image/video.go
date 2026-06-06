package image

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

// 支持的视频格式列表
var supportedVideoFormats = []string{".mp4", ".avi", ".mov", ".wmv", ".flv", ".mkv", ".webm", ".m4v"}

// KeyFrame 关键帧结构
type KeyFrame struct {
	// 关键帧文件名
	FilePath string `json:"filePath"`
	// 关键帧时间戳
	Timestamp string `json:"timestamp"`
	// 关键帧编号
	FrameNum int `json:"frameNum"`
	// 关键帧图像数据
	Data []byte `json:"data,omitempty"`
}

// FrameData 存储帧数据和时间戳
type FrameData struct {
	Image     image.Image
	Timestamp int
}

// IsSupportedVideoFormat 检查文件格式是否支持
func IsSupportedVideoFormat(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return slices.Contains(supportedVideoFormats, ext)
}

// VideoKeyframeExtraction 提取视频关键帧
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

	// 初始化关键帧列表
	var keyFrames []KeyFrame
	// 存储前一帧图像，用于计算帧间差异
	var prevImage image.Image
	// 帧提取间隔，单位为秒
	frameInterval := 1
	// 获取视频时长
	duration, err := GetVideoDuration(inputFile)
	// 处理获取时长失败的情况
	if err != nil {
		return nil, fmt.Errorf("获取视频时长失败: %w", err)
	}
	// 检查视频时长是否合理
	if duration > 3600 { // 限制最大处理时长为1小时
		return nil, fmt.Errorf("视频时长过长，最大支持1小时的视频")
	}

	// 计算需要处理的帧数
	frameCount := int(duration) / frameInterval
	if int(duration)%frameInterval > 0 {
		frameCount++
	}

	// 串行提取帧数据，避免FFmpeg并发读取同一文件导致数据损坏
	var allFrames []FrameData
	for i := 0; float64(i) < duration; i += frameInterval {
		buf := new(bytes.Buffer)
		err := ExtractKeyFrames(inputFile, i, buf)
		if err != nil {
			logger.Error("LunarCore", "提取帧失败 %d 秒: %v", i, err)
			continue
		}
		if buf.Len() == 0 {
			logger.Error("LunarCore", "提取的帧数据为空 %d 秒", i)
			continue
		}
		currImage, err := jpeg.Decode(buf)
		if err != nil {
			logger.Error("LunarCore", "解码图像失败: %v", err)
			continue
		}
		allFrames = append(allFrames, FrameData{Image: currImage, Timestamp: i})
	}

	// 按时间戳排序帧数据
	if len(allFrames) > 0 {
		// 对帧数据按照时间戳从小到大排序
		for i := 0; i < len(allFrames)-1; i++ {
			for j := i + 1; j < len(allFrames); j++ {
				if allFrames[i].Timestamp > allFrames[j].Timestamp {
					allFrames[i], allFrames[j] = allFrames[j], allFrames[i]
				}
			}
		}
	}

	// 处理排序后的帧数据
	for _, frameData := range allFrames {
		// 检查是否不是第一帧
		if prevImage != nil {
			// 计算当前帧与前一帧的差异
			diff := CalculateImageDifference(prevImage, frameData.Image)
			// 如果差异小于等于阈值(0.45)，则跳过当前帧
			if diff <= 0.45 {
				continue
			}
			// 创建关键帧文件
			frameFileName, frameDataBytes, err := CreateKeyframeFile(frameData.Image, keyFrames)
			// 处理创建关键帧文件失败的情况
			if err != nil {
				logger.Error("LunarCore", "创建关键帧文件失败: %v", err)
				continue
			}
			// 格式化时间戳
			timestampStr := FormatTime(frameData.Timestamp)
			// 将当前帧添加到关键帧列表
			keyFrames = append(keyFrames, CreateKeyFrame(frameFileName, timestampStr, len(keyFrames), frameDataBytes))

		} else {
			// 创建关键帧文件
			frameFileName, frameDataBytes, err := CreateKeyframeFile(frameData.Image, keyFrames)
			// 处理创建关键帧文件失败的情况
			if err != nil {
				logger.Error("LunarCore", "创建关键帧文件失败: %v", err)
				continue
			}
			// 格式化时间戳
			timestampStr := FormatTime(frameData.Timestamp)
			// 将第一帧添加到关键帧列表
			keyFrames = append(keyFrames, CreateKeyFrame(frameFileName, timestampStr, len(keyFrames), frameDataBytes))
		}
		// 更新前一帧为当前帧
		prevImage = frameData.Image
	}

	// 检查是否提取到关键帧
	if len(keyFrames) == 0 {
		return nil, fmt.Errorf("未提取到关键帧，请检查视频文件是否正常")
	}
	// 返回提取的关键帧列表
	return keyFrames, nil
}

// GetVideoDuration 获取视频时长
func GetVideoDuration(inputFile string) (float64, error) {
	// 使用ffprobe获取视频信息
	data, err := ffmpeg.Probe(inputFile)
	// 处理ffprobe执行失败的情况
	if err != nil {
		return 0, err
	}
	// 定义JSON输出结构体
	var output map[string]any
	// 解析ffprobe输出为JSON格式
	if err := json.Unmarshal([]byte(data), &output); err != nil {
		return 0, fmt.Errorf("解析ffprobe输出失败: %w", err)
	}
	// 从format部分提取时长
	if format, ok := output["format"].(map[string]interface{}); ok {
		// 检查format中是否包含duration字段
		if durationStr, ok := format["duration"].(string); ok {
			// 解析时长字符串为浮点数
			duration, err := strconv.ParseFloat(durationStr, 64)
			// 处理解析时长失败的情况
			if err != nil {
				return 0, fmt.Errorf("解析时长失败: %w", err)
			}
			// 处理时长为0的情况
			if duration <= 0 {
				return 0, fmt.Errorf("视频时长为0秒")
			}
			// 返回视频时长
			return duration, nil
		}
	}
	// 处理format中未包含duration字段的情况
	return 0, fmt.Errorf("从ffprobe输出中提取时长失败")
}

// CalculateImageDifference 计算两张图像的差异
func CalculateImageDifference(img1, img2 image.Image) float64 {
	// 获取图像边界
	bounds1 := img1.Bounds()
	bounds2 := img2.Bounds()
	// 使用较小的边界以避免索引越界
	minWidth := min(bounds1.Dx(), bounds2.Dx())
	minHeight := min(bounds1.Dy(), bounds2.Dy())
	// 检查任一图像是否为空
	if minWidth == 0 || minHeight == 0 {
		return 1.0
	}
	// 计算总像素数
	totalPixels := minWidth * minHeight
	// 初始化总差异
	totalDiff := 0.0
	// 遍历图像1的每个像素
	for y := 0; y < minHeight; y++ {
		// 遍历图像1的每个像素
		for x := 0; x < minWidth; x++ {
			// 获取像素颜色
			c1 := img1.At(x, y)
			c2 := img2.At(x, y)
			// 转换为RGBA
			r1, g1, b1, _ := c1.RGBA()
			r2, g2, b2, _ := c2.RGBA()
			// 计算RGB差异
			rDiff := float64(r1-r2) / 65535.0
			gDiff := float64(g1-g2) / 65535.0
			bDiff := float64(b1-b2) / 65535.0
			// 计算欧几里得距离
			pixelDiff := (rDiff*rDiff + gDiff*gDiff + bDiff*bDiff) / 3.0
			totalDiff += pixelDiff
		}
	}
	// 归一化差异到[0, 1]
	averageDiff := totalDiff / float64(totalPixels)
	// 返回归一化后的差异
	return averageDiff
}

// FormatTime 格式化时间
func FormatTime(seconds int) string {
	// 处理负数时间
	if seconds <= 0 {
		return "00:00:00"
	}
	// 处理正常时间
	t := time.Duration(seconds) * time.Second
	// 格式化时间为HH:MM:SS
	return fmt.Sprintf("%02d:%02d:%02d", int(t.Hours()), int(t.Minutes())%60, int(t.Seconds())%60)
}

// CreateKeyframeFile 编码关键帧图像为JPEG字节数据
func CreateKeyframeFile(currImage image.Image, keyFrames []KeyFrame) (string, []byte, error) {
	// 生成关键帧文件名（仅作为标识符，不实际写入磁盘）
	frameFileName := fmt.Sprintf("key_frame_%d.jpg", len(keyFrames)+1)
	// 将当前帧转换为RGBA格式，确保通道数为4
	rgbaImage := screenshot.ToRGBA(currImage)
	// 调整图像大小，确保宽高都不超过1024
	resizedImage := screenshot.ResizeToFit(rgbaImage, 1024, 1024)

	// 创建内存缓冲区
	buf := new(bytes.Buffer)
	// 优化JPEG编码参数，提高压缩质量和速度
	opt := &jpeg.Options{
		Quality: 85,
	}
	// 将调整后的图像编码为JPEG格式并写入缓冲区
	if err := jpeg.Encode(buf, resizedImage, opt); err != nil {
		return frameFileName, nil, fmt.Errorf("编码图像失败: %w", err)
	}

	return frameFileName, buf.Bytes(), nil
}

// ExtractKeyFrames 提取视频关键帧
func ExtractKeyFrames(inputFile string, i int, buf *bytes.Buffer) error {
	kwargs1 := ffmpeg.KwArgs{"ss": fmt.Sprintf("%d", i)}
	kwargs2 := ffmpeg.KwArgs{"vframes": 1, "an": "", "f": "image2pipe", "vcodec": "mjpeg"}

	stream := ffmpeg.Input(inputFile, kwargs1).Output("pipe:1", kwargs2)
	if *config.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(*config.FfmpegPath)
	}
	err := stream.WithOutput(buf, os.Stderr).Run()
	return err
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

	// 从URL中提取文件扩展名
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
