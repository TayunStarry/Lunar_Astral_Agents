package BilibiliViewer

// 手动冒烟：真实下载 60 秒并按统一观看管线理解一个切片（会产生一次模型调用）。
//   $env:BILI_LIFE_UNDERSTAND_PROBE="1"; go test -run TestManualUnderstandProbe -v ./agents/BilibiliViewer

import (
	"os"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestManualUnderstandProbe(t *testing.T) {
	if os.Getenv("BILI_LIFE_UNDERSTAND_PROBE") != "1" {
		t.Skip("设置 BILI_LIFE_UNDERSTAND_PROBE=1 才真实下载并调用模型")
	}
	config.LocalDir = `D:\Lunar_Astral_Agents\local_data`
	info, err := fetchVideoInfo("BV1ponw6vEB9")
	if err != nil {
		t.Fatalf("view 失败: %v", err)
	}
	play, err := fetchPlayURL(info.Bvid, info.Cid, 32)
	if err != nil {
		t.Fatalf("playurl 失败: %v", err)
	}
	local, err := downloadClip(play, info.Bvid, 60)
	if err != nil {
		t.Fatalf("下载失败: %v", err)
	}
	defer os.Remove(local)
	segments, err := prepareVideoSegments(local, 1)
	if err != nil {
		t.Fatalf("切片失败: %v", err)
	}
	text, err := understandOneSlice(local, info, segments[0], 1, len(segments))
	if err != nil {
		t.Fatalf("理解失败: %v", err)
	}
	t.Logf("观影感受(%d 字)=%s", len([]rune(text)), truncate(text, 400))
}
