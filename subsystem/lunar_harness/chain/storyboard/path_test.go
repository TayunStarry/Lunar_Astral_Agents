package storyboard

import (
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestPrepareOutputFolderUsesTitleAndSuffix(t *testing.T) {
	old := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = old })

	if got := prepareOutputFolder("云海:图书馆"); got != "云海图书馆" {
		t.Fatalf("标题名应去掉非法字符，实际 %q", got)
	}
	doc := filepath.Join(config.LocalDir, filepath.FromSlash(docRootRel), "云海图书馆"+slideFileExt)
	if err := os.MkdirAll(filepath.Dir(doc), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc, []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := prepareOutputFolder("云海:图书馆"); got != "云海图书馆-2" {
		t.Fatalf("同名幻灯片已存在应追加序号，实际 %q", got)
	}
}

func TestPrepareOutputFolderChecksAudioDir(t *testing.T) {
	old := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = old })

	audioDir := filepath.Join(config.LocalDir, filepath.FromSlash(voiceOutputRoot), "星海")
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := prepareOutputFolder("星海"); got != "星海-2" {
		t.Fatalf("同名配音目录已存在应追加序号，实际 %q", got)
	}
}

func TestDocumentRootUsesLens(t *testing.T) {
	if docRootRel != "documents/lens" {
		t.Fatalf("分镜文档根目录应为 documents/lens，实际 %q", docRootRel)
	}
	if docDisplayRoot != "lens" {
		t.Fatalf("分镜文档展示前缀应为 lens，实际 %q", docDisplayRoot)
	}
}
