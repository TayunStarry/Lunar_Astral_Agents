package resource

import (
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestResolveLocalSourceStripsServiceRoutePrefix(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	rel := "multimedia/8929c1c1bdf053bc.jpg"
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("fake-image"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, source := range []string{
		"/file/read/" + rel,
		"file/read/" + rel,
		"/file/miniature/" + rel,
		"file/miniature/" + rel,
	} {
		resolved, err := resolveLocalSource(source)
		if err != nil {
			t.Fatalf("%s 应能解析到本地文件: %v", source, err)
		}
		if resolved != abs {
			t.Fatalf("%s 解析结果错误: %s", source, resolved)
		}
	}
}

func TestIsLocalVideoURL(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	rel := "multimedia/33bfb0e20c938527.mp4"
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("fake-video"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, source := range []string{
		"/file/read/" + rel,
		config.LocalBaseURL() + "/file/read/" + rel,
	} {
		if !IsLocalVideoURL(source) {
			t.Fatalf("%s 应识别为本机视频", source)
		}
	}
	if IsLocalVideoURL("/file/read/multimedia/not-video.jpg") {
		t.Fatal("图片不应识别为本机视频")
	}
}
