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
		t.Fatalf("目录名应取标题并去掉非法字符，实际 %q", got)
	}
	dir := filepath.Join(config.LocalDir, filepath.FromSlash(docRootRel), "云海图书馆")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := prepareOutputFolder("云海:图书馆"); got != "云海图书馆-2" {
		t.Fatalf("同名目录应追加序号，实际 %q", got)
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
