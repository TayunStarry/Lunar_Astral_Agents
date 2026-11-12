package storyboard

// slide_test.go 校验幻灯片模板拼接与配音时长解析。

import (
	"encoding/binary"
	"strings"
	"testing"
)

// buildTestWAV 生成一段最小可解析的 PCM WAV（仅用于时长计算）。
func buildTestWAV(sampleRate, byteRate, dataSize int) []byte {
	out := make([]byte, 44+dataSize)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+dataSize))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1)
	binary.LittleEndian.PutUint16(out[22:24], 1)
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(out[32:34], 2)
	binary.LittleEndian.PutUint16(out[34:36], 16)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataSize))
	return out
}

func TestWavDurationMs(t *testing.T) {
	// 8000Hz、每秒 16000 字节、8000 字节数据 = 500ms
	wav := buildTestWAV(8000, 16000, 8000)
	if got := wavDurationMs(wav); got != 500 {
		t.Fatalf("时长 = %d ms，期望 500", got)
	}
	if got := wavDurationMs([]byte("not a wav")); got != 0 {
		t.Fatalf("非法数据应返回 0，实际 %d", got)
	}
}

func TestRenderSlidePage(t *testing.T) {
	html, err := renderSlidePage(slidePage{
		Title: "云海图书馆",
		Count: 2,
		Slides: []slideItem{
			{Title: "第一幕 · 1-1", Image: "../../multimedia/generated/云海/1.png", Audio: "../../audios/generated/云海/voice-01.wav", DurationMs: 4200},
			{Title: "第一幕 · 1-2", Image: "../../multimedia/generated/云海/2.png", DurationMs: 3200},
		},
	})
	if err != nil {
		t.Fatalf("模板渲染失败: %v", err)
	}
	for _, want := range []string{
		"云海图书馆",
		`data-duration="4200"`,
		"voice-01.wav",
		"class=\"slide\"",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("渲染结果缺少 %q", want)
		}
	}
	// 第二镜没有配音时不应输出空的 data-audio。
	if strings.Count(html, "data-audio=") != 1 {
		t.Fatalf("data-audio 数量 = %d，期望 1", strings.Count(html, "data-audio="))
	}
}
