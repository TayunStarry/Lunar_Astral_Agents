package module

import (
	"LunarSubsystem/GeneralConfig"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// MediaChunkSeconds 单个媒体片段的最大时长（秒）。
// llama-server 按视频抽帧频率（--video-fps，默认配置 0.5fps）抽取视觉 token，
// 60 秒片段最多 30 帧，配合 20480 上下文窗口可安全容纳，避免提示超限。
const MediaChunkSeconds = 60.0

// MaxMediaWidth 媒体片段的最大宽度（像素）。
// llama-server 的多模态批处理要求非因果注意力块整体落入单个 ubatch，
// 高分辨率视频单帧的视觉 token 数会超出 n_ubatch 上限，触发 GGML 断言使多模态实例崩溃；
// 重编码到 640px 后每帧 token 数降至安全范围（实测 Gemma 多模态约 100 token/帧）。
const MaxMediaWidth = 640

// NormalizedFPS 重编码片段的输出帧率：低帧率仅用于压缩重编码耗时，
// 实际抽帧频率由 llama-server 端 --video-fps 决定，与此处互不影响
const NormalizedFPS = 10

// MediaSegment 媒体目录内的视频片段信息
type MediaSegment struct {
	// File 媒体目录内的文件名（通过 file:// 引用）
	File string `json:"file"`
	// Start 片段起始时间（秒）
	Start float64 `json:"start"`
	// End 片段结束时间（秒）
	End float64 `json:"end"`
}

// MediaDir llama-server 媒体目录（--media-path 指向的目录，确保以路径分隔符结尾）
func MediaDir() string {
	dir := filepath.Join(*GeneralConfig.LocalDir, "cache", "media") + string(filepath.Separator)
	// 确保目录存在（llama-server 不会自动创建）
	os.MkdirAll(filepath.Dir(dir), 0755)
	return dir
}

// VideoToMedia 将视频本地化并复制/分段到 llama-server 媒体目录
//
// 输入支持 HTTP(S) URL、data:video/xxx;base64 URI 与本地文件路径。
// 时长不超过 MediaChunkSeconds 的视频直接复制为单个媒体文件；
// 更长的视频使用 FFmpeg -c copy 快速切分为多个片段。
// 返回媒体目录内的片段列表（文件名 + 起止时间），由 TypeScript 拼接 file:// 引用。
func VideoToMedia(inputFile string) ([]MediaSegment, error) {
	// 本地化输入：非本地路径先落到临时文件
	localPath := inputFile
	isTemp := false
	if strings.HasPrefix(inputFile, "http://") || strings.HasPrefix(inputFile, "https://") {
		tempFile, err := downloadToTempFile(inputFile)
		if err != nil {
			return nil, fmt.Errorf("下载视频文件失败: %w", err)
		}
		localPath, isTemp = tempFile, true
	} else if strings.HasPrefix(inputFile, "data:video/") {
		tempFile, err := dataURIToTempFile(inputFile)
		if err != nil {
			return nil, fmt.Errorf("解码视频数据失败: %w", err)
		}
		localPath, isTemp = tempFile, true
	}
	if isTemp {
		defer os.Remove(localPath)
	}

	// 校验本地文件存在
	if _, err := os.Stat(localPath); err != nil {
		return nil, fmt.Errorf("视频文件不存在: %s", localPath)
	}

	// 探测视频时长
	duration, err := GetVideoDuration(localPath)
	if err != nil {
		return nil, fmt.Errorf("获取视频时长失败: %w", err)
	}
	if duration > 3600 {
		return nil, fmt.Errorf("视频时长过长，最大支持1小时的视频")
	}

	// 以输入源稳定哈希作为媒体文件名，同一视频重复处理直接覆盖
	hash := fmt.Sprintf("%x", sha1.Sum([]byte(inputFile)))
	// 宽度超过安全上限（或探测失败）的源必须重编码归一化，防止 llama-server 端 n_ubatch 断言崩溃
	needNormalize := true
	if width, err := GetVideoWidth(localPath); err == nil && width > 0 && width <= MaxMediaWidth {
		needNormalize = false
	}
	ext := strings.ToLower(filepath.Ext(localPath))
	if needNormalize || !IsSupportedVideoFormat("v"+ext) {
		ext = ".mp4"
	}

	// 计算分段数
	segments := int((duration-0.001)/MediaChunkSeconds) + 1
	if segments < 1 {
		segments = 1
	}

	mediaDir := MediaDir()
	result := make([]MediaSegment, 0, segments)
	for i := 0; i < segments; i++ {
		start := float64(i) * MediaChunkSeconds
		end := start + MediaChunkSeconds
		if end > duration {
			end = duration
		}
		// 单片段视频直接复制原始文件，多片段才需要 FFmpeg 切分
		fileName := fmt.Sprintf("%s%s", hash, ext)
		if segments > 1 {
			fileName = fmt.Sprintf("%s_seg%d%s", hash, i, ext)
		}
		outPath := filepath.Join(mediaDir, fileName)
		var err error
		if needNormalize {
			// 归一化重编码：输入侧定位片段起点，重编码后边界逐帧精确
			err = normalizeVideoSegment(localPath, outPath, start, end-start)
		} else if segments > 1 {
			err = cutVideoSegment(localPath, outPath, start, end-start)
		} else {
			err = copyFile(localPath, outPath)
		}
		if err != nil {
			return nil, fmt.Errorf("写入视频片段%d失败: %w", i, err)
		}
		result = append(result, MediaSegment{File: fileName, Start: start, End: end})
	}
	return result, nil
}

// cutVideoSegment 使用 FFmpeg 流复制切分视频片段（不重编码，速度快，仅用于已达安全分辨率的源）
func cutVideoSegment(inputFile string, outFile string, start float64, duration float64) error {
	// FFmpeg 覆盖输出需要 -y 标志，ffmpeg-go 无法正确传递布尔参数，改为提前删除旧文件
	os.Remove(outFile)
	stream := ffmpeg.Input(inputFile).
		Output(outFile, ffmpeg.KwArgs{
			"ss":       fmt.Sprintf("%.3f", start),
			"t":        fmt.Sprintf("%.3f", duration),
			"c":        "copy",
			"map":      "0",
			"movflags": "+faststart",
			"loglevel": "error",
		})
	if *GeneralConfig.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(*GeneralConfig.FfmpegPath)
	}
	return stream.Run()
}

