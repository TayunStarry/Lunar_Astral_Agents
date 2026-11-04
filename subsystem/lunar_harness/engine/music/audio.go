package music

// audio.go 渲染产物的自检与归一化（纯 Go，无外部依赖）。
//
// fluidsynth 以 16bit PCM 写出 WAV；本文件负责：
// 1) 解析 RIFF/WAVE，测量峰值、RMS、削波比例与时长；
// 2) 裁剪首尾近静音，并把整体响度归一化到目标 RMS（峰值留余量）。

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// wavPCM 解析后的 PCM16 WAV。
type wavPCM struct {
	// Channels 声道数
	Channels int
	// SampleRate 采样率
	SampleRate int
	// Bits 位深
	Bits int
	// FmtChunk 原始 fmt 块内容
	FmtChunk []byte
	// Data PCM 采样数据
	Data []byte
}

// InspectAudio 读取 WAV 并给出可听性报告（不修改文件）。
func InspectAudio(path string) (AudioReport, error) {
	wav, err := readWAV(path)
	if err != nil {
		return AudioReport{}, err
	}
	return analyzeWAV(wav, wav.Data), nil
}

// normalizeWAV 裁剪首尾静音并归一化响度，原地重写 WAV。
// 非 PCM16 音频原样保留，仅返回体检报告。
func normalizeWAV(path string) (AudioReport, error) {
	wav, err := readWAV(path)
	if err != nil {
		return AudioReport{}, err
	}
	if wav.Bits != 16 || wav.Channels <= 0 {
		return analyzeWAV(wav, wav.Data), nil
	}
	samples := int16Samples(wav.Data)
	if len(samples) == 0 {
		return analyzeWAV(wav, wav.Data), nil
	}

	frames := len(samples) / wav.Channels
	peak := framePeaks(samples, wav.Channels)
	if peak <= 0 {
		return analyzeWAV(wav, wav.Data), nil
	}
	threshold := peak * math.Pow(10, -audioSilenceTopDB/20)

	first, last := -1, -1
	for frame := 0; frame < frames; frame++ {
		if framePeakAt(samples, wav.Channels, frame) <= threshold {
			continue
		}
		if first < 0 {
			first = frame
		}
		last = frame
	}
	if first < 0 {
		return analyzeWAV(wav, wav.Data), nil
	}
	pad := int(0.05 * float64(wav.SampleRate))
	if pad < 0 {
		pad = 0
	}
	start := maxInt(0, first-pad)
	end := minInt(frames, last+1+pad)
	kept := samples[start*wav.Channels : end*wav.Channels]

	rms := rmsOf(kept)
	if rms <= 0 {
		return analyzeWAV(wav, wav.Data), nil
	}
	keptPeak := peakOf(kept)
	// 目标响度与峰值上限都以满量程为基准，需换算回原始采样单位后再缩放
	scale := renderTargetRMS * pcmFullScale / rms
	if keptPeak*scale > renderPeakCeiling*pcmFullScale {
		scale = renderPeakCeiling * pcmFullScale / keptPeak
	}

	scaled := make([]byte, len(kept)*2)
	var clipped int
	for index, sample := range kept {
		value := float64(sample) * scale
		rounded := int(math.Round(value))
		if rounded > 32767 {
			rounded = 32767
			clipped++
		}
		if rounded < -32768 {
			rounded = -32768
			clipped++
		}
		binary.LittleEndian.PutUint16(scaled[index*2:index*2+2], uint16(int16(rounded)))
	}
	wav.Data = scaled
	if err := writeWAV(path, wav); err != nil {
		return AudioReport{}, err
	}
	report := analyzeWAV(wav, scaled)
	if len(scaled) > 0 && float64(clipped)/float64(len(kept)) > audioMaxClipRatio {
		report.Issues = appendUnique(report.Issues, issueAudioClipped)
		report.OK = false
	}
	return report, nil
}

// readWAV 解析 RIFF/WAVE 的 fmt 与 data 块。
func readWAV(path string) (*wavPCM, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取音频失败: %w", err)
	}
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, fmt.Errorf("不是合法的 WAV 文件")
	}
	wav := &wavPCM{}
	offset := 12
	for offset+8 <= len(raw) {
		id := string(raw[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(raw[offset+4 : offset+8]))
		bodyStart := offset + 8
		bodyEnd := bodyStart + size
		if bodyEnd > len(raw) {
			bodyEnd = len(raw)
		}
		switch id {
		case "fmt ":
			wav.FmtChunk = raw[bodyStart:bodyEnd]
			if size >= 16 {
				wav.Channels = int(binary.LittleEndian.Uint16(wav.FmtChunk[2:4]))
				wav.SampleRate = int(binary.LittleEndian.Uint32(wav.FmtChunk[4:8]))
				wav.Bits = int(binary.LittleEndian.Uint16(wav.FmtChunk[14:16]))
			}
		case "data":
			wav.Data = raw[bodyStart:bodyEnd]
		}
		offset = bodyEnd
		if size%2 == 1 {
			offset++
		}
	}
	if wav.Channels <= 0 || wav.SampleRate <= 0 || len(wav.Data) == 0 {
		return nil, fmt.Errorf("WAV 缺少 fmt 或 data 块")
	}
	return wav, nil
}

