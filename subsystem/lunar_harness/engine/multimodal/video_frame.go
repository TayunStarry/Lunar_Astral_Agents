package multimodal

// 抽帧基础组元：ExtractVideoFrameAt / EquidistantTimes / MaxVideoFrames / FormatSeconds
//
// 上提自运行时沙箱的 video.frames 实现（engine/runtime/api_media.go），作为全项目唯一的
// 单帧抽取实现。视频与动图的分片、等距采样与观看任务见 watch.go，不在此处重复实现。

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

// ExtractVideoFrameAt 用 FFmpeg 抽取 t 秒处单帧并缩放至 maxDim（0 表示不缩放），返回 RGBA 图像
func ExtractVideoFrameAt(localPath string, t float64, maxDim int) (*image.RGBA, error) {
	buf := &bytes.Buffer{}
	// 一秒钟看一次片，一次调用就抽十几帧：FFmpeg 的版本横幅不能刷进实况日志，
	// 只在失败时把它的 stderr 片段带进错误信息。
	errBuf := &bytes.Buffer{}
	stream := ffmpeg.Input(localPath, ffmpeg.KwArgs{"ss": strconv.FormatFloat(t, 'f', 3, 64)}).
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
			FormatSeconds(t), err, strings.TrimSpace(errBuf.String()))
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
