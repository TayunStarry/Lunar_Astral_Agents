package Descriptor

import (
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestEnsureLocalVideoPathReusesLocalCopy(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	name := "33bfb0e20c938527.mp4"
	abs := filepath.Join(config.LocalDir, "multimedia", name)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("fake-video"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 远程 CDN 地址已失效时，应优先复用本地同名副本，而不是直接报 404。
	got, err := ensureLocalVideoPath("https://cdn.example.com/"+name, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("应复用本地副本 %s，实际 %s", abs, got)
	}

	// 句柄标题同样可作为回退依据。
	got, err = ensureLocalVideoPath("https://cdn.example.com/expired", name)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("标题回退失败，实际 %s", got)
	}

	// 本机 URL 指向不存在的规范化路径时，也要回退到同名副本。
	missing := "http://127.0.0.1:36789/file/read/multimedia/video_missing.mp4"
	got, err = ensureLocalVideoPath(missing, name)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("缺失本机路径应回退到同名副本，实际 %s", got)
	}
}
