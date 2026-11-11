package FileSupport

// versions_test.go 校验版本备份、恢复与「任务起点」版本选择。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/internal/testenv"
)

// isolateVersions 隔离数据目录，并在临时目录清理前关闭版本库连接（Windows 文件锁）。
func isolateVersions(t *testing.T) string {
	t.Helper()
	dir := testenv.Isolate(t)
	t.Cleanup(func() { _ = Close() })
	return dir
}

func TestBackupRestoreAndOldestSince(t *testing.T) {
	isolateVersions(t)
	if err := os.MkdirAll(Abs("documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	rel := "documents/story.md"
	if err := os.WriteFile(Abs(rel), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Unix()
	if _, err := Backup(rel); err != nil {
		t.Fatalf("第一次备份失败: %v", err)
	}
	if err := os.WriteFile(Abs(rel), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(rel); err != nil {
		t.Fatalf("第二次备份失败: %v", err)
	}
	if err := os.WriteFile(Abs(rel), []byte("v3"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldest, ok := OldestSince(rel, since)
	if !ok {
		t.Fatal("应找到任务开始后的最早版本")
	}
	if err := Restore(rel, oldest.ID); err != nil {
		t.Fatalf("恢复版本失败: %v", err)
	}
	got, err := os.ReadFile(Abs(rel))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v1" {
		t.Fatalf("应恢复到任务开始前的内容 v1，实际 %q", got)
	}
}

func TestBackupRecordsMissingFileForDeletion(t *testing.T) {
	isolateVersions(t)
	if err := os.MkdirAll(Abs("documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	rel := "documents/new.txt"
	version, err := Backup(rel)
	if err != nil {
		t.Fatalf("记录不存在文件失败: %v", err)
	}
	if version.Exists {
		t.Fatal("不存在的文件版本应标记 Exists=false")
	}
	if err := os.WriteFile(Abs(rel), []byte("created"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Restore(rel, version.ID); err != nil {
		t.Fatalf("回滚创建操作失败: %v", err)
	}
	if _, err := os.Stat(Abs(rel)); !os.IsNotExist(err) {
		t.Fatalf("回滚后新建文件应被删除，实际 err=%v", err)
	}
}

// TestBackupWithOpRecordsOperation 版本记录带上触发操作，Stats 统计版本与内容占用。
func TestBackupWithOpRecordsOperation(t *testing.T) {
	isolateVersions(t)
	if err := os.MkdirAll(Abs("documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	rel := "documents/op.md"
	if err := os.WriteFile(Abs(rel), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := BackupWithOp(rel, "replace"); err != nil {
		t.Fatalf("备份失败: %v", err)
	}
	versions, err := List(rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Op != "replace" {
		t.Fatalf("版本应记录操作类型 replace，实际 %+v", versions)
	}
	stats, err := Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 1 || stats.Versions != 1 || stats.Bytes != 2 {
		t.Fatalf("版本库统计不正确: %+v", stats)
	}
}

// TestVersionRetentionKeepsRecentOnly 单文件超过上限后只保留最近若干条。
func TestVersionRetentionKeepsRecentOnly(t *testing.T) {
	isolateVersions(t)
	if err := os.MkdirAll(Abs("documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	rel := "documents/history.md"
	for i := 0; i < maxVersionsPerFile+5; i++ {
		if err := os.WriteFile(Abs(rel), []byte(strconv.Itoa(i)), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Backup(rel); err != nil {
			t.Fatalf("第 %d 次备份失败: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	versions, err := List(rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != maxVersionsPerFile {
		t.Fatalf("应保留最近 %d 条，实际 %d", maxVersionsPerFile, len(versions))
	}
}

// TestClearVersions 清空单文件历史后不再返回版本。
func TestClearVersions(t *testing.T) {
	isolateVersions(t)
	if err := os.MkdirAll(Abs("documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	rel := "documents/clear.md"
	if err := os.WriteFile(Abs(rel), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(rel); err != nil {
		t.Fatal(err)
	}
	if err := Clear(rel); err != nil {
		t.Fatalf("清空版本失败: %v", err)
	}
	versions, err := List(rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("清空后不应再有版本，实际 %d", len(versions))
	}
}

// TestLegacyVersionDirectoryImported 旧 file_versions 目录首次打开时导入并改名。
func TestLegacyVersionDirectoryImported(t *testing.T) {
	dir := isolateVersions(t)
	rel := "documents/legacy.md"
	legacyDir := filepath.Join(dir, legacyVersionRootRel, "deadbeef")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	backupRel := filepath.ToSlash(filepath.Join(legacyVersionRootRel, "deadbeef", "1.bak"))
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(backupRel)), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := Version{
		ID:        "1794000000000000000",
		Path:      rel,
		Exists:    true,
		Size:      3,
		BackupRel: backupRel,
		CreatedAt: time.Now().Unix(),
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, legacy.ID+".json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	versions, err := List(rel)
	if err != nil {
		t.Fatalf("读取版本失败: %v", err)
	}
	if len(versions) != 1 || versions[0].ID != legacy.ID {
		t.Fatalf("旧目录未导入版本库: %+v", versions)
	}
	if _, err := os.Stat(filepath.Join(dir, migratedVersionRootRel)); err != nil {
		t.Fatalf("迁移后旧目录应改名: %v", err)
	}
	if err := Restore(rel, legacy.ID); err != nil {
		t.Fatalf("恢复迁移版本失败: %v", err)
	}
	got, err := os.ReadFile(Abs(rel))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("迁移版本内容不正确: %q", got)
	}
}
