package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestFile 写入测试文件并创建父目录。
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readTestFile 读取文件内容。
func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// setupStaging 在临时目标下创建解压目录与指定文件。
func setupStaging(t *testing.T, target string, files map[string]string) string {
	t.Helper()
	staging := filepath.Join(stagingRoot(target), "case")
	for rel, content := range files {
		writeTestFile(t, filepath.Join(staging, filepath.FromSlash(rel)), content)
	}
	return staging
}

// planFor 用相对路径构造安装计划。
func planFor(staging string, rels ...string) InstallPlan {
	files := make([]FileEntry, 0, len(rels))
	for _, rel := range rels {
		files = append(files, FileEntry{Path: rel})
	}
	return InstallPlan{Staging: staging, FromVersion: "v1", ToVersion: "v2", Files: files}
}

func TestApplyPlanSuccess(t *testing.T) {
	target := t.TempDir()
	writeTestFile(t, filepath.Join(target, "old.txt"), "old")
	writeTestFile(t, filepath.Join(target, "keep.txt"), "keep")
	staging := setupStaging(t, target, map[string]string{
		"old.txt":   "new",
		"added.txt": "added",
	})

	plan := planFor(staging, "old.txt", "added.txt")
	plan.Target = target
	if err := ApplyPlan(plan, nil); err != nil {
		t.Fatalf("ApplyPlan 失败: %v", err)
	}

	if got := readTestFile(t, filepath.Join(target, "old.txt")); got != "new" {
		t.Fatalf("old.txt = %q", got)
	}
	if got := readTestFile(t, filepath.Join(target, "added.txt")); got != "added" {
		t.Fatalf("added.txt = %q", got)
	}
	if got := readTestFile(t, filepath.Join(target, "keep.txt")); got != "keep" {
		t.Fatalf("keep.txt = %q", got)
	}
	st, err := readState(target)
	if err != nil || st == nil {
		t.Fatalf("读取状态失败: %v", err)
	}
	if st.Version != "v2" || st.FileCount != 2 {
		t.Fatalf("状态不正确: %+v", st)
	}
	if !fileExists(filepath.Join(target, ".update", "backup")) {
		t.Fatal("备份目录应存在")
	}
	if fileExists(journalPath(target)) {
		t.Fatal("提交后事务日志应被移除")
	}
}

func TestApplyPlanBackupFailureRollsBack(t *testing.T) {
	target := t.TempDir()
	writeTestFile(t, filepath.Join(target, "old.txt"), "old")
	staging := setupStaging(t, target, map[string]string{"old.txt": "new"})

	// 让备份目录的父路径是一个文件，迫使 backupOne 的 MkdirAll 失败。
	blocked := filepath.Join(target, "blocked")
	writeTestFile(t, blocked, "not a dir")

	plan := planFor(staging, "old.txt")
	plan.Target = target
	plan.Backup = filepath.Join(blocked, "backup")

	err := ApplyPlan(plan, nil)
	if err == nil {
		t.Fatal("期望备份失败")
	}
	if got := readTestFile(t, filepath.Join(target, "old.txt")); got != "old" {
		t.Fatalf("回滚后 old.txt = %q", got)
	}
	if st, _ := readState(target); st != nil {
		t.Fatalf("失败后不应留下安装状态: %+v", st)
	}
}

func TestRollbackJournalRestoresPartialApply(t *testing.T) {
	target := t.TempDir()
	writeTestFile(t, filepath.Join(target, "old.txt"), "new-content")
	writeTestFile(t, filepath.Join(target, "added.txt"), "added-content")

	backup := filepath.Join(backupRoot(target), "n1")
	writeTestFile(t, filepath.Join(backup, "old.txt"), "old-content")

	j := &Journal{
		Schema:      updateSchema,
		Nonce:       "n1",
		Phase:       phaseApplying,
		Target:      target,
		FromVersion: "v1",
		ToVersion:   "v2",
		Staging:     filepath.Join(stagingRoot(target), "n1"),
		Backup:      backup,
		Files: []FileEntry{
			{Path: "old.txt", HadOld: true},
			{Path: "added.txt", HadOld: false},
		},
		StateBefore: &State{Schema: updateSchema, Version: "v1"},
	}
	if err := RollbackJournal(j, nil); err != nil {
		t.Fatalf("回滚失败: %v", err)
	}
	if got := readTestFile(t, filepath.Join(target, "old.txt")); got != "old-content" {
		t.Fatalf("old.txt = %q", got)
	}
	if fileExists(filepath.Join(target, "added.txt")) {
		t.Fatal("新增文件应被移除")
	}
	st, err := readState(target)
	if err != nil || st == nil || st.Version != "v1" {
		t.Fatalf("状态未恢复: %+v %v", st, err)
	}
	if fileExists(journalPath(target)) {
		t.Fatal("回滚完成后事务日志应被移除")
	}
}

func TestRecoverOnStartRollsBackInterruptedApply(t *testing.T) {
	target := t.TempDir()
	writeTestFile(t, filepath.Join(target, "old.txt"), "new-content")
	backup := filepath.Join(backupRoot(target), "n2")
	writeTestFile(t, filepath.Join(backup, "old.txt"), "old-content")

	j := &Journal{
		Schema:      updateSchema,
		Nonce:       "n2",
		Phase:       phaseApplying,
		Target:      target,
		FromVersion: "v1",
		ToVersion:   "v2",
		Staging:     filepath.Join(stagingRoot(target), "n2"),
		Backup:      backup,
		Files:       []FileEntry{{Path: "old.txt", HadOld: true}},
	}
	if err := saveJournal(target, j); err != nil {
		t.Fatal(err)
	}

	recovered, err := RecoverOnStart(target)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if !recovered {
		t.Fatal("应报告执行了回滚")
	}
	if got := readTestFile(t, filepath.Join(target, "old.txt")); got != "old-content" {
		t.Fatalf("old.txt = %q", got)
	}
}

func TestSafeJoinRejectsEscape(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"../x", "a/../../x", "/abs", `C:\abs`} {
		if _, err := safeJoin(root, rel); err == nil {
			t.Fatalf("路径 %q 应被拒绝", rel)
		}
	}
	if _, err := safeJoin(root, "a/b.txt"); err != nil {
		t.Fatalf("正常路径被拒绝: %v", err)
	}
}
