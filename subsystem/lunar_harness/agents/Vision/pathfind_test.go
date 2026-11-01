package Vision

// pathfind_test.go 视觉理解的路径定位验收：句柄 / 完整路径 / 相对路径 / 纯文件名与歧义。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/locator"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// setupVisionLocator 准备隔离的媒体目录与句柄库
func setupVisionLocator(t *testing.T) {
	t.Helper()
	origLocal, origArchive := config.LocalDir, config.ArchiveDBPath
	config.LocalDir = t.TempDir()
	config.ArchiveDBPath = filepath.Join(config.LocalDir, "database", "handles.db")
	_ = resource.Close()
	locator.Invalidate()
	t.Cleanup(func() {
		_ = resource.Close()
		locator.Invalidate()
		config.LocalDir, config.ArchiveDBPath = origLocal, origArchive
	})
	if err := resource.Init(config.ArchiveDBPath); err != nil {
		t.Fatalf("初始化句柄库失败: %v", err)
	}
}

func writeVisionFile(t *testing.T, rel string) {
	t.Helper()
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("fake-image"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestResolveMediaPathForms 三种路径形式解析到同一个图片句柄。
func TestResolveMediaPathForms(t *testing.T) {
	setupVisionLocator(t)
	writeVisionFile(t, "multimedia/pic.png")

	var first string
	for _, form := range []string{"multimedia/pic.png", "pic.png", "PIC.PNG"} {
		handle, err := ResolveHandle(form)
		if err != nil {
			t.Fatalf("%q 解析句柄失败: %v", form, err)
		}
		if !strings.HasPrefix(handle, "@I.") {
			t.Fatalf("%q 应解析为图片句柄，实际 %s", form, handle)
		}
		if first == "" {
			first = handle
		} else if handle != first {
			t.Fatalf("%q 应解析到同一句柄 %s，实际 %s", form, first, handle)
		}
	}
}

// TestResolveMediaAmbiguous 同名图片必须返回候选列表。
func TestResolveMediaAmbiguous(t *testing.T) {
	setupVisionLocator(t)
	writeVisionFile(t, "multimedia/pic.png")
	writeVisionFile(t, "documents/notes/pic.png")

	_, err := ResolveHandle("pic.png")
	if err == nil {
		t.Fatal("同名图片应返回歧义错误")
	}
	for _, candidate := range []string{"multimedia/pic.png", "documents/notes/pic.png"} {
		if !strings.Contains(err.Error(), candidate) {
			t.Fatalf("歧义提示缺少候选 %s：%s", candidate, err.Error())
		}
	}
}
