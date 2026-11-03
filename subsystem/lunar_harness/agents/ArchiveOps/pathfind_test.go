package ArchiveOps

// pathfind_test.go 归档操作的路径定位验收：纯文件名目标与歧义来源。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

func writeDocFile(t *testing.T, rel, content string) {
	t.Helper()
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestOpsResolveByFileName 目标压缩包与来源文件都可以只写文件名。
func TestOpsResolveByFileName(t *testing.T) {
	prepareDocuments(t)
	resourcehub.Invalidate()
	t.Cleanup(resourcehub.Invalidate)

	created := Create("/document/pkg.zip", []string{"a.txt"})
	if !created.OK {
		t.Fatalf("纯文件名来源应能创建压缩包: %s", created.Summary)
	}
	added := AddEntry("pkg.zip", "c.txt", "documents/a.txt", "")
	if !added.OK {
		t.Fatalf("纯文件名目标应能新增条目: %s", added.Summary)
	}
}

// TestOpsAmbiguousSource 同名来源文件必须返回候选列表而不是随意选择。
func TestOpsAmbiguousSource(t *testing.T) {
	prepareDocuments(t)
	resourcehub.Invalidate()
	t.Cleanup(resourcehub.Invalidate)

	writeDocFile(t, "documents/notes/a.txt", "nested-alpha")
	result := Create("/document/out.zip", []string{"a.txt"})
	if result.OK {
		t.Fatalf("同名来源不应直接成功: %s", result.Summary)
	}
	for _, candidate := range []string{"documents/a.txt", "documents/notes/a.txt"} {
		if !strings.Contains(result.Summary, candidate) {
			t.Fatalf("歧义提示缺少候选 %s：%s", candidate, result.Summary)
		}
	}
}
