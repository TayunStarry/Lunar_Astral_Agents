package resource

// classify_test.go 声明优先、存储层归位与强制校验（不联网）

import (
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/mediaobject"
)

func TestClassifyDeclaredWins(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(abs, []byte("\x89PNG\r\n\x1a\nfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Classify(ClassifyInput{
		SourceKind: SourceKindPath, Source: abs, LocalPath: abs,
		KindHint: KindAudio,
	})
	if res.Kind != KindAudio || res.Basis != ClassifyBasisDeclared {
		t.Fatalf("显式 Kind 应优先，实际 kind=%s basis=%s", res.Kind, res.Basis)
	}
}

// TestClassifyWithoutDeclarationReturnsEmpty 入站分类只认声明：没有声明就不猜
func TestClassifyWithoutDeclarationReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "mislabelled.mp4")
	if err := os.WriteFile(abs, []byte("\x89PNG\r\n\x1a\nfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Classify(ClassifyInput{SourceKind: SourceKindPath, Source: abs, LocalPath: abs})
	if res.Kind != "" {
		t.Fatalf("未声明类型时不应推断，实际 kind=%s", res.Kind)
	}
	for _, raw := range []string{
		"https://cdn.example.com/abc123",
		"https://example.com/a.gif",
		"https://www.bilibili.com/video/BV1xx411c7mD",
	} {
		if got := Classify(ClassifyInput{SourceKind: SourceKindURL, Source: raw}); got.Kind != "" {
			t.Fatalf("%s 未声明类型时不应推断，实际 %s", raw, got.Kind)
		}
	}
}

// TestDetectStorageKindByMagic 存储层归位仍按字节魔数（仅内部使用，不参与信封分类）
func TestDetectStorageKindByMagic(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "mislabelled.mp4")
	if err := os.WriteFile(abs, []byte("\x89PNG\r\n\x1a\nfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := DetectStorageKind(ClassifyInput{SourceKind: SourceKindPath, Source: abs, LocalPath: abs})
	if res.Kind != KindImage || res.Ext != ".png" || res.Basis != ClassifyBasisMagic {
		t.Fatalf("存储归位应读文件头，实际 kind=%s ext=%s basis=%s", res.Kind, res.Ext, res.Basis)
	}

	videoAbs := filepath.Join(dir, "download")
	mp4 := append([]byte{0x00, 0x00, 0x00, 0x18}, []byte("ftypisom")...)
	mp4 = append(mp4, 0x00, 0x00, 0x00, 0x00)
	if err := os.WriteFile(videoAbs, mp4, 0o644); err != nil {
		t.Fatal(err)
	}
	video := DetectStorageKind(ClassifyInput{SourceKind: SourceKindPath, Source: videoAbs, LocalPath: videoAbs})
	if video.Kind != KindVideo || video.Ext != ".mp4" {
		t.Fatalf("无扩展名视频应按文件头归位，实际 kind=%s ext=%s", video.Kind, video.Ext)
	}
}

// TestDetectStorageKindFallbacks 存储层归位兜底：远程链接 U、无扩展名本地 D
func TestDetectStorageKindFallbacks(t *testing.T) {
	if got := DetectStorageKind(ClassifyInput{SourceKind: SourceKindURL, Source: "https://cdn.example.com/abc123"}); got.Kind != KindURL {
		t.Fatalf("远程无扩展名链接应归网页，实际 %s", got.Kind)
	}
	if got := DetectStorageKind(ClassifyInput{SourceKind: SourceKindPath, Source: "/tmp/blob"}); got.Kind != KindDocument {
		t.Fatalf("本地无扩展名实体应归文件，实际 %s", got.Kind)
	}
}

// TestVideoPlatformHostsShared 共享域名表同时服务于发送侧声明与后端校验
func TestVideoPlatformHostsShared(t *testing.T) {
	for _, raw := range []string{
		"https://www.bilibili.com/video/BV1xx411c7mD",
		"https://b23.tv/abc123",
		"https://www.youtube.com/watch?v=abc123",
		"https://youtu.be/abc123",
		"https://vimeo.com/123456",
	} {
		if !mediaobject.IsVideoPlatformURL(raw) {
			t.Fatalf("%s 应在共享视频平台域名表内", raw)
		}
	}
	if mediaobject.IsVideoPlatformURL("https://example.com/news") {
		t.Fatalf("普通网页不应命中视频平台域名表")
	}
}

// TestVerifyStoredRejectsMismatch 声明类型必须与字节魔数一致
func TestVerifyStoredRejectsMismatch(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "a.png")
	if err := os.WriteFile(png, []byte("\x89PNG\r\n\x1a\nfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyStored(KindImage, png, "image/png"); err != nil {
		t.Fatalf("静态 PNG 应通过校验: %v", err)
	}
	if err := VerifyStored(KindAnimated, png, "image/png"); err == nil {
		t.Fatalf("静态图声明为动图应判定无效")
	}
	if err := VerifyStored(KindVideo, png, "video/mp4"); err == nil {
		t.Fatalf("PNG 声明为视频应判定无效")
	}
	if err := VerifyStored(KindImage, png, "image/jpeg"); err == nil {
		t.Fatalf("声明 MIME 与实际字节不一致应判定无效")
	}

	gif := filepath.Join(dir, "a.gif")
	if err := os.WriteFile(gif, []byte("GIF89a...."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyStored(KindAnimated, gif, "image/gif"); err != nil {
		t.Fatalf("GIF 动图声明应通过校验: %v", err)
	}
	if err := VerifyStored(KindImage, gif, "image/gif"); err == nil {
		t.Fatalf("动图声明为静态图应判定无效")
	}
}
