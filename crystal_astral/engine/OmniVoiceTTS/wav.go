package OmniVoiceTTS

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// ==== WAV 编解码 ====

// EncodePCMToWAV 将单声道 float32 采样编码为 16bit PCM WAV
func EncodePCMToWAV(samples []float32, sampleRate int) []byte {
	numSamples := len(samples)
	byteRate := sampleRate * 2
	blockAlign := 2
	dataSize := numSamples * 2
	fileSize := 36 + dataSize

	buf := make([]byte, 44+dataSize)

	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], uint32(fileSize))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1)
	binary.LittleEndian.PutUint16(buf[22:], 1)
	binary.LittleEndian.PutUint32(buf[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(buf[28:], uint32(byteRate))
	binary.LittleEndian.PutUint16(buf[32:], uint16(blockAlign))
	binary.LittleEndian.PutUint16(buf[34:], 16)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:], uint32(dataSize))

	for i, sample := range samples {
		v := int32(sample * 32767.0)
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		binary.LittleEndian.PutUint16(buf[44+i*2:], uint16(int16(v)))
	}
	return buf
}

// loadWAVMono24k 读取 WAV 并转为 24kHz 单声道 float32 采样（[-1,1]）
// 支持 PCM16/Float32，双声道自动混单，其他采样率线性重采样
func loadWAVMono24k(path string) ([]float32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 44 {
		return nil, fmt.Errorf("文件过小，不是有效的 WAV")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("不是 RIFF/WAVE 文件")
	}

	// 解析 chunk，定位 fmt 与 data
	var audioFormat, channels, bitsPerSample uint16
	var sampleRate uint32
	var pcm []byte
	var isFloat bool
	off := 12
	for off+8 <= len(data) {
		id := string(data[off : off+4])
		size := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		body := off + 8
		if body+size > len(data) {
			size = len(data) - body
		}
		switch id {
		case "fmt ":
			audioFormat = binary.LittleEndian.Uint16(data[body:])
			channels = binary.LittleEndian.Uint16(data[body+2:])
			sampleRate = binary.LittleEndian.Uint32(data[body+4:])
			bitsPerSample = binary.LittleEndian.Uint16(data[body+14:])
			isFloat = audioFormat == 3
		case "data":
			pcm = data[body : body+size]
		}
		off = body + size
		if size%2 == 1 {
			off++
		}
	}
	if len(pcm) == 0 {
		return nil, fmt.Errorf("缺少 data 块")
	}
	if channels == 0 {
		return nil, fmt.Errorf("无效的声道数")
	}

	// 解码为 float32
	var samples []float32
	switch {
	case isFloat && bitsPerSample == 32:
		n := len(pcm) / 4
		samples = make([]float32, n)
		for i := 0; i < n; i++ {
			samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(pcm[i*4:]))
		}
	case !isFloat && bitsPerSample == 16:
		n := len(pcm) / 2
		samples = make([]float32, n)
		for i := 0; i < n; i++ {
			samples[i] = float32(int16(binary.LittleEndian.Uint16(pcm[i*2:]))) / 32768.0
		}
	default:
		return nil, fmt.Errorf("不支持的格式: format=%d bits=%d（仅支持 PCM16/Float32）", audioFormat, bitsPerSample)
	}

	// 多声道混单
	if channels > 1 {
		n := len(samples) / int(channels)
		mono := make([]float32, n)
		for i := 0; i < n; i++ {
			var sum float32
			for c := 0; c < int(channels); c++ {
				sum += samples[i*int(channels)+c]
			}
			mono[i] = sum / float32(channels)
		}
		samples = mono
	}

	// 线性重采样到 24kHz
	if sampleRate != SampleRate {
		samples = resampleLinear(samples, int(sampleRate), SampleRate)
	}
	return samples, nil
}

// resampleLinear 线性插值重采样
func resampleLinear(src []float32, from, to int) []float32 {
	if from <= 0 || to <= 0 || len(src) == 0 || from == to {
		return src
	}
	outLen := int(float64(len(src)) * float64(to) / float64(from))
	out := make([]float32, outLen)
	ratio := float64(from) / float64(to)
	for i := range out {
		pos := float64(i) * ratio
		i0 := int(pos)
		if i0 >= len(src)-1 {
			out[i] = src[len(src)-1]
			continue
		}
		frac := float32(pos - float64(i0))
		out[i] = src[i0]*(1-frac) + src[i0+1]*frac
	}
	return out
}
