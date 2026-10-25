package BilibiliViewer

// 手动冒烟：真实首页/搜索页选片，验证参考词打分与涉政/对立过滤。
//   $env:BILI_LIFE_SELECT_PROBE="1"; go test -run TestManualSelectionProbe -v ./agents/BilibiliViewer

import (
	"os"
	"testing"
)

func TestManualSelectionProbe(t *testing.T) {
	if os.Getenv("BILI_LIFE_SELECT_PROBE") != "1" {
		t.Skip("设置 BILI_LIFE_SELECT_PROBE=1 才打开真实浏览器选片")
	}
	cfg := LoadConfig()
	browser, err := openBilibiliBrowser()
	if err != nil {
		t.Fatalf("打开浏览器失败: %v", err)
	}
	defer browser.close()
	link, err := browser.selectVideo(cfg, nil)
	if err != nil {
		t.Fatalf("选片失败: %v", err)
	}
	t.Logf("选中: score=%d title=%s href=%s", link.Score, link.Title, link.Href)
}
