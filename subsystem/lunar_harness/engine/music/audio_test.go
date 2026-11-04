package music

// audio_test.go 渲染产物自检与归一化的测试（不依赖 fluidsynth）。

import (
	"encoding/binary"
	"math"
	"path/filepath"
	"testing"
)

// testWAV 构造单声道 PCM16 WAV。
func testWAV(samples []int16) *wavPCM {
	fmtChunk := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtChunk[0:2], 1)
	binary.LittleEndian.PutUint16(fmtChunk[2:4], 1)
	binary.LittleEndian.PutUint32(fmtChunk[4:8], 44100)
	binary.LittleEndian.PutUint32(fmtChunk[8:12], 44100*2)
	binary.LittleEndian.PutUint16(fmtChunk[12:14], 2)
	binary.LittleEndian.PutUint16(fmtChunk[14:16], 16)
	data := make([]byte, len(samples)*2)
	for index, sample := range samples {
		binary.LittleEndian.PutUint16(data[index*2:index*2+2], uint16(sample))
	}
	return &wavPCM{Channels: 1, SampleRate: 44100, Bits: 16, FmtChunk: fmtChunk, Data: data}
}

// writeTestWAV 落盘测试音频。
func writeTestWAV(t *testing.T, samples []int16) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.wav")
	if err := writeWAV(path, testWAV(samples)); err != nil {
		t.Fatalf("写入测试 WAV 失败: %v", err)
	}
	return path
}

// sineSamples 生成指定长度与振幅的正弦波。
func sineSamples(seconds, amplitude float64) []int16 {
	count := int(seconds * 44100)
	samples := make([]int16, count)
	for index := 0; index < count; index++ {
		samples[index] = int16(amplitude * math.Sin(2*math.Pi*440*float64(index)/44100))
	}
	return samples
}

func TestInspectAudioReportsSilence(t *testing.T) {
	path := writeTestWAV(t, make([]int16, 44100))
	report, err := InspectAudio(path)
	if err != nil {
		t.Fatalf("自检失败: %v", err)
	}
	if report.OK || !containsIssue(report.Issues, issueAudioSilent) {
		t.Fatalf("静音应判为不可听，实际 %+v", report)
	}
}

func TestInspectAudioReportsClipping(t *testing.T) {
	samples := make([]int16, 44100)
	for index := range samples {
		samples[index] = 32767
	}
	path := writeTestWAV(t, samples)
	report, err := InspectAudio(path)
	if err != nil {
		t.Fatalf("自检失败: %v", err)
	}
	if report.OK || !containsIssue(report.Issues, issueAudioClipped) {
		t.Fatalf("满量程方波应判削波，实际 %+v", report)
	}
}

func TestNormalizeWAVLevelsQuietTone(t *testing.T) {
	path := writeTestWAV(t, sineSamples(1, 3000))
	report, err := normalizeWAV(path)
	if err != nil {
		t.Fatalf("归一化失败: %v", err)
	}
	if report.RMS < 0.10 || report.RMS > 0.16 {
		t.Fatalf("归一化后 RMS = %.3f，期望接近 %.2f", report.RMS, renderTargetRMS)
	}
	if report.Peak > renderPeakCeiling {
		t.Fatalf("归一化后峰值 %.3f 超过上限 %.2f", report.Peak, renderPeakCeiling)
	}
	if !report.OK {
		t.Fatalf("归一化后应通过自检，实际 %v", report.Issues)
	}
}

func TestNormalizeWAVTrimsSilence(t *testing.T) {
	samples := make([]int16, 0, 3*44100)
	samples = append(samples, make([]int16, 22050)...)
	samples = append(samples, sineSamples(0.5, 3000)...)
	samples = append(samples, make([]int16, 22050)...)
	path := writeTestWAV(t, samples)
	report, err := normalizeWAV(path)
	if err != nil {
		t.Fatalf("归一化失败: %v", err)
	}
	if report.Seconds < 0.5 || report.Seconds > 0.75 {
		t.Fatalf("裁剪后时长 = %.3f 秒，期望约 0.6 秒", report.Seconds)
	}
	if report.Peak > renderPeakCeiling {
		t.Fatalf("裁剪后峰值 %.3f 超过上限", report.Peak)
	}
}
