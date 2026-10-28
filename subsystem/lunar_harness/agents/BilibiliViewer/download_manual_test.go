package BilibiliViewer

// 手动冒烟：下载 60 秒并按统一观看管线切片（不调用模型）。
//   $env:BILI_LIFE_DOWNLOAD_PROBE="1"; go test -run TestManualDownloadProbe -v ./agents/BilibiliViewer

import (
	"os"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestManualDownloadProbe(t *testing.T) {
	if os.Getenv("BILI_LIFE_DOWNLOAD_PROBE") != "1" {
		t.Skip("设置 BILI_LIFE_DOWNLOAD_PROBE=1 才真实下载")
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
	local, err := downloadClip(play, info.Bvid, 120)
	if err != nil {
		t.Fatalf("下载失败: %v", err)
	}
	defer os.Remove(local)
	segments, err := prepareVideoSegments(local, 2)
	if err != nil {
		t.Fatalf("切片失败: %v", err)
	}
	t.Logf("下载 %s → %d 个切片", local, len(segments))
	for _, seg := range segments {
		t.Logf("切片 %.2f-%.2fs", seg.Start, seg.End)
	}
}
