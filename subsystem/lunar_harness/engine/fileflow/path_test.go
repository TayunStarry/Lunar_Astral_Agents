package fileflow

// path_test.go 路径解析的回归：LocalDir 是相对路径时，调用方传入「相对 LocalDir」的
// 路径不能被再补一层 documents/（历史快照曾记成 documents/local_data/documents/…）。

import (
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestResolveStripsLocalDirPrefix(t *testing.T) {
	original := config.LocalDir
	config.LocalDir = "local_data"
	defer func() { config.LocalDir = original }()

	_, rel, err := Resolve(filepath.Join("local_data", "documents", "项目"))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if rel != "documents/项目" {
		t.Fatalf("相对 LocalDir 的路径应剥掉前缀，实际 %q", rel)
	}
}

func TestResolveAbsolutePathWithRelativeLocalDir(t *testing.T) {
	original := config.LocalDir
	config.LocalDir = "local_data"
	defer func() { config.LocalDir = original }()

	abs, err := filepath.Abs(filepath.Join("local_data", "documents", "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, rel, err := Resolve(abs)
	if err != nil {
		t.Fatalf("绝对路径解析失败: %v", err)
	}
	if rel != "documents/a.md" {
		t.Fatalf("绝对路径应解析为相对 LocalDir 的路径，实际 %q", rel)
	}
}
