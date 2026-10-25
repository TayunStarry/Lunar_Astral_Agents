package BilibiliViewer

// 手动冒烟：真实浏览器打开视频详情页，滚到评论区，用页面内 fetch 读评论。
//   $env:BILI_LIFE_COMMENTS_PROBE="1"; go test -run TestManualCommentsProbe -v ./agents/BilibiliViewer

import (
	"os"
	"testing"
	"time"
)

func TestManualCommentsProbe(t *testing.T) {
	if os.Getenv("BILI_LIFE_COMMENTS_PROBE") != "1" {
		t.Skip("设置 BILI_LIFE_COMMENTS_PROBE=1 才打开真实浏览器")
	}
	info, err := fetchVideoInfo("BV1ponw6vEB9")
	if err != nil {
		t.Fatalf("view 失败: %v", err)
	}
	browser, err := openBilibiliBrowser()
	if err != nil {
		t.Fatalf("打开浏览器失败: %v", err)
	}
	defer browser.close()
	if err := browser.navigate(info.URL, 30*time.Second); err != nil {
		t.Fatalf("打开详情页失败: %v", err)
	}
	browser.wait(2 * time.Second)
	browser.scrollToComments()
	comments, err := browser.readCommentsAPI(info.Aid, 15)
	if err != nil {
		t.Fatalf("读评论失败: %v", err)
	}
	t.Logf("读取评论 %d 条", len(comments))
	for i, c := range comments {
		if i >= 3 {
			break
		}
		t.Logf("%d. %s：%s", i+1, c.User, truncate(c.Content, 80))
	}
}
