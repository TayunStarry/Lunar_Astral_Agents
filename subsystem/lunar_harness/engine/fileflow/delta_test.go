package fileflow

// delta_test.go 内容级增量记录的验收：新增 / 删除 / 替换 / 二进制四类记录、
// 只靠数据库按流程回退、以及保留策略清理。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

// recordTestOp 执行一次快照 + 增量采集 + 落库，返回生成的记录。
func recordTestOp(t *testing.T, flowID string, seq int, op, target string, paths []string, mutate func()) OpRecord {
	t.Helper()
	snapshot, err := Capture(flowID, seq, op, target, paths)
	if err != nil {
		t.Fatalf("抓取快照失败: %v", err)
	}
	if mutate != nil {
		mutate()
	}
	entry := snapshot.Entries[0]
	built, err := BuildOp(OpInput{
		FlowID: flowID, Seq: seq, Step: seq, Mode: ModeAssigned,
		Op: op, Target: target, PathRel: entry.Rel, PathAbs: entry.Abs, Tool: "test_tool",
		Snapshot: snapshot,
	})
	if err != nil {
		t.Fatalf("采集内容增量失败: %v", err)
	}
	if err := built.Commit(Review{OK: true}); err != nil {
		t.Fatalf("写入增量记录失败: %v", err)
	}
	ops, err := FlowOps(flowID)
	if err != nil {
		t.Fatalf("读取流程记录失败: %v", err)
	}
	for _, record := range ops {
		if record.Seq == seq {
			return record
		}
	}
	t.Fatalf("未找到步骤 %d 的记录", seq)
	return OpRecord{}
}

// TestDeltaCreateRecordsNewContent 新增文件必须记录全文片段与哈希 / 大小。
func TestDeltaCreateRecordsNewContent(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = CloseDeltaStore() })
	content := "第一行\n第二行\n"
	record := recordTestOp(t, "delta-create", 1, OpCreate, "/document/notes/new.md",
		[]string{"/document/notes/new.md"}, func() {
			writeTestFile(t, "documents/notes/new.md", content)
		})
	if len(record.Deltas) != 1 || record.Deltas[0].Kind != DeltaCreate {
		t.Fatalf("新增操作应生成 create 增量: %+v", record.Deltas)
	}
	delta := record.Deltas[0]
	if delta.NewText != content || delta.NewSize != int64(len(content)) {
		t.Fatalf("新增增量内容不正确: %+v", delta)
	}
	if delta.OldSize != 0 || record.BeforeSize != 0 || record.AfterSize != int64(len(content)) {
		t.Fatalf("大小字段不正确: delta=%+v record=%+v", delta, record)
	}
	if record.AfterSHA256 == "" {
		t.Fatalf("新增操作应记录操作后哈希: %+v", record)
	}
}

// TestDeltaDeleteRecordsRemovedContent 删除文件必须记录被删内容与操作前哈希。
func TestDeltaDeleteRecordsRemovedContent(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = CloseDeltaStore() })
	original := "要删掉的内容\n"
	abs := writeTestFile(t, "documents/notes/old.md", original)
	record := recordTestOp(t, "delta-delete", 1, OpDelete, "/document/notes/old.md",
		[]string{"/document/notes/old.md"}, func() {
			if err := os.Remove(abs); err != nil {
				t.Fatal(err)
			}
		})
	if len(record.Deltas) != 1 || record.Deltas[0].Kind != DeltaDelete {
		t.Fatalf("删除操作应生成 delete 增量: %+v", record.Deltas)
	}
	delta := record.Deltas[0]
	if delta.OldText != original || delta.NewText != "" || delta.NewSize != 0 {
		t.Fatalf("删除增量内容不正确: %+v", delta)
	}
	if record.BeforeSHA256 == "" || record.BeforeSize != int64(len(original)) {
		t.Fatalf("删除操作应记录操作前哈希与大小: %+v", record)
	}
}

