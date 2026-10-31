package ArchiveReader

// reader_test.go 归档解析智能体的单元测试：zip 结构 / 条目读取 / GGUF 元数据 / 边界。

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// writeTestZip 在 LocalDir/documents 下生成测试压缩包。
func writeTestZip(t *testing.T, rel string, entries map[string][]byte) {
	t.Helper()
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range entries {
		part, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveListAndReadEntry(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	writeTestZip(t, "documents/pkg.zip", map[string][]byte{
		"a.txt":      []byte("alpha"),
		"docs/b.md":  []byte("# 文档"),
		"binary.dat": {0, 1, 2},
	})

	entries, err := ListEntries("documents/pkg.zip")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("应查询到 3 个条目，实际 %d", len(entries))
	}

	text, err := ReadEntry("documents/pkg.zip", "docs/b.md", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "# 文档") {
		t.Fatalf("条目内容读取错误: %q", text)
	}

	if _, err := ReadEntry("documents/pkg.zip", "binary.dat", 0); err == nil {
		t.Fatal("二进制条目应拒绝文本读取")
	}
	if _, err := ReadEntry("documents/pkg.zip", "missing.txt", 0); err == nil {
		t.Fatal("缺失条目应返回错误")
	}

	summary, err := Summary("documents/pkg.zip")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(summary, "a.txt") || !strings.Contains(summary, "条目：3") {
		t.Fatalf("结构摘要不完整: %s", summary)
	}
}

// TestInspectMatchesEntry 带需求时应返回结构摘要 + 命中条目内容。
func TestInspectMatchesEntry(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	writeTestZip(t, "documents/pkg.zip", map[string][]byte{
		"readme.md": []byte("这里有说明"),
		"main.go":   []byte("package main"),
	})
	text, err := Inspect("documents/pkg.zip", "看看 readme.md 写了什么")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "这里有说明") {
		t.Fatalf("应读取命中条目: %s", text)
	}
}

func TestGGUFSummary(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	dir := filepath.Join(config.LocalDir, "documents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	buffer.WriteString("GGUF")
	if err := binary.Write(&buffer, binary.LittleEndian, uint32(3)); err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(&buffer, binary.LittleEndian, uint64(0)); err != nil { // tensor count
		t.Fatal(err)
	}
	if err := binary.Write(&buffer, binary.LittleEndian, uint64(0)); err != nil { // metadata count
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "model.gguf"), buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := GGUFSummary("documents/model.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(summary, "GGUF") || !strings.Contains(summary, "model.gguf") {
		t.Fatalf("GGUF 摘要不完整: %s", summary)
	}
}

// TestArchiveBoundaries 非压缩包与越界路径必须拒绝。
func TestArchiveBoundaries(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	dir := filepath.Join(config.LocalDir, "documents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(note, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveArchivePath("documents/note.txt"); err == nil {
		t.Fatal("普通文本不应被当作压缩包")
	}
	if _, _, err := ResolveFilePath("../../outside.txt"); err == nil {
		t.Fatal("越界路径必须拒绝")
	}
	if _, err := Summary("documents/note.txt"); err == nil {
		t.Fatal("非压缩包摘要应返回错误")
	}
}
