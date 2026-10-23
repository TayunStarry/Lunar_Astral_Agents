package BilibiliLife

// 手动冒烟：下载 60 秒并切段（不调用模型）。
//   $env:BILI_LIFE_DOWNLOAD_PROBE="1"; go test -run TestManualDownloadProbe -v ./agents/BilibiliLife

import (
	"os"
	"testing"

	"LunarSubsystem/GeneralConfig"
	module "LunarSubsystem/LunarHarness/engine/multimodal"
)

func TestManualDownloadProbe(t *testing.T) {
	if os.Getenv("BILI_LIFE_DOWNLOAD_PROBE") != "1" {
		t.Skip("设置 BILI_LIFE_DOWNLOAD_PROBE=1 才真实下载")
	}
	*GeneralConfig.LocalDir = `D:\Lunar_Astral_Agents\local_data`
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
	segments, err := module.VideoToMedia(local)
	if err != nil {
		_ = os.Remove(local)
		t.Fatalf("切段失败: %v", err)
	}
	t.Logf("下载 %s → %d 个切片，首个=%s", local, len(segments), segments[0].File)
	for _, seg := range segments {
		_ = os.Remove(*GeneralConfig.LocalDir + "/multimedia/" + seg.File)
	}
	_ = os.Remove(local)
}
