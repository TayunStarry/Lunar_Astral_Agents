package module

// 视频/动态图抽帧组元与「图片序列帧」解读链路
//
// 本文件承载两类内容：
//  1. 抽帧基础组元（ExtractVideoFrameAt / EquidistantTimes / MaxVideoFrames / FormatSeconds）
//     —— 上提自 LTP9 引擎的 video.frames 实现（crystal_astral/engine/9.1-Flash/api_media.go），
//     作为全项目唯一的抽帧实现，供插件 API 与月华感知者共用，避免重复算法。
//  2. 序列帧解读链路（VideoToFrames / AnimatedImageToFrames）
//     —— 供不支持 llama-server 「媒体目录 + file:// 引用」机制的架构（如云端 OpenAI 兼容 API）使用：
//     本地抽帧为 base64 图片序列，以多个 image_url 提交，无需服务端解码视频文件。
//     采样网格与分组刻意与 file 模式对齐（0.5fps / 60s 一段），保证两种模式的视觉 token 量级一致。

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"math"
	"os"
	"strconv"
	"time"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// MaxVideoFrames 单次抽帧的最大输出帧数上限（等距抽样时的硬上限）。
const MaxVideoFrames = 60

// MediaFrame 序列帧模式的单帧信息
type MediaFrame struct {
	// Image 帧图片的完整 data URI（data:image/jpeg;base64,...），可直接作为 image_url 提交
	Image string `json:"image"`
	// Time 帧在源视频中的时间点（秒）
	Time float64 `json:"time"`
}

// MediaFrameGroup 序列帧模式的时间分组，与 file 模式的 MediaSegment 同构
type MediaFrameGroup struct {
	// Start 分组起始时间（秒）
	Start float64 `json:"start"`
	// End 分组结束时间（秒）
	End float64 `json:"end"`
	// Frames 该时间分组内的采样帧（按时间顺序）
	Frames []MediaFrame `json:"frames"`
}

// ExtractVideoFrameAt 用 FFmpeg 抽取 t 秒处单帧并缩放至 maxDim（0 表示不缩放），返回 RGBA 图像
func ExtractVideoFrameAt(localPath string, t float64, maxDim int) (*image.RGBA, error) {
	buf := &bytes.Buffer{}
	stream := ffmpeg.Input(localPath, ffmpeg.KwArgs{"ss": strconv.FormatFloat(t, 'f', 3, 64)}).
		Output("pipe:1", ffmpeg.KwArgs{
			"frames:v": "1",
			"f":        "image2pipe",
			"vcodec":   "mjpeg",
			"qscale:v": "2",
		})
	if *GeneralConfig.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(*GeneralConfig.FfmpegPath)
	}
	if err := stream.WithOutput(buf, os.Stderr).Run(); err != nil {
		return nil, fmt.Errorf("FFmpeg 抽取 %s 处画面失败: %v", FormatSeconds(t), err)
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("FFmpeg 在 %s 处无画面输出", FormatSeconds(t))
	}
	img, _, err := image.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("解码 %s 处帧失败: %v", FormatSeconds(t), err)
	}
	rgba := ToRGBA(img)
	if maxDim > 0 {
		rgba = ResizeToFit(rgba, maxDim, maxDim)
	}
	return rgba, nil
}

// EquidistantTimes 生成 n 个等距时间点（恒含时间 0；n=1 时仅返回 0），硬上限 MaxVideoFrames
func EquidistantTimes(n int, duration float64) []float64 {
	if n < 1 {
		n = 1
	}
	if n > MaxVideoFrames {
		n = MaxVideoFrames
	}
	if duration <= 0 || n == 1 {
		return []float64{0}
	}
	ts := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		ts = append(ts, duration*float64(i)/float64(n-1))
	}
	return ts
}

// FormatSeconds 秒 → "HH:MM:SS" 时间刻度字符串
func FormatSeconds(t float64) string {
	d := time.Duration(t * float64(time.Second))
	return fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

// frameSampleTimes 生成分组 [start, end) 内的采样时间点：
// 以 AnimatedSampleFps（0.5fps，与 llama-server 的 --video-fps 一致）为网格步长，
// 与 file 模式由服务端抽帧的时间点分布保持同构（60 秒分组 ≈ 30 帧）。
func frameSampleTimes(start float64, end float64) []float64 {
	span := end - start
	if span <= 0 {
		return []float64{start}
	}
	n := max(int(math.Ceil(span*AnimatedSampleFps)), 1)
	step := 1.0 / AnimatedSampleFps
	times := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		times = append(times, start+float64(i)*step)
	}
	return times
}

