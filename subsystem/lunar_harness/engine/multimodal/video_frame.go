package multimodal

// 抽帧基础组元：ExtractVideoFrameAt / EquidistantTimes / MaxVideoFrames / FormatSeconds / FormatPreciseSeconds
//
// 上提自运行时沙箱的 video.frames 实现（engine/runtime/api_media.go），作为全项目唯一的
// 单帧抽取实现。视频与动图的分片、等距采样与观看任务见 watch.go，不在此处重复实现。
//
// 解码参数也统一收在 ffmpegDecodeArgs：mjpeg 等非标准流需要 strict=unofficial，
// 任何抽帧调用都从这里取参数，不在各自函数里另写一份。

import (
	"bytes"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// MaxVideoFrames 单次抽帧的最大输出帧数上限（等距抽样时的硬上限）。
const MaxVideoFrames = 60

// ffmpegDecodeArgs 所有抽帧调用共用的输入/解码参数。
//
// strict=unofficial 对应 strict_std_compliance ≤ unofficial：mjpeg 等流可能采用
// 非全范围 YUV，不放开严格度会直接报 "Non full-range YUV is non-standard" 并失败。
func ffmpegDecodeArgs(extra ffmpeg.KwArgs) ffmpeg.KwArgs {
	args := ffmpeg.KwArgs{"strict": "unofficial"}
	for key, value := range extra {
		args[key] = value
	}
	return args
}

// ExtractVideoFrameAt 用 FFmpeg 抽取 t 秒处单帧并缩放至 maxDim（0 表示不缩放），返回 RGBA 图像
func ExtractVideoFrameAt(localPath string, t float64, maxDim int) (*image.RGBA, error) {
	buf := &bytes.Buffer{}
	// 一秒钟看一次片，一次调用就抽十几帧：FFmpeg 的版本横幅不能刷进实况日志，
	// 只在失败时把它的 stderr 片段带进错误信息。
	errBuf := &bytes.Buffer{}
	stream := ffmpeg.Input(localPath, ffmpegDecodeArgs(ffmpeg.KwArgs{"ss": strconv.FormatFloat(t, 'f', 3, 64)})).
		Output("pipe:1", ffmpeg.KwArgs{
			"frames:v": "1",
			"f":        "image2pipe",
			"vcodec":   "mjpeg",
			"qscale:v": "2",
			"loglevel": "error",
		})
	if config.FfmpegPath != "" {
		stream = stream.SetFfmpegPath(config.FfmpegPath)
	}
	if err := stream.WithOutput(buf, errBuf).Run(); err != nil {
		return nil, fmt.Errorf("FFmpeg 抽取 %s 处画面失败: %v %s",
			FormatPreciseSeconds(t), err, strings.TrimSpace(errBuf.String()))
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("FFmpeg 在 %s 处无画面输出", FormatPreciseSeconds(t))
	}
	img, _, err := image.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("解码 %s 处帧失败: %v", FormatPreciseSeconds(t), err)
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

// FormatPreciseSeconds 秒 → "HH:MM:SS.mmm" 时间刻度字符串。
//
// 抽帧失败排查必须用毫秒精度：动图总时长可能不足 1 秒，用 FormatSeconds
// 会把 48 个不同的请求时间点全显示成 00:00:00，看上去像是重复抽同一帧。
func FormatPreciseSeconds(t float64) string {
	d := time.Duration(t * float64(time.Second))
	return fmt.Sprintf("%02d:%02d:%02d.%03d",
		int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60, d.Milliseconds()%1000)
}
