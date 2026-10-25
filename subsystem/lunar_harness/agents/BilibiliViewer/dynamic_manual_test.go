package BilibiliViewer

// 手动冒烟：只发一条动态并 @ 目标（不下载视频、不写评论）。
//   $env:BILI_LIFE_DYNAMIC_PROBE="1"; go test -run TestManualDynamicProbe -v ./agents/BilibiliViewer

import (
	"os"
	"testing"
	"time"
)

func TestManualDynamicProbe(t *testing.T) {
	if os.Getenv("BILI_LIFE_DYNAMIC_PROBE") != "1" {
		t.Skip("设置 BILI_LIFE_DYNAMIC_PROBE=1 才真实发一条动态")
	}
	testLoginWait = 90 * time.Second
	browser, err := openBilibiliBrowser()
	if err != nil {
		t.Fatalf("打开浏览器失败: %v", err)
	}
	defer browser.close()
	if err := browser.ensureLogin(); err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	dynID, err := browser.publishDiary("测试一下日记动态，这条是月华自己发的。")
	if err != nil {
		t.Fatalf("发日记动态失败: %v", err)
	}
	t.Logf("动态已发布 dynId=%s url=https://t.bilibili.com/%s", dynID, dynID)
}
