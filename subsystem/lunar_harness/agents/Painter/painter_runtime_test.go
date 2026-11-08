package Painter

// painter_runtime_test.go 钉住绘制者的运行时画作行为：路径归一化、参考图绑定与运行时消息形态。

import (
	"fmt"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// fakeEnv 测试用的绘制环境：记录视觉副本生成结果，可按路径注入失败。
type fakeEnv struct {
	fail map[string]bool
}

func (f fakeEnv) PushImages(images []string) []string { return images }

func (f fakeEnv) VisionCopy(relPath, tag string) (int, int, error) {
	if f.fail[relPath] {
		return 0, 0, fmt.Errorf("视觉副本生成失败")
	}
	return 640, 480, nil
}

func (f fakeEnv) VisionReference(handle, blobPath string) string {
	if handle != "" {
		return "vision:" + handle
	}
	return "vision:" + blobPath
}

func (f fakeEnv) ReadImageBytes(source string) ([]byte, error) { return nil, nil }

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

// TestBindReferenceFallsBackToRuntimeArtwork 未显式声明参考图时沿用最近产出画作：
// 人格对话里的「把刚才那张图改成…」仍能拿到基座，且任务正文不再重复附图。
func TestBindReferenceFallsBackToRuntimeArtwork(t *testing.T) {
	s := &session{env: fakeEnv{}}
	s.artwork = ReferenceState{
		Image:  "multimedia/images/a.png",
		Handle: "@I.001",
		Vision: "http://127.0.0.1:36789/file/miniature/multimedia/images/a.png?tag=@I.001",
		Width:  512,
		Height: 512,
	}
	if err := s.bindReference(nil); err != nil {
		t.Fatalf("沿用运行时画作不应失败: %v", err)
	}

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

// TestBindReferenceExplicitTextOnly 显式任务没有参考图时必须走纯文生图，不再沿用最近画作。
func TestBindReferenceExplicitTextOnly(t *testing.T) {
	s := &session{env: fakeEnv{}}
	s.artwork = ReferenceState{Image: "multimedia/images/a.png", Handle: "@I.001", Vision: "vision:@I.001"}
	if err := s.bindReference(&Reference{Explicit: true}); err != nil {
		t.Fatalf("显式纯文生图不应失败: %v", err)
	}
	if s.reference.Image != "" || s.fromRuntime {
		t.Fatalf("显式纯文生图不应沿用最近画作，实得 %#v（fromRuntime=%v）", s.reference, s.fromRuntime)
	}
	if !s.textOnly {
		t.Fatal("应标记为纯文生图任务")
	}
	if !strings.Contains(s.referenceStatus(), "纯文生图") {
		t.Fatalf("参考图状态应说明纯文生图，实得 %q", s.referenceStatus())
	}
}

// TestBindReferenceMultiple 单步多参考图：第一张作为基座，其余作为附加参考图。
func TestBindReferenceMultiple(t *testing.T) {
	s := &session{env: fakeEnv{}}
	err := s.bindReference(&Reference{
		Handle:   "@I.003",
		Source:   "multimedia/images/a.png",
		Explicit: true,
		Extras: []Reference{
			{Handle: "@I.004", Source: "multimedia/images/b.png"},
		},
	})
	if err != nil {
		t.Fatalf("多参考图不应失败: %v", err)
	}
	if s.reference.Image != "multimedia/images/a.png" || s.reference.Handle != "@I.003" {
		t.Fatalf("第一张参考图应作为基座，实得 %#v", s.reference)
	}
	if len(s.extras) != 1 || s.extras[0].Image != "multimedia/images/b.png" {
		t.Fatalf("附加参考图未绑定，实得 %#v", s.extras)
	}
	items, ok := s.runtime[0].Content.([]agentbase.ContentPart)
	if !ok || len(items) != 3 {
		t.Fatalf("运行时消息应为文本+两张图片，实际 %#v", s.runtime[0].Content)
	}
	if !strings.Contains(items[0].Text, "@I.003") || !strings.Contains(items[0].Text, "@I.004") ||
		!strings.Contains(items[0].Text, "第一张是图生图基座") {
		t.Fatalf("运行时文本未说明参考图职责: %q", items[0].Text)
	}
}

// TestBindReferenceFailureNotSilent 显式参考图视觉副本失败时必须报错，禁止静默降级。
func TestBindReferenceFailureNotSilent(t *testing.T) {
	s := &session{env: fakeEnv{fail: map[string]bool{"multimedia/images/a.png": true}}}
	err := s.bindReference(&Reference{Handle: "@I.003", Source: "multimedia/images/a.png", Explicit: true})
	if err == nil || !strings.Contains(err.Error(), "@I.003") {
		t.Fatalf("参考图视觉副本失败应返回明确错误，实得 %v", err)
	}
}

// TestExplicitReferenceBlocksTextToImage 显式参考图任务必须走图生图，不能改成文生图重画。
func TestExplicitReferenceBlocksTextToImage(t *testing.T) {
	s := &session{forceImg2Img: true}
	if out := s.handleDiffusionGeneration(map[string]any{"prompt": "a cat"}); !strings.Contains(out, "image_to_image") {
		t.Fatalf("带参考图时文生图应被拒绝，实得 %q", out)
	}
	if out := s.handlePortrait(map[string]any{"character": "月华"}); !strings.Contains(out, "image_to_image") {
		t.Fatalf("带参考图时立绘应被拒绝，实得 %q", out)
	}
}

// TestTextOnlyBlocksImageToImage 显式纯文生图任务不得调用图生图。
func TestTextOnlyBlocksImageToImage(t *testing.T) {
	s := &session{textOnly: true}
	out := s.handleImageToImage(map[string]any{"prompt": "a cat"})
	if !strings.Contains(out, "纯文生图") || !strings.Contains(out, "diffusion_generation") {
		t.Fatalf("纯文生图任务调用图生图应被拒绝，实得 %q", out)
	}
}

// TestSetRuntimeImages 钉住运行时消息的形态：文本带句柄，图片是视觉副本地址。
func TestSetRuntimeImages(t *testing.T) {
	s := &session{}
	vision := "http://127.0.0.1:36789/file/miniature/multimedia/images/a.png?tag=@I.001"
	s.setRuntimeImages(ReferenceState{Handle: "@I.001", Vision: vision}, nil, "最新作品")

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