// normalizeVideoSegment 重编码视频片段到安全规格（宽度 ≤ MaxMediaWidth、帧率 NormalizedFPS、H.264/yuv420p）
//
// 输入侧 -ss 定位片段起点（重编码路径下高效且边界逐帧精确），
// 仅保留视频流；缩放表达式中的逗号需转义，避免被 FFmpeg 当作滤镜分隔符。
func normalizeVideoSegment(inputFile string, outFile string, start float64, duration float64) error {
	// FFmpeg 覆盖输出需要 -y 标志，ffmpeg-go 无法正确传递布尔参数，改为提前删除旧文件
	os.Remove(outFile)
	input := ffmpeg.Input(inputFile)
	if start > 0 {
		input = ffmpeg.Input(inputFile, ffmpeg.KwArgs{"ss": fmt.Sprintf("%.3f", start)})
	}
	stream := input.Output(outFile, ffmpeg.KwArgs{
		"t":        fmt.Sprintf("%.3f", duration),
		"vf":       fmt.Sprintf("scale=trunc(min(%d\\,iw)/2)*2:-2,fps=%d,format=yuv420p", MaxMediaWidth, NormalizedFPS),
		"c:v":      "libx264",
		"preset":   "veryfast",
		"crf":      "23",
		"map":      "0:v:0",
		"movflags": "+faststart",
		"loglevel": "error",
	})
	if *GeneralConfig.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(*GeneralConfig.FfmpegPath)
	}
	if err := stream.Run(); err != nil {
		return err
	}
	// 校验输出非空，防止半截文件进入媒体目录
	if info, err := os.Stat(outFile); err != nil {
		return fmt.Errorf("输出文件缺失: %w", err)
	} else if info.Size() == 0 {
		return fmt.Errorf("输出文件为空")
	}
	return nil
}

// GetVideoWidth 获取视频首条视频流的宽度（像素），无视频流或探测失败时返回错误
func GetVideoWidth(inputFile string) (int, error) {
	data, err := ffmpeg.Probe(inputFile)
	if err != nil {
		return 0, err
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(data), &output); err != nil {
		return 0, fmt.Errorf("解析ffprobe输出失败: %w", err)
	}
	streams, ok := output["streams"].([]any)
	if !ok {
		return 0, fmt.Errorf("ffprobe输出中无streams字段")
	}
	for _, s := range streams {
		if m, ok := s.(map[string]any); ok && m["codec_type"] == "video" {
			if w, ok := m["width"].(float64); ok && w > 0 {
				return int(w), nil
			}
		}
	}
	return 0, fmt.Errorf("未找到视频流")
}

// copyFile 复制文件到目标路径
func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// dataURIToTempFile 将 data:video/xxx;base64 URI 解码为本地临时文件
func dataURIToTempFile(uri string) (string, error) {
	parts := strings.SplitN(uri, ",", 2)
	if len(parts) != 2 || !strings.HasSuffix(parts[0], "base64") {
		return "", fmt.Errorf("无效的 data URI 格式")
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %w", err)
	}
	tempFile, err := os.CreateTemp("", "video_media_*.mp4")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	tempFile.Close()
	return tempFile.Name(), nil
}