// writeWAV 以标准 PCM 布局重写 WAV。
func writeWAV(path string, wav *wavPCM) error {
	fmtChunk := wav.FmtChunk
	if len(fmtChunk) == 0 {
		return fmt.Errorf("WAV 缺少 fmt 块，无法重写")
	}
	riffSize := 4 + 8 + len(fmtChunk) + 8 + len(wav.Data)
	buffer := make([]byte, 0, 12+riffSize)
	buffer = append(buffer, []byte("RIFF")...)
	buffer = append(buffer, uint32LE(uint32(riffSize))...)
	buffer = append(buffer, []byte("WAVE")...)
	buffer = append(buffer, []byte("fmt ")...)
	buffer = append(buffer, uint32LE(uint32(len(fmtChunk)))...)
	buffer = append(buffer, fmtChunk...)
	buffer = append(buffer, []byte("data")...)
	buffer = append(buffer, uint32LE(uint32(len(wav.Data)))...)
	buffer = append(buffer, wav.Data...)
	if err := os.WriteFile(path, buffer, 0o644); err != nil {
		return fmt.Errorf("写入音频失败: %w", err)
	}
	return nil
}

// analyzeWAV 计算时长、峰值、RMS 与削波比例。
func analyzeWAV(wav *wavPCM, data []byte) AudioReport {
	samples := int16Samples(data)
	report := AudioReport{}
	if len(samples) == 0 || wav.Channels <= 0 || wav.SampleRate <= 0 {
		report.Issues = append(report.Issues, issueAudioSilent)
		return report
	}
	peak := peakOf(samples)
	rms := rmsOf(samples)
	clipped := 0
	for _, sample := range samples {
		abs := float64(sample)
		if abs < 0 {
			abs = -abs
		}
		if abs >= 32760 {
			clipped++
		}
	}
	frames := len(samples) / wav.Channels
	report.Seconds = float64(frames) / float64(wav.SampleRate)
	report.Peak = peak / 32768
	report.RMS = rms / 32768
	report.ClippedRatio = float64(clipped) / float64(len(samples))
	report.OK = true
	if report.Seconds < audioMinSeconds || report.RMS < audioMinRMS {
		report.Issues = append(report.Issues, issueAudioSilent)
		report.OK = false
	}
	if report.ClippedRatio > audioMaxClipRatio {
		report.Issues = append(report.Issues, issueAudioClipped)
		report.OK = false
	}
	return report
}

// int16Samples 把 PCM16 字节流转为采样切片。
func int16Samples(data []byte) []int16 {
	count := len(data) / 2
	samples := make([]int16, count)
	for index := 0; index < count; index++ {
		samples[index] = int16(binary.LittleEndian.Uint16(data[index*2 : index*2+2]))
	}
	return samples
}

// framePeaks 返回整段 PCM 的峰值绝对值。
func framePeaks(samples []int16, channels int) float64 {
	peak := 0.0
	for index := 0; index < len(samples); index += channels {
		if value := math.Abs(float64(samples[index])); value > peak {
			peak = value
		}
	}
	return peak
}

// framePeakAt 返回指定帧的峰值绝对值。
func framePeakAt(samples []int16, channels int, frame int) float64 {
	peak := 0.0
	start := frame * channels
	for index := 0; index < channels; index++ {
		value := math.Abs(float64(samples[start+index]))
		if value > peak {
			peak = value
		}
	}
	return peak
}

// peakOf 返回采样序列的峰值绝对值。
func peakOf(samples []int16) float64 {
	peak := 0.0
	for _, sample := range samples {
		value := math.Abs(float64(sample))
		if value > peak {
			peak = value
		}
	}
	return peak
}

// rmsOf 返回采样序列的均方根。
func rmsOf(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}
	sum := 0.0
	for _, sample := range samples {
		sum += float64(sample) * float64(sample)
	}
	return math.Sqrt(sum / float64(len(samples)))
}

// uint32LE 小端编码。
func uint32LE(value uint32) []byte {
	out := make([]byte, 4)
	binary.LittleEndian.PutUint32(out, value)
	return out
}

// maxInt 整数取大。
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// minInt 整数取小。
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// appendUnique 追加不重复的问题码。
func appendUnique(issues []string, code string) []string {
	for _, issue := range issues {
		if issue == code {
			return issues
		}
	}
	return append(issues, code)
}
