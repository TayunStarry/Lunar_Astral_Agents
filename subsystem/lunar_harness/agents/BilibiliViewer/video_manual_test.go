package BilibiliViewer

// 手动冒烟：校验 B 站公开 view/playurl 接口（不登录、不下载）。
//   $env:BILI_LIFE_API_PROBE="1"; go test -run TestManualVideoAPIProbe -v ./agents/BilibiliViewer

import (
	"os"
	"testing"
)

func TestManualVideoAPIProbe(t *testing.T) {
	if os.Getenv("BILI_LIFE_API_PROBE") != "1" {
		t.Skip("设置 BILI_LIFE_API_PROBE=1 才请求真实 B 站接口")
	}
	info, err := fetchVideoInfo("BV1ponw6vEB9")
	if err != nil {
		t.Fatalf("view 失败: %v", err)
	}
	t.Logf("view: %s aid=%d cid=%d duration=%d owner=%s", info.Title, info.Aid, info.Cid, info.Duration, info.Owner)
	play, err := fetchPlayURL(info.Bvid, info.Cid, 32)
	if err != nil {
		t.Fatalf("playurl 失败: %v", err)
	}
	if play == "" {
		t.Fatal("playurl 为空")
	}
	t.Logf("playurl 长度=%d 前缀=%s", len(play), truncate(play, 80))
}