// TestDeltaReplaceRecordsLineRange 替换必须记录替换前后片段及其行号定位。
func TestDeltaReplaceRecordsLineRange(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = CloseDeltaStore() })
	original := "alpha\nbravo\ncharlie\n"
	updated := "alpha\nBRAVO\ncharlie\n"
	writeTestFile(t, "documents/notes/edit.md", original)
	record := recordTestOp(t, "delta-replace", 1, OpWrite, "/document/notes/edit.md",
		[]string{"/document/notes/edit.md"}, func() {
			writeTestFile(t, "documents/notes/edit.md", updated)
		})
	if len(record.Deltas) == 0 {
		t.Fatal("替换操作必须产生内容增量")
	}
	delta := record.Deltas[0]
	if delta.OldText != "bravo" || delta.NewText != "BRAVO" {
		t.Fatalf("替换片段不正确: %+v", delta)
	}
	if delta.LineFrom != 2 || delta.LineTo != 2 {
		t.Fatalf("替换行号定位不正确: %+v", delta)
	}
	if !strings.Contains(delta.Locator, "第 2-2 行") {
		t.Fatalf("定位描述不正确: %q", delta.Locator)
	}
}

// TestDeltaBinaryRecordsReadableConclusion 二进制文件只记录哈希 / 大小与可读差异结论。
func TestDeltaBinaryRecordsReadableConclusion(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = CloseDeltaStore() })
	original := []byte{0x00, 0x01, 0x02, 0x03}
	updated := append(append([]byte{}, original...), 0xFF, 0xFE)
	abs := filepath.Join(config.LocalDir, "documents", "notes", "blob.bin")
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, original, 0o644); err != nil {
		t.Fatal(err)
	}
	record := recordTestOp(t, "delta-binary", 1, OpWrite, "/document/notes/blob.bin",
		[]string{"/document/notes/blob.bin"}, func() {
			if err := os.WriteFile(abs, updated, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	if len(record.Deltas) != 1 {
		t.Fatalf("二进制修改应生成一条增量: %+v", record.Deltas)
	}
	delta := record.Deltas[0]
	if !delta.Binary || delta.OldText != "" || delta.NewText != "" {
		t.Fatalf("二进制增量不应保存文本片段: %+v", delta)
	}
	if !strings.Contains(delta.BinaryNote, "追加") {
		t.Fatalf("二进制增量应给出可读差异结论: %q", delta.BinaryNote)
	}
	if delta.OldSize != int64(len(original)) || delta.NewSize != int64(len(updated)) {
		t.Fatalf("二进制大小变化不正确: %+v", delta)
	}
}

// TestRollbackToStepFromDatabaseOnly 删除快照目录后，文本操作仍应只靠数据库记录回退到指定步骤。
func TestRollbackToStepFromDatabaseOnly(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = CloseDeltaStore() })
	flowID := "rollback-flow"
	original := "version-one\n"

	recordTestOp(t, flowID, 1, OpCreate, "/document/notes/a.md",
		[]string{"/document/notes/a.md"}, func() {
			writeTestFile(t, "documents/notes/a.md", original)
		})
	recordTestOp(t, flowID, 2, OpWrite, "/document/notes/a.md",
		[]string{"/document/notes/a.md"}, func() {
			writeTestFile(t, "documents/notes/a.md", "version-two\n")
		})
	recordTestOp(t, flowID, 3, OpCreate, "/document/notes/b.md",
		[]string{"/document/notes/b.md"}, func() {
			writeTestFile(t, "documents/notes/b.md", "second\n")
		})

	// 去掉快照，证明回退用的是数据库内容增量而不是快照兜底
	if err := os.RemoveAll(filepath.Join(config.LocalDir, filepath.FromSlash(SnapshotRootRel), flowID)); err != nil {
		t.Fatalf("删除快照目录失败: %v", err)
	}

	result, err := RollbackTo(flowID, 1)
	if err != nil {
		t.Fatalf("回退失败: %v", err)
	}
	if !result.OK || result.Reverted != 2 {
		t.Fatalf("应回退步骤 2、3 两条操作: %+v", result)
	}
	if got := readTestFile(t, "documents/notes/a.md"); got != original {
		t.Fatalf("步骤 2 的修改未还原: %q", got)
	}
	if _, statErr := os.Stat(filepath.Join(config.LocalDir, "documents", "notes", "b.md")); !os.IsNotExist(statErr) {
		t.Fatalf("步骤 3 新建的文件未删除: %v", statErr)
	}

	result, err = RollbackTo(flowID, 0)
	if err != nil {
		t.Fatalf("回退到 0 失败: %v", err)
	}
	if !result.OK || result.Reverted != 1 {
		t.Fatalf("应回退步骤 1: %+v", result)
	}
	if _, statErr := os.Stat(filepath.Join(config.LocalDir, "documents", "notes", "a.md")); !os.IsNotExist(statErr) {
		t.Fatalf("步骤 1 新建的文件未删除: %v", statErr)
	}

	ops, err := FlowOps(flowID)
	if err != nil {
		t.Fatalf("读取流程记录失败: %v", err)
	}
	for _, op := range ops {
		if op.Status != OpStatusRolledBack {
			t.Fatalf("回退后状态未更新: %+v", op)
		}
	}
}

