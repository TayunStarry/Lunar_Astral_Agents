package fileflow

// flow_test.go 文件操作流程的核心验收：快照还原、审查通过 / 不通过、回滚成功与回滚失败上报。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

// writeTestFile 在本地目录下写入文件并返回绝对路径。
func writeTestFile(t *testing.T, rel, content string) string {
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

// readTestFile 读取文件内容。
func readTestFile(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(config.LocalDir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", rel, err)
	}
	return string(raw)
}

// TestSnapshotRestoreRestoresExactContent 快照回滚必须把文件还原到操作前的字节内容。
func TestSnapshotRestoreRestoresExactContent(t *testing.T) {
	testenv.Isolate(t)
	original := "第一行\n第二行\n"
	writeTestFile(t, "documents/notes/a.md", original)

	snapshot, err := Capture("testflow", 1, OpWrite, "/document/notes/a.md", []string{"/document/notes/a.md"})
	if err != nil {
		t.Fatalf("抓取快照失败: %v", err)
	}
	if len(snapshot.Entries) != 1 || !snapshot.Entries[0].Existed {
		t.Fatalf("快照条目不正确: %+v", snapshot.Entries)
	}
	writeTestFile(t, "documents/notes/a.md", "被改坏的内容")
	if err := Restore(snapshot); err != nil {
		t.Fatalf("回滚失败: %v", err)
	}
	if got := readTestFile(t, "documents/notes/a.md"); got != original {
		t.Fatalf("回滚后内容不一致: %q", got)
	}
}

// TestSnapshotRestoreDeletesCreatedFile 操作前不存在的文件，回滚时必须删除。
func TestSnapshotRestoreDeletesCreatedFile(t *testing.T) {
	testenv.Isolate(t)
	writeTestFile(t, "documents/notes/keep.md", "keep")
	snapshot, err := Capture("testflow", 2, OpCreate, "/document/notes/new.md", []string{"/document/notes/new.md"})
	if err != nil {
		t.Fatalf("抓取快照失败: %v", err)
	}
	writeTestFile(t, "documents/notes/new.md", "新建内容")
	if err := Restore(snapshot); err != nil {
		t.Fatalf("回滚失败: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(config.LocalDir, "documents", "notes", "new.md")); !os.IsNotExist(statErr) {
		t.Fatalf("回滚应删除新建文件，实际 err=%v", statErr)
	}
}

// TestReviewPassesWhenExpectationMatches 操作结果与规划预期一致时审查通过。
func TestReviewPassesWhenExpectationMatches(t *testing.T) {
	testenv.Isolate(t)
	writeTestFile(t, "documents/notes/b.md", "内容包含月华")
	exists := true
	review := Evaluate([]Expect{{Path: "/document/notes/b.md", Exists: &exists, Contains: []string{"月华"}}})
	if !review.OK || review.Checked != 1 {
		t.Fatalf("预期应通过: %+v", review)
	}
}

// TestReviewFailureRollsBack 审查不通过时必须立即回滚，并把上下文写进审查结果。
func TestReviewFailureRollsBack(t *testing.T) {
	testenv.Isolate(t)
	original := "操作前的内容"
	writeTestFile(t, "documents/notes/c.md", original)
	snapshot, err := Capture("testflow", 3, OpWrite, "/document/notes/c.md", []string{"/document/notes/c.md"})
	if err != nil {
		t.Fatalf("抓取快照失败: %v", err)
	}
	writeTestFile(t, "documents/notes/c.md", "错误内容")
	exists := true
	review := EvaluateAndRollback(snapshot,
		[]Expect{{Path: "/document/notes/c.md", Exists: &exists, Contains: []string{"预期内容"}}},
		nil, affectedPathsOf(snapshot))
	if review.OK {
		t.Fatal("内容不匹配时审查必须失败")
	}
	if !review.RolledBack || review.RollbackError != "" {
		t.Fatalf("审查失败后应回滚成功: %+v", review)
	}
	if !strings.Contains(review.Context, "已回滚到操作前版本") {
		t.Fatalf("审查上下文应说明回滚: %q", review.Context)
	}
	if got := readTestFile(t, "documents/notes/c.md"); got != original {
		t.Fatalf("回滚后内容不一致: %q", got)
	}
}

// TestRollbackFailureIsExplicit 快照副本缺失时回滚必须报错，不能静默。
func TestRollbackFailureIsExplicit(t *testing.T) {
	testenv.Isolate(t)
	writeTestFile(t, "documents/notes/d.md", "原始内容")
	snapshot, err := Capture("testflow", 4, OpDelete, "/document/notes/d.md", []string{"/document/notes/d.md"})
	if err != nil {
		t.Fatalf("抓取快照失败: %v", err)
	}
	blobAbs := filepath.Join(config.LocalDir, filepath.FromSlash(snapshot.Entries[0].Blob))
	if err := os.Remove(blobAbs); err != nil {
		t.Fatalf("删除快照副本失败: %v", err)
	}
	writeTestFile(t, "documents/notes/d.md", "错误内容")
	exists := true
	review := EvaluateAndRollback(snapshot,
		[]Expect{{Path: "/document/notes/d.md", Exists: &exists, Contains: []string{"预期内容"}}},
		nil, affectedPathsOf(snapshot))
	if review.RollbackError == "" {
		t.Fatalf("快照副本缺失时回滚必须显式上报: %+v", review)
	}
	if review.RolledBack {
		t.Fatalf("回滚失败时不应标记为已回滚: %+v", review)
	}
	if !strings.Contains(review.Context, "回滚失败") {
		t.Fatalf("上下文应说明回滚失败: %q", review.Context)
	}
}

// TestSnapshotRestoreDirectory 目录快照必须能还原整棵目录树。
func TestSnapshotRestoreDirectory(t *testing.T) {
	testenv.Isolate(t)
	writeTestFile(t, "documents/folder/a.md", "A")
	writeTestFile(t, "documents/folder/sub/b.md", "B")
	snapshot, err := Capture("testflow", 5, OpMove, "/document/folder", []string{"/document/folder"})
	if err != nil {
		t.Fatalf("抓取目录快照失败: %v", err)
	}
	if len(snapshot.Entries) != 1 || !snapshot.Entries[0].IsDir || len(snapshot.Entries[0].Files) != 2 {
		t.Fatalf("目录快照条目不正确: %+v", snapshot.Entries)
	}
	if err := os.RemoveAll(filepath.Join(config.LocalDir, "documents", "folder")); err != nil {
		t.Fatalf("删除目录失败: %v", err)
	}
	if err := Restore(snapshot); err != nil {
		t.Fatalf("目录回滚失败: %v", err)
	}
	if got := readTestFile(t, "documents/folder/a.md"); got != "A" {
		t.Fatalf("目录回滚后 a.md 不一致: %q", got)
	}
	if got := readTestFile(t, "documents/folder/sub/b.md"); got != "B" {
		t.Fatalf("目录回滚后 sub/b.md 不一致: %q", got)
	}
}

// TestReviewDirectoryExistsPass 目录只做存在性校验时不应被误判为失败（目录移动依赖此语义）。
func TestReviewDirectoryExistsPass(t *testing.T) {
	testenv.Isolate(t)
	if err := os.MkdirAll(filepath.Join(config.LocalDir, "documents", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	exists := true
	review := Evaluate([]Expect{{Path: "/document/empty", Exists: &exists}})
	if !review.OK || review.Checked != 1 {
		t.Fatalf("目录存在性预期应通过: %+v", review)
	}
	broken := Evaluate([]Expect{{Path: "/document/empty", Contains: []string{"内容"}}})
	if broken.OK {
		t.Fatalf("目录内容校验应显式失败: %+v", broken)
	}
}

// affectedPathsOf 生成预期过滤用的路径集合。
func affectedPathsOf(snapshot Snapshot) map[string]bool {
	out := make(map[string]bool, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		out[entry.Rel] = true
	}
	return out
}
