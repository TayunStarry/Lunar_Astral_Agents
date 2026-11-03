package Painter

// painter_runtime_test.go 钉住绘制者的运行时画作行为：路径归一化、参考图回退与运行时消息形态。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
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

// TestBindReferenceFallsBackToRuntimeArtwork 未指定参考图时沿用最近产出画作：
// 图生图能直接拿到基座，且任务正文不再重复附图（图片已由运行时消息挂载）。
func TestBindReferenceFallsBackToRuntimeArtwork(t *testing.T) {
	s := &session{}
	s.artwork = ReferenceState{
		Image:  "multimedia/images/a.png",
		Handle: "@I.001",
		Vision: "http://127.0.0.1:36789/file/miniature/multimedia/images/a.png?tag=@I.001",
		Width:  512,
		Height: 512,
	}
	s.bindReference(nil)

	if s.reference.Image != s.artwork.Image {
		t.Fatalf("应沿用运行时画作作为参考图，实得 %#v", s.reference)
	}
	if !s.fromRuntime {
		t.Fatal("参考图应标记为来自运行时画作")
	}
	if content, ok := s.buildTaskContent("把风格改成扁平化").(string); !ok || content != "把风格改成扁平化" {
		t.Fatalf("来自运行时画作时任务正文不应重复附图，实得 %#v", s.buildTaskContent("把风格改成扁平化"))
	}
}

// TestSetRuntimeImage 钉住运行时消息的形态：文本带句柄，图片是视觉副本地址。
func TestSetRuntimeImage(t *testing.T) {
	s := &session{}
	vision := "http://127.0.0.1:36789/file/miniature/multimedia/images/a.png?tag=@I.001"
	s.setRuntimeImage("@I.001", vision, "最新作品")

	if len(s.runtime) != 1 {
		t.Fatalf("应只有一条运行时消息，实际 %d 条", len(s.runtime))
	}
	items, ok := s.runtime[0].Content.([]agentbase.ContentPart)
	if !ok || len(items) != 2 {
		t.Fatalf("运行时消息应为文本+图片两项，实际 %#v", s.runtime[0].Content)
	}
	if items[0].Type != "text" || !strings.Contains(items[0].Text, "@I.001") {
		t.Fatalf("运行时消息文本应包含句柄，实际 %q", items[0].Text)
	}
	if items[1].Type != "image_url" || items[1].ImageURL == nil || items[1].ImageURL.URL != vision {
		t.Fatalf("运行时消息图片地址不符: %#v", items[1])
	}
}
