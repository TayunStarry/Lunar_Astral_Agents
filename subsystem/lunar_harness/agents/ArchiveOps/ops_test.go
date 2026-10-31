package ArchiveOps

// ops_test.go 归档操作智能体的单元测试：创建 / 新增 / 移除 / 删除全生命周期。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/ArchiveReader"
	"LunarSubsystem/LunarHarness/engine/config"
)

// prepareDocuments 在临时 LocalDir 下写入两个源文件。
func prepareDocuments(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })
	dir := filepath.Join(config.LocalDir, "documents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"a.txt": "alpha", "b.txt": "beta"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreateAddRemoveDelete(t *testing.T) {
	prepareDocuments(t)

	created := Create("/document/pkg.zip", []string{"/document/a.txt", "/document/b.txt"})
	if !created.OK {
		t.Fatalf("创建压缩包失败: %s", created.Summary)
	}
	// 产物必须落在 unknown/（不可读二进制根），而不是 documents/
	if !strings.HasPrefix(created.Path, "unknown/") {
		t.Fatalf("压缩包应持久化到 unknown/，实际 %q", created.Path)
	}
	entries, err := ArchiveReader.ListEntries(created.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("创建后应有 2 个条目，实际 %d", len(entries))
	}

	added := AddEntry(created.Path, "c.txt", "", "gamma")
	if !added.OK {
		t.Fatalf("新增条目失败: %s", added.Summary)
	}
	text, err := Read(created.Path, "c.txt", 0)
	if err != nil || !strings.Contains(text, "gamma") {
		t.Fatalf("新增条目读取失败: %v / %q", err, text)
	}

	// 同一条目再次新增应覆盖而不是重复
	overwritten := AddEntry(created.Path, "c.txt", "", "gamma-2")
	if !overwritten.OK {
		t.Fatalf("覆盖条目失败: %s", overwritten.Summary)
	}
	entries, _ = ArchiveReader.ListEntries(created.Path)
	if len(entries) != 3 {
		t.Fatalf("覆盖后条目数应保持 3，实际 %d", len(entries))
	}

	removed := RemoveEntry(created.Path, "b.txt")
	if !removed.OK {
		t.Fatalf("移除条目失败: %s", removed.Summary)
	}
	text, err = Read(created.Path, "b.txt", 0)
	if err == nil {
		t.Fatalf("已移除条目仍可读取: %q", text)
	}

	deleted := Delete(created.Path)
	if !deleted.OK {
		t.Fatalf("删除压缩包失败: %s", deleted.Summary)
	}
	if _, _, err := ArchiveReader.ResolveArchivePath(created.Path); err == nil {
		t.Fatal("删除后压缩包仍存在")
	}
}

// TestCreateFromSourceFile 新增条目可直接引用 /document 下的本地文件。
func TestCreateFromSourceFile(t *testing.T) {
	prepareDocuments(t)
	created := Create("/document/pkg.zip", []string{"/document/a.txt"})
	if !created.OK {
		t.Fatalf("创建压缩包失败: %s", created.Summary)
	}
	added := AddEntry(created.Path, "copied.txt", "/document/b.txt", "")
	if !added.OK {
		t.Fatalf("从本地文件新增条目失败: %s", added.Summary)
	}
	text, err := Read(created.Path, "copied.txt", 0)
	if err != nil || !strings.Contains(text, "beta") {
		t.Fatalf("本地文件条目内容错误: %v / %q", err, text)
	}
}

// TestOperationBoundaries 非压缩包目标与空来源必须拒绝。
func TestOperationBoundaries(t *testing.T) {
	prepareDocuments(t)
	if result := Create("/document/a.txt", []string{"/document/b.txt"}); result.OK {
		t.Fatal("非 .zip / .ltpx 目标不应创建成功")
	}
	if result := Create("/document/empty.zip", nil); result.OK {
		t.Fatal("空来源不应创建成功")
	}
	if result := Delete("/document/not-exist.zip"); result.OK {
		t.Fatal("不存在的压缩包不应删除成功")
	}
	if result := AddEntry("/document/not-exist.zip", "../escape.txt", "", "x"); result.OK {
		t.Fatal("含 .. 的条目名必须拒绝")
	}
}
