package ArchiveReader

// pathfind_test.go 归档解析的路径定位验收：三种路径形式、句柄与歧义候选。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

// TestResolveArchiveByPathForms 完整路径 / 相对路径 / 纯文件名定位到同一压缩包。
func TestResolveArchiveByPathForms(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	resourcehub.Invalidate()
	t.Cleanup(func() {
		resourcehub.Invalidate()
		config.LocalDir = oldDir
	})

	writeTestZip(t, "unknown/pkg.zip", map[string][]byte{"a.txt": []byte("alpha")})
	for _, form := range []string{"unknown/pkg.zip", "pkg.zip"} {
		abs, rel, err := ResolveArchivePath(form)
		if err != nil {
			t.Fatalf("%q 定位失败: %v", form, err)
		}
		if rel != "unknown/pkg.zip" || abs == "" {
			t.Fatalf("%q 定位结果错误: %s / %s", form, rel, abs)
		}
	}
}

// TestResolveArchiveAmbiguous 同名压缩包必须返回候选列表。
func TestResolveArchiveAmbiguous(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	resourcehub.Invalidate()
	t.Cleanup(func() {
		resourcehub.Invalidate()
		config.LocalDir = oldDir
	})

	writeTestZip(t, "unknown/pkg.zip", map[string][]byte{"a.txt": []byte("one")})
	writeTestZip(t, "documents/sub/pkg.zip", map[string][]byte{"b.txt": []byte("two")})
	_, _, err := ResolveArchivePath("pkg.zip")
	if err == nil {
		t.Fatal("同名压缩包应返回歧义错误")
	}
	message := err.Error()
	for _, candidate := range []string{"unknown/pkg.zip", "documents/sub/pkg.zip"} {
		if !strings.Contains(message, candidate) {
			t.Fatalf("歧义提示缺少候选 %s：%s", candidate, message)
		}
	}
}
