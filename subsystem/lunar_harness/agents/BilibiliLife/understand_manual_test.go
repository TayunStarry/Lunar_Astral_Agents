package BilibiliLife

// 手动冒烟：真实下载 60 秒并调用一次描述者（影像）理解（会产生一次模型调用）。
//   $env:BILI_LIFE_UNDERSTAND_PROBE="1"; go test -run TestManualUnderstandProbe -v ./agents/BilibiliLife

import (
	"os"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestManualUnderstandProbe(t *testing.T) {
	if os.Getenv("BILI_LIFE_UNDERSTAND_PROBE") != "1" {
		t.Skip("设置 BILI_LIFE_UNDERSTAND_PROBE=1 才真实下载并调用模型")
	}
	*config.LocalDir = `D:\Lunar_Astral_Agents\local_data`
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
	summary, slices, err := understandVideo(local, info, 1)
	for _, one := range slices {
		t.Logf("slice %d err=%q text=%q", one.Index, one.Error, truncate(one.Text, 120))
	}
	if err != nil {
		t.Fatalf("理解失败: %v", err)
	}
	t.Logf("summary(%d 字)=%s", len([]rune(summary)), truncate(summary, 400))
}
