package lunar

import (
	"strings"
	"testing"
)

// TestLocalPathToRel 钉住运行时画作的路径归一化规则。
func TestLocalPathToRel(t *testing.T) {
	cases := map[string]string{
		"/file/read/multimedia/images/a.png":                                "multimedia/images/a.png",
		"D:\\data\\local_data\\multimedia\\images\\a.png":                   "multimedia/images/a.png",
		"multimedia/images/a.png":                                           "multimedia/images/a.png",
		"http://127.0.0.1:36789/file/read/multimedia/a.png":                 "multimedia/a.png",
		"http://127.0.0.1:36789/file/miniature/multimedia/a.png?tag=@I.001": "multimedia/a.png",
		"":             "",
		"audios/a.mp3": "",
	}
	for raw, want := range cases {
		if got := localPathToRel(raw); got != want {
			t.Fatalf("localPathToRel(%q) = %q; want %q", raw, got, want)
		}
	}
}

// TestSetRuntimeImage 钉住运行时消息的形态：文本带句柄，图片是视觉副本地址。
func TestSetRuntimeImage(t *testing.T) {
	role := &PainterRole{ModelBuilder: newModelBuilder("", "绘制者", 40)}
	vision := "http://127.0.0.1:36789/file/miniature/multimedia/images/a.png?tag=@I.001"
	role.setRuntimeImage("@I.001", vision, "最新作品")

	if len(role.RuntimeMessages) != 1 {
		t.Fatalf("应只有一条运行时消息，实际 %d 条", len(role.RuntimeMessages))
	}
	items, ok := role.RuntimeMessages[0].Content.([]ContentItem)
	if !ok || len(items) != 2 {
		t.Fatalf("运行时消息应为文本+图片两项，实际 %#v", role.RuntimeMessages[0].Content)
	}
	if items[0].Type != ContentText || !strings.Contains(items[0].Text, "@I.001") {
		t.Fatalf("运行时消息文本应包含句柄，实际 %q", items[0].Text)
	}
	if items[1].Type != ContentImageURL || items[1].ImageURL == nil || items[1].ImageURL.URL != vision {
		t.Fatalf("运行时消息图片地址不符: %#v", items[1])
	}
}
