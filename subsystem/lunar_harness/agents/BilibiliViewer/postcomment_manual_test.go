package BilibiliViewer

// 手动冒烟：真实发一条评论（不下载视频）。用于验证 CSRF/评论接口。
//   $env:BILI_LIFE_COMMENT_POST="1"; go test -run TestManualPostComment -v ./agents/BilibiliViewer

import (
	"os"
	"testing"
	"time"
)

func TestManualPostComment(t *testing.T) {
	if os.Getenv("BILI_LIFE_COMMENT_POST") != "1" {
		t.Skip("设置 BILI_LIFE_COMMENT_POST=1 才真实发评论")
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
	info, err := fetchVideoInfo("BV1ponw6vEB9")
	if err != nil {
		t.Fatalf("view 失败: %v", err)
	}
	if err := browser.postComment(info.Aid, "测试一下评论接口，月华在验证真实浏览器链路。"); err != nil {
		t.Fatalf("发评论失败: %v", err)
	}
	t.Log("评论已发布")
}