// TestCleanupDeltasByAgeAndFlowCount 保留策略按流程数与时间清理，不误删保留范围内的记录。
func TestCleanupDeltasByAgeAndFlowCount(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = CloseDeltaStore() })
	db, err := deltaConn()
	if err != nil {
		t.Fatalf("打开增量库失败: %v", err)
	}
	for i := 0; i < 4; i++ {
		flowID := "cleanup-flow-" + string(rune('a'+i))
		writeTestFile(t, "documents/notes/"+flowID+".md", "x")
		recordTestOp(t, flowID, 1, OpCreate, "/document/notes/"+flowID+".md",
			[]string{"/document/notes/" + flowID + ".md"}, func() {})
		// 人为拉开时间：i 越大越新
		if _, err := db.Exec(`UPDATE `+deltaOpTable+` SET created_at = ? WHERE flow_id = ?`,
			time.Now().Add(-time.Duration(10-i)*time.Hour).Unix(), flowID); err != nil {
			t.Fatalf("调整时间戳失败: %v", err)
		}
	}

	if err := CleanupDeltas(2, 0); err != nil {
		t.Fatalf("按流程数清理失败: %v", err)
	}
	for i, flowID := range []string{"cleanup-flow-a", "cleanup-flow-b"} {
		ops, err := FlowOps(flowID)
		if err != nil {
			t.Fatalf("读取清理后的流程失败: %v", err)
		}
		if len(ops) != 0 {
			t.Fatalf("最旧的流程 %d 应被清理: %+v", i, ops)
		}
	}
	for _, flowID := range []string{"cleanup-flow-c", "cleanup-flow-d"} {
		ops, err := FlowOps(flowID)
		if err != nil {
			t.Fatalf("读取保留流程失败: %v", err)
		}
		if len(ops) != 1 {
			t.Fatalf("保留范围内流程不应被清理: %s → %d", flowID, len(ops))
		}
	}

	if _, err := db.Exec(`UPDATE `+deltaOpTable+` SET created_at = ?`, time.Now().Add(-31*24*time.Hour).Unix()); err != nil {
		t.Fatalf("调整时间戳失败: %v", err)
	}
	if err := CleanupDeltas(10, 0); err != nil {
		t.Fatalf("按时间清理失败: %v", err)
	}
	ops, err := FlowOps("cleanup-flow-d")
	if err != nil {
		t.Fatalf("读取清理后的流程失败: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("超过保留期的记录应被清理: %+v", ops)
	}
}
