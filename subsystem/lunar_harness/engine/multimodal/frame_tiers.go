package multimodal

// frame_tiers.go 观看抽帧的自适应定档：按切片内容在 16/24/32/48 四档中取帧。
//
// 判定只看 3 张 96px 探测帧（不进模型、不落盘），代价远低于多抽十几张 640px 帧；
// 探测失败时回退常规档，不中断观看流程。

import (
	"image"
	"math"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// autoWatchFrames 按切片时长与画面变化强度决定采样帧数
func autoWatchFrames(localPath string, segment MediaSegment) int {
	return frameTierFor(segment.End-segment.Start, probeMotion(localPath, segment))
}

// frameTierFor 定档规则（纯函数，便于回归测试）
//
// 运动强度（平均绝对亮度差，0-255）：
//
//	< 3   几乎静止 → 16 帧
//	< 10  常规运动 → 24 帧
//	< 22  快节奏   → 32 帧
//	≥ 22  剧烈变化 → 48 帧
//
// 时长只在极长切片上做下限抬升：10 分钟以上的动图即便画面安静，也不能只抽十几帧。
func frameTierFor(span, motion float64) int {
	frames := 24
	switch {
	case motion < motionQuiet:
		frames = WatchFramesMin
	case motion < motionCalm:
		frames = 24
	case motion < motionBusy:
		frames = 32
	default:
		frames = WatchFramesMax
	}
	switch {
	case span > veryLongSegmentSeconds:
		frames = WatchFramesMax
	case span > longSegmentSeconds && frames < 32:
		frames = 32
	}
	return frames
}

// probeMotion 估算切片内的画面变化强度：在 10% / 50% / 90% 处各取一张 96px 探测帧，
// 取相邻两张的最大平均绝对亮度差；采样失败返回 0（按常规档处理）。
func probeMotion(localPath string, segment MediaSegment) float64 {
	span := segment.End - segment.Start
	if span <= 0 {
		return 0
	}
	ratios := []float64{0.1, 0.5, 0.9}
	images := make([]*image.RGBA, 0, len(ratios))
	for _, ratio := range ratios {
		t := segment.Start + span*ratio
		img, err := ExtractVideoFrameAt(localPath, t, motionProbeDim)
		if err != nil {
			logger.Warn(LogModuleMedia, "观看定档探测帧(%s)抽取失败，跳过: %v", FormatSeconds(t), err)
			continue
		}
		images = append(images, img)
	}
	maxDiff := 0.0
	for i := 1; i < len(images); i++ {
		if diff := meanAbsDiff(images[i-1], images[i]); diff > maxDiff {
			maxDiff = diff
		}
	}
	return maxDiff
}

// meanAbsDiff 两张同尺寸 RGBA 图的平均绝对亮度差（ITU-R BT.601 加权）
func meanAbsDiff(a, b *image.RGBA) float64 {
	bounds := a.Bounds().Intersect(b.Bounds())
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return 0
	}
	sum := 0.0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			ai := a.PixOffset(x, y)
			bi := b.PixOffset(x, y)
			la := 0.299*float64(a.Pix[ai]) + 0.587*float64(a.Pix[ai+1]) + 0.114*float64(a.Pix[ai+2])
			lb := 0.299*float64(b.Pix[bi]) + 0.587*float64(b.Pix[bi+1]) + 0.114*float64(b.Pix[bi+2])
			sum += math.Abs(la - lb)
		}
	}
	return sum / float64(width*height)
}
