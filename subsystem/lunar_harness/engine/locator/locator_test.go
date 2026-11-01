package locator

// locator_test.go 统一定位器验收：三种路径形式、大小写、歧义候选、句柄、1000+ 文件性能。

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// setupLocal 准备隔离的 LocalDir 并清空索引
func setupLocal(t *testing.T) {
	t.Helper()
	orig := config.LocalDir
	config.LocalDir = t.TempDir()
	Invalidate()
	t.Cleanup(func() {
		Invalidate()
		config.LocalDir = orig
	})
	if err := os.MkdirAll(filepath.Join(config.LocalDir, "documents"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeTestFile 写入测试文件（自动建目录）
func writeTestFile(t *testing.T, rel string, content string) string {
	t.Helper()
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return abs
}

// TestResolveThreePathForms 完整 / 相对 / 纯文件名三种写法必须命中同一个文件。
func TestResolveThreePathForms(t *testing.T) {
	setupLocal(t)
	abs := writeTestFile(t, "documents/notes/a.md", "alpha")

	forms := []string{
		"documents/notes/a.md", // 完整路径
		"notes/a.md",           // 相对路径
		"a.md",                 // 纯文件名
		"A.MD",                 // 大小写不敏感
		"/document/notes/a.md", // /document 前缀
		"notes/a.md",           // 后缀匹配
	}
	for _, form := range forms {
		match, err := Resolve(form, []string{"documents"})
		if err != nil {
			t.Fatalf("%q 定位失败: %v", form, err)
		}
		if match.Rel != "documents/notes/a.md" || match.Abs != abs {
			t.Fatalf("%q 定位结果错误: %s / %s", form, match.Rel, match.Abs)
		}
	}

	// 路径后缀：x/y/b.md 命中 documents/x/y/b.md
	writeTestFile(t, "documents/x/y/b.md", "beta")
	match, err := Resolve("x/y/b.md", []string{"documents"})
	if err != nil || match.Rel != "documents/x/y/b.md" {
		t.Fatalf("路径后缀定位失败: %+v %v", match, err)
	}
}

// TestResolveAmbiguous 同名文件必须返回候选列表。
func TestResolveAmbiguous(t *testing.T) {
	setupLocal(t)
	writeTestFile(t, "documents/notes/a.md", "one")
	writeTestFile(t, "documents/archive/a.md", "two")

	_, err := Resolve("a.md", []string{"documents"})
	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("应返回歧义错误，实际 %v", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Fatalf("候选数应为 2，实际 %v", ambiguous.Candidates)
	}
	message := err.Error()
	for _, want := range []string{"documents/notes/a.md", "documents/archive/a.md"} {
		if !strings.Contains(message, want) {
			t.Fatalf("歧义提示缺少候选 %s：%s", want, message)
		}
	}
}

// TestResolveHandle 句柄与路径解析到同一个实体。
func TestResolveHandle(t *testing.T) {
	setupLocal(t)
	origArchive := config.ArchiveDBPath
	config.ArchiveDBPath = filepath.Join(config.LocalDir, "database", "handles.db")
	_ = resource.Close()
	t.Cleanup(func() {
		_ = resource.Close()
		config.ArchiveDBPath = origArchive
	})
	if err := resource.Init(config.ArchiveDBPath); err != nil {
		t.Fatalf("初始化句柄库失败: %v", err)
	}
	writeTestFile(t, "documents/notes/a.md", "alpha")
	_, handle, err := resource.RegisterPath("documents/notes/a.md")
	if err != nil || handle == "" {
		t.Fatalf("登记句柄失败: %v / %q", err, handle)
	}

	match, err := Resolve(handle, []string{"documents"})
	if err != nil {
		t.Fatalf("句柄定位失败: %v", err)
	}
	if match.Handle != handle || match.Rel != "documents/notes/a.md" {
		t.Fatalf("句柄定位结果错误: %+v", match)
	}
}

// TestResolvePerformance1000Files 1000+ 文件目录下单次定位必须远低于 100ms。
func TestResolvePerformance1000Files(t *testing.T) {
	setupLocal(t)
	names := make([]string, 0, 1000)
	for dir := 0; dir < 10; dir++ {
		for file := 0; file < 100; file++ {
			name := fmt.Sprintf("d%02d_file%03d.md", dir, file)
			writeTestFile(t, fmt.Sprintf("documents/bulk/d%02d/%s", dir, name), "x")
			names = append(names, name)
		}
	}
	// 预热：首次查询构建索引
	if _, err := Resolve(names[0], []string{"documents"}); err != nil {
		t.Fatalf("预热定位失败: %v", err)
	}
	start := time.Now()
	for index, name := range names {
		if index >= 100 {
			break
		}
		if _, err := Resolve(name, []string{"documents"}); err != nil {
			t.Fatalf("%s 定位失败: %v", name, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("100 次定位耗时 %v，超过 100ms", elapsed)
	}
}
