package multimodal

// video_frame_test.go 抽帧组元：采样时间点不退化、解码严格度统一、亚秒时间可读。

import (
	"testing"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// TestSegmentSampleTimesDistinctAcrossShortAnimation 0.5 秒动图按 48 帧采样：
// 时间点全在 1 秒内，但毫秒精度各不相同，不会重复抽同一画面。
func TestSegmentSampleTimesDistinctAcrossShortAnimation(t *testing.T) {
	times := segmentSampleTimes(MediaSegment{Start: 0, End: 0.5}, WatchFramesMax)
	if len(times) != WatchFramesMax {
		t.Fatalf("0.5 秒应能容纳 %d 个采样点，实际 %d", WatchFramesMax, len(times))
	}
	seen := make(map[string]bool, len(times))
	for _, ts := range times {
		label := FormatPreciseSeconds(ts)
		if label == "00:00:00.000" && ts != 0 {
			t.Fatalf("非零时间点被显示成 0: %.6f", ts)
		}
		if seen[label] {
			t.Fatalf("时间点重复: %s", label)
		}
		seen[label] = true
	}
}

// TestSegmentSampleTimesCapsWhenSpanTooShort 极短切片不能按 48 帧请求，
// 否则多个帧号会命中同一个毫秒时间点。
func TestSegmentSampleTimesCapsWhenSpanTooShort(t *testing.T) {
	times := segmentSampleTimes(MediaSegment{Start: 0, End: 0.02}, WatchFramesMax)
	if len(times) == 0 || len(times) > 20 {
		t.Fatalf("20ms 切片应压缩帧数，实际 %d", len(times))
	}
	seen := make(map[string]bool, len(times))
	for _, ts := range times {
		label := FormatPreciseSeconds(ts)
		if seen[label] {
			t.Fatalf("时间点重复: %s", label)
		}
		seen[label] = true
	}
}

// TestSegmentSampleTimesInsideSegment 常规切片：时间点递增且都落在 [start, end) 内。
func TestSegmentSampleTimesInsideSegment(t *testing.T) {
	times := segmentSampleTimes(MediaSegment{Start: 10, End: 70}, WatchFramesMax)
	if len(times) != WatchFramesMax {
		t.Fatalf("60 秒切片应抽 %d 帧，实际 %d", WatchFramesMax, len(times))
	}
	for i, ts := range times {
		if ts < 10 || ts >= 70 {
			t.Fatalf("第 %d 个时间点越界: %.3f", i+1, ts)
		}
		if i > 0 && ts <= times[i-1] {
			t.Fatalf("时间点未递增: %.3f <= %.3f", ts, times[i-1])
		}
	}
}

// TestFFmpegDecodeArgsUnofficial 所有抽帧统一放开解码严格度，且保留调用方参数。
func TestFFmpegDecodeArgsUnofficial(t *testing.T) {
	args := ffmpegDecodeArgs(ffmpeg.KwArgs{"ss": "1.000"})
	if args["strict"] != "unofficial" {
		t.Fatalf("应设置 strict=unofficial，实际 %v", args["strict"])
	}
	if args["ss"] != "1.000" {
		t.Fatalf("调用方参数被覆盖: %v", args["ss"])
	}
}

// TestFormatPreciseSecondsKeepsSubSecond 亚秒时间点不能被截断成 00:00:00。
func TestFormatPreciseSecondsKeepsSubSecond(t *testing.T) {
	if got := FormatPreciseSeconds(0.042); got != "00:00:00.042" {
		t.Fatalf("亚秒时间格式不符: %s", got)
	}
	if got := FormatSeconds(0.042); got != "00:00:00" {
		t.Fatalf("FormatSeconds 仍应保持整秒刻度: %s", got)
	}
}