// extractFrames 按时间点抽帧并编码为 JPEG data URI；个别帧抽取/编码失败仅跳过，不中断整体抽取
func extractFrames(localPath string, times []float64, quality int) []MediaFrame {
	frames := make([]MediaFrame, 0, len(times))
	for i, t := range times {
		img, err := ExtractVideoFrameAt(localPath, t, MaxMediaWidth)
		if err != nil {
			LoggerGeneral.Warn("MultimodalAnalysis", "序列帧第%d帧(%s)抽取失败，跳过: %v", i, FormatSeconds(t), err)
			continue
		}
		buf := &bytes.Buffer{}
		if err := encodeImage(buf, img, "jpeg", quality); err != nil {
			LoggerGeneral.Warn("MultimodalAnalysis", "序列帧第%d帧(%s)编码失败，跳过: %v", i, FormatSeconds(t), err)
			continue
		}
		frames = append(frames, MediaFrame{
			Image: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
			Time:  t,
		})
	}
	return frames
}

// VideoToFrames 视频 → 按时间分组抽帧为 base64 图片序列（序列帧解读模式）
//
// 输入支持 HTTP(S) URL、data:video/xxx;base64 URI 与本地文件路径。
// 分组与 file 模式的 MediaSegment 一致（每 MediaChunkSeconds 秒一组），
// 组内按 AnimatedSampleFps 采样网格抽帧，宽高不超过 MaxMediaWidth，JPEG 编码为 data URI。
// 不写媒体目录（无需服务端解码视频），全部产物均在内存中返回。
func VideoToFrames(inputFile string) ([]MediaFrameGroup, error) {
	localPath, cleanup, err := localizeVideoSource(inputFile)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	duration, err := GetVideoDuration(localPath)
	if err != nil {
		return nil, fmt.Errorf("获取视频时长失败: %w", err)
	}
	if duration > 3600 {
		return nil, fmt.Errorf("视频时长过长，最大支持1小时的视频")
	}

	quality := *GeneralConfig.JPEGQuality
	segments := max(int((duration-0.001)/MediaChunkSeconds)+1, 1)
	groups := make([]MediaFrameGroup, 0, segments)
	for i := range segments {
		start := float64(i) * MediaChunkSeconds
		end := start + MediaChunkSeconds
		if end > duration {
			end = duration
		}
		frames := extractFrames(localPath, frameSampleTimes(start, end), quality)
		if len(frames) == 0 {
			return nil, fmt.Errorf("视频片段%d未抽取到可用帧", i)
		}
		groups = append(groups, MediaFrameGroup{Start: start, End: end, Frames: frames})
	}

	frameCount := 0
	for _, g := range groups {
		frameCount += len(g.Frames)
	}
	LoggerGeneral.Info("MultimodalAnalysis", "VideoToFrames: 时长 %.2fs → %d 组 / %d 帧, 来源=%s",
		duration, len(groups), frameCount, inputFile)
	return groups, nil
}

// AnimatedImageToFrames 动态图 → 慢放转码后抽帧为 base64 图片序列（序列帧解读模式，单分组）
//
// 与 file 模式一致地先做等比例慢放转码（保证采样点落在动作的不同相位），
// 但转码产物写临时文件而非媒体目录，随后按采样网格抽帧，帧数约等于 AnimatedTargetSamples。
func AnimatedImageToFrames(imgData []byte) ([]MediaFrameGroup, error) {
	if len(imgData) == 0 {
		return nil, fmt.Errorf("动态图数据为空")
	}
	if len(imgData) > maxAnimatedBytes {
		return nil, fmt.Errorf("动态图数据过大，超过50MB限制")
	}
	if !IsAnimatedImage(imgData) {
		return nil, fmt.Errorf("不是动态图（多帧 GIF / APNG / 动态 WebP）")
	}

	// 落到临时文件：ffmpeg 需要按扩展名/探测识别解复用器，管道输入对动态 WebP 不可靠
	format := detectImageFormat(imgData)
	tempFile, err := writeTempImage(imgData, format)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempFile)

	// 探测原始时长：失败时按最小时长处理（等价于最大慢放）
	duration, err := GetVideoDuration(tempFile)
	if err != nil {
		LoggerGeneral.Warn("MultimodalAnalysis", "AnimatedImageToFrames: 动态图时长探测失败(%v)，按最小时长处理", err)
		duration = 0
	}
	target := animatedTargetSeconds(duration)

	// 慢放转码到临时文件（序列帧模式不产出媒体目录文件）
	tmpVideo, err := os.CreateTemp("", "lunar_animated_frames_*.mp4")
	if err != nil {
		return nil, fmt.Errorf("创建临时视频文件失败: %w", err)
	}
	videoPath := tmpVideo.Name()
	tmpVideo.Close()
	defer os.Remove(videoPath)
	if err := encodeAnimatedVideo(tempFile, videoPath, duration, target); err != nil {
		return nil, err
	}

	frames := extractFrames(videoPath, frameSampleTimes(0, target), *GeneralConfig.JPEGQuality)
	if len(frames) == 0 {
		return nil, fmt.Errorf("动态图未抽取到可用帧")
	}

	LoggerGeneral.Info("MultimodalAnalysis", "AnimatedImageToFrames: %s(%d bytes, %.2fs) → %d 帧(慢放 %.2fx, %.2fs)",
		format, len(imgData), duration, len(frames), animatedSlowFactor(duration, target), target)
	return []MediaFrameGroup{{Start: 0, End: target, Frames: frames}}, nil
}