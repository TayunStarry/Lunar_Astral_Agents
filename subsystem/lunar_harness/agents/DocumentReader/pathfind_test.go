package DocumentReader

// pathfind_test.go 文献解析的路径定位验收：句柄 / 完整路径 / 相对路径 / 纯文件名与歧义。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

// setupDocLocator 准备隔离的文档库与句柄库
func setupDocLocator(t *testing.T) {
	t.Helper()
	origLocal, origArchive := config.LocalDir, config.ArchiveDBPath
	config.LocalDir = t.TempDir()
	config.ArchiveDBPath = filepath.Join(config.LocalDir, "database", "handles.db")
	_ = resource.Close()
	resourcehub.Invalidate()
	t.Cleanup(func() {
		_ = resource.Close()
		resourcehub.Invalidate()
		config.LocalDir, config.ArchiveDBPath = origLocal, origArchive
	})
	if err := resource.Init(config.ArchiveDBPath); err != nil {
		t.Fatalf("初始化句柄库失败: %v", err)
	}
}

func writeDoc(t *testing.T, rel, content string) {
	t.Helper()
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestResolveHandlePathForms 三种路径形式解析到同一个文本句柄。
func TestResolveHandlePathForms(t *testing.T) {
	setupDocLocator(t)
	writeDoc(t, "documents/notes/a.txt", "# 标题\n正文内容\n")

	var first string
	for _, form := range []string{"documents/notes/a.txt", "notes/a.txt", "a.txt", "A.TXT"} {
		handle, err := ResolveHandle(form)
		if err != nil {
			t.Fatalf("%q 解析句柄失败: %v", form, err)
		}
		if first == "" {
			first = handle
		} else if handle != first {
			t.Fatalf("%q 应解析到同一句柄 %s，实际 %s", form, first, handle)
		}
	}
	profile, err := ProfileOf(first)
	if err != nil || profile == "" {
		t.Fatalf("句柄格式识别失败: %v / %q", err, profile)
	}
}

// TestResolveHandleAmbiguous 同名文件必须返回候选列表。
func TestResolveHandleAmbiguous(t *testing.T) {
	setupDocLocator(t)
	writeDoc(t, "documents/notes/a.txt", "one")
	writeDoc(t, "documents/archive/a.txt", "two")

	_, err := ResolveHandle("a.txt")
	if err == nil {
		t.Fatal("同名文件应返回歧义错误")
	}
	for _, candidate := range []string{"documents/notes/a.txt", "documents/archive/a.txt"} {
		if !strings.Contains(err.Error(), candidate) {
			t.Fatalf("歧义提示缺少候选 %s：%s", candidate, err.Error())
		}
	}
}
